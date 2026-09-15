package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// The endpoints the wizard uses to get somebody from a phone to an archive.
//
// Each one starts work and returns at once, because a person watching a blank
// screen for ten seconds assumes it has hung. What is happening is reported by
// polling the state, which is also what the page watches to know when the archive
// has appeared underneath it.

// handleState says what the server is doing. It is the only thing the wizard polls.
func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	state := s.session.state()
	if open := s.session.archive(); open != nil {
		state["archive"] = s.summarise(open)
	}
	write(w, r, state)
}

// handleBackups lists the iPhone backups on this computer.
//
// A problem reaching them is part of the answer rather than an error, because "none
// found" and "this program has not been allowed to look" are the same empty list and
// completely different situations for the person reading it.
func (s *server) handleBackups(w http.ResponseWriter, r *http.Request) {
	if s.importer == nil {
		http.Error(w, "this server was started without the means to import anything", http.StatusNotImplemented)
		return
	}

	backups, problem := s.importer.Backups()
	if backups == nil {
		backups = []Backup{}
	}

	// Where it looked, so an empty list can be read correctly. Nowhere to look and
	// nothing found there are the same empty list and different situations.
	looked := s.importer.Locations()
	if looked == nil {
		looked = []string{}
	}

	body := map[string]any{"backups": backups, "looked": looked}
	if problem.Said != "" {
		body["problem"] = problem.Said
	}
	// The name of what to do about it rather than the words: the page reads in
	// whichever language its reader chose, and it already has the words for every
	// failure somebody can act on.
	if problem.Guidance != "" {
		body["guidance"] = problem.Guidance
	}
	write(w, r, body)
}

// opening is what a wizard step asks for. The fields each step needs are named
// separately rather than shared, so a request that is missing one says which.
type opening struct {
	Path     string `json:"path"`
	Backup   string `json:"backup"`
	File     string `json:"file"`
	Key      string `json:"key"`
	Into     string `json:"into"`
	Contacts string `json:"contacts"`
}

// handleOpen reads an archive that is already readable.
func (s *server) handleOpen(w http.ResponseWriter, r *http.Request) {
	ask, ok := s.accept(w, r, func(a opening) string {
		if a.Path == "" {
			return "which file to open is needed"
		}
		return ""
	})
	if !ok {
		return
	}

	//nolint:contextcheck // the work outlives this request on purpose; see start.
	s.begun(w, r, s.start(StepOpening, Saying("openingArchive", "Opening the archive."),
		func(context.Context, Progress) (string, error) { return ask.Path, nil }, ask.Contacts))
}

// handleExtract takes the message store out of an iPhone backup.
func (s *server) handleExtract(w http.ResponseWriter, r *http.Request) {
	ask, ok := s.accept(w, r, func(a opening) string {
		if a.Backup == "" {
			return "which backup to take it out of is needed"
		}
		return ""
	})
	if !ok {
		return
	}

	into := s.workspaceFor(ask.Into)
	//nolint:contextcheck // the work outlives this request on purpose; see start.
	s.begun(w, r, s.start(StepExtracting,
		Saying("takingMessagesOut", "Taking the messages out of the backup, with their pictures."),
		func(ctx context.Context, say Progress) (string, error) {
			return s.importer.Extract(ctx, ask.Backup, into, say)
		}, ask.Contacts))
}

// handleDecrypt turns an encrypted Android backup into something readable.
func (s *server) handleDecrypt(w http.ResponseWriter, r *http.Request) {
	ask, ok := s.accept(w, r, func(a opening) string {
		switch {
		case a.File == "":
			return "which backup to decrypt is needed"
		case a.Key == "":
			return "the 64-digit key is needed"
		default:
			return ""
		}
	})
	if !ok {
		return
	}

	// The key is copied into the work and the request's own copy goes out of scope
	// with it. It is never written down, never logged, and never put in an address.
	key := ask.Key
	into := s.workspaceFor(ask.Into)

	//nolint:contextcheck // the work outlives this request on purpose; see start.
	s.begun(w, r, s.start(StepDecrypting,
		Saying("decryptingBackup", "Decrypting the backup. Nothing is being uploaded."),
		func(ctx context.Context, say Progress) (string, error) {
			return s.importer.Decrypt(ctx, ask.File, key, into, say)
		}, ask.Contacts))
}

