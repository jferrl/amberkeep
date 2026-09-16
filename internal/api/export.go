package api

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Writing the archive out, from a browser.
//
// The command has been able to do this from the beginning and the page could not,
// which put the most ordinary thing somebody wants — a copy of their own history
// they can keep, print or hand to a solicitor — behind a terminal. The shape follows
// the import: work runs in the background, one thing says what is happening, and the
// page asks that one thing.
//
// Unlike a migration this writes nothing anybody depends on. It produces files in a
// folder and touches no original and no device, so it does not need the word typed
// out, and it may be asked for again as often as somebody likes.

// ExportStage is how far along an export is.
type ExportStage string

// The stages.
const (
	// ExportIdle is nothing asked for yet.
	ExportIdle ExportStage = "idle"
	// ExportWriting is writing files.
	ExportWriting ExportStage = "writing"
	// ExportDone is a folder of files, and where it is.
	ExportDone ExportStage = "done"
	// ExportFailed is what went wrong, and what to do about it.
	ExportFailed ExportStage = "failed"
)

// ExportRequest is what a page asks for.
type ExportRequest struct {
	// Into is the folder the files go in. Empty means the workspace.
	Into string `json:"into"`
	// Formats are the ones wanted: html, text, json.
	Formats []string `json:"formats"`
	// Only names the conversations to write, by address. Empty means all of them.
	Only []string `json:"only,omitempty"`
	// Words are what the exported page calls itself, in the reader's language. The
	// page sends them because the page is where this program's words live; a server
	// that guessed would produce an English archive for a Spanish reader, which is
	// what it did.
	Words struct {
		Title       string `json:"title"`
		Noun        string `json:"noun"`
		Placeholder string `json:"placeholder"`
	} `json:"words"`

	// Groups includes group conversations. Ignored when Only names them.
	Groups bool `json:"groups"`
	// Notices writes what WhatsApp did as well as what people said.
	Notices bool `json:"notices"`
	// Media copies the archive's own photographs, videos and recordings out beside
	// the pages. It does nothing at all for an archive that has none, which is most
	// of them; where it does something it is usually the largest part of the export.
	Media bool `json:"media"`

	// Me and Location come from how the server was started, not from the page: they
	// are how every other part of this archive is already being read, and an export
	// that disagreed with the screen it was started from would be a different
	// archive wearing the same name.
	Me       string         `json:"-"`
	Location *time.Location `json:"-"`
}

// Exported is what an export produced.
type Exported struct {
	// Into is the folder to go and look in.
	Into          string   `json:"into"`
	Conversations int      `json:"conversations"`
	Messages      int      `json:"messages"`
	Bytes         int64    `json:"bytes"`
	Formats       []string `json:"formats"`
	// Carried is how many of the archive's own photographs, videos and recordings
	// were copied out beside the pages, and how much they came to. Zero for the
	// great majority of archives, which have none.
	Carried      int   `json:"carried,omitempty"`
	CarriedBytes int64 `json:"carried_bytes,omitempty"`
}

// Exportable is an archive that can write itself out.
//
// A separate interface rather than more of Archive, because an archive that cannot
// do this is still a perfectly good archive to read — and because widening the one
// every caller implements, to add something only one of them needs, is how an
// interface stops being a seam and starts being a list.
type Exportable interface {
	Export(ctx context.Context, ask ExportRequest, say Progress) (Exported, error)
}

// Export is how far along an export is, and what it produced.
//
// The running commentary is a Note rather than a sentence, so a page writing "7.450
// de 10.500 conversaciones escritas" can, instead of being handed the English and
// the raw digits. It is embedded rather than held in a field of its own because what
// the page is told has not changed shape: a detail, and now a name and the numbers
// beside it.
type Export struct {
	Stage ExportStage `json:"stage"`
	Step  Step        `json:"step,omitempty"`
	Note
	// Guidance is the several lines of what to do about a failure.
	Guidance string    `json:"guidance,omitempty"`
	Result   *Exported `json:"result,omitempty"`
}

// exporting holds how far along the one export is.
//
// One at a time, like everything else here: two exports into the same folder would
// race over the same filenames, and the page has nowhere to show a second one.
type exporting struct {
	mu    sync.Mutex
	state Export
}

func newExporting() *exporting { return &exporting{state: Export{Stage: ExportIdle}} }

func (e *exporting) now() Export {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

// begin starts an export, reporting whether it did. It does not when one is running.
func (e *exporting) begin() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state.Stage == ExportWriting {
		return false
	}
	e.state = Export{Stage: ExportWriting, Step: StepWriting, Note: Saying("starting", "Starting.")}
	return true
}

func (e *exporting) progress(step Step, said Note) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state.Stage != ExportWriting {
		return
	}
	e.state.Step, e.state.Note = step, said
}

func (e *exporting) done(result Exported) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.state = Export{Stage: ExportDone, Result: &result}
}

func (e *exporting) failed(detail, guidance string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.state = Export{Stage: ExportFailed, Note: Note{Text: detail}, Guidance: guidance}
}

func (e *exporting) forget() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state.Stage == ExportWriting {
		return
	}
	e.state = Export{Stage: ExportIdle}
}

// handleExport says how far along an export is. The only thing the page polls.
func (s *server) handleExport(w http.ResponseWriter, r *http.Request) {
	write(w, r, s.exports.now())
}

// handleWriteExport starts one.
func (s *server) handleWriteExport(w http.ResponseWriter, r *http.Request) {
	open, ok := s.reading(w)
	if !ok {
		return
	}
	out, can := open.archive.(Exportable)
	if !can {
		http.Error(w, "this archive cannot be written out", http.StatusNotImplemented)
		return
	}

	var ask ExportRequest
	if !readRequest(w, r, &ask) {
		return
	}
	if len(ask.Formats) == 0 {
		http.Error(w, "say which formats to write: html, text or json", http.StatusBadRequest)
		return
	}
	if ask.Into == "" {
		ask.Into = s.opts.Workspace
	}
	ask.Me, ask.Location = s.opts.Me, s.opts.Location

	if !s.exports.begin() {
		http.Error(w, "an export is already running", http.StatusConflict)
		return
	}
	// Taken here, before the work starts, because this reply is about this request:
	// it was accepted and it is writing. Read after the goroutine and a small archive
	// can finish first, so the same request would sometimes be answered with the
	// state of its own ending — true, but not what was asked, and not the same answer
	// twice.
	started := s.exports.now()

	// The work outlives the request that asked for it: an archive of a million
	// messages takes a minute, and somebody who closes the tab halfway through
	// should come back to a finished folder rather than half of one.
	//nolint:contextcheck // see above
	go func() {
		ctx := context.WithoutCancel(s.background)
		result, err := out.Export(ctx, ask, Progress(s.exports.progress))
		if err != nil {
			s.exports.failed(sentence(err), s.advise(err))
			return
		}
		s.exports.done(result)
	}()

	writeStatus(w, r, http.StatusAccepted, started)
}

// handleForgetExport clears a finished export so the screen can be used again.
func (s *server) handleForgetExport(w http.ResponseWriter, r *http.Request) {
	s.exports.forget()
	write(w, r, s.exports.now())
}