// handleClose forgets the archive and goes back to the beginning.
func (s *server) handleClose(w http.ResponseWriter, r *http.Request) {
	s.session.close()
	write(w, r, s.session.state())
}

// accept reads a wizard request and checks it, answering the caller when it cannot.
func (s *server) accept(w http.ResponseWriter, r *http.Request, check func(opening) string) (opening, bool) {
	if s.importer == nil {
		http.Error(w, "this server was started without the means to import anything", http.StatusNotImplemented)
		return opening{}, false
	}

	var ask opening
	if !readRequest(w, r, &ask) {
		return opening{}, false
	}
	if missing := check(ask); missing != "" {
		http.Error(w, missing, http.StatusBadRequest)
		return opening{}, false
	}

	if ask.Into != "" {
		s.session.setWorkspace(ask.Into)
	}
	return ask, true
}

// readRequest reads a wizard request into v.
//
// A wizard request is a few short strings. Anything larger is not one, and reading it
// would only give a malformed request somewhere to put a gigabyte.
func readRequest(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(v); err != nil {
		http.Error(w, "that request could not be read", http.StatusBadRequest)
		return false
	}
	return true
}

// begun answers a request that asked for work.
//
// The status says the work has been taken rather than finished, and the body is the
// state the page is about to start polling, so the first thing it draws is what is
// happening now rather than what was happening one round trip ago.
func (s *server) begun(w http.ResponseWriter, r *http.Request, started bool) {
	if !started {
		http.Error(w, "something is already being opened", http.StatusConflict)
		return
	}
	writeStatus(w, r, http.StatusAccepted, s.session.state())
}

// start runs one piece of work in the background and opens whatever it produces.
// It reports whether the work began, which it does not when something else is
// already running: two decryptions writing to the same file would ruin both.
//
// The context deliberately outlives the request that began the work. A person who
// closes the tab halfway through a decryption should come back to a finished
// archive, not to a half-written file, and the request is over as soon as it has
// been accepted.
func (s *server) start(step Step, said Note, work func(context.Context, Progress) (string, error), contacts string) bool {
	if !s.session.begin(step, said) {
		return false
	}

	go func() {
		ctx := context.WithoutCancel(s.background)
		say := Progress(s.session.progress)

		path, err := work(ctx, say)
		if err != nil {
			s.session.failed(sentence(err), s.advise(err))
			return
		}

		say(StepOpening, Saying("readingArchive", "Reading the archive."))
		archive, err := s.importer.Open(ctx, path, contacts, say)
		if err != nil {
			s.session.failed(sentence(err), s.advise(err))
			return
		}

		open, err := s.prepare(ctx, archive)
		if err != nil {
			_ = archive.Close()
			s.session.failed(sentence(err), s.advise(err))
			return
		}
		s.session.ready(open)
	}()
	return true
}

// workspaceFor is where to write, which is what the person was shown and had the
// chance to change.
func (s *server) workspaceFor(asked string) string {
	if asked != "" {
		return asked
	}
	return s.opts.Workspace
}

// sentence turns a failure into the one line shown prominently on screen.
//
// The wrapping that helps somebody reading a log is noise to somebody who just
// wants to know what went wrong, so only the outermost cause is shown and the
// advice below it does the explaining.
func sentence(err error) string {
	said := err.Error()
	if cut := strings.Index(said, ": "); cut > 0 && len(said) > cut+2 {
		// Keep the first clause when what follows is another layer of wrapping
		// rather than the actual reason.
		var deeper interface{ Unwrap() error }
		if errors.As(err, &deeper) {
			return said
		}
	}
	return said
}

// advise returns the several lines of help that go with a failure, when there are
// any. What they say is the caller's business; this package only carries them.
func (s *server) advise(err error) string {
	if s.opts.Advise == nil {
		return ""
	}
	return s.opts.Advise(err)
}

// Workspace is where this program writes when nobody says otherwise.
//
// Exported because the parts that write are not all inside this package: reading a
// backup off a phone puts it somewhere, and a second copy of this rule would be two
// programs disagreeing about where somebody's files went.
func Workspace() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return defaultWorkspace(home)
}

// defaultWorkspace is where things are written when nobody says otherwise.
//
// It is inside the user's own home directory and named after this program, so that
// somebody who has forgotten everything about this can still find their archive by
// looking. It is shown on screen before anything is written to it.
func defaultWorkspace(home string) string {
	if home == "" {
		return "amberkeep"
	}
	return filepath.Join(home, "Amberkeep")
}
