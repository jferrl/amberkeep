package api

import (
	"context"
	"net/http"

	"github.com/jferrl/amberkeep/internal/guide"
)

// The endpoints a migration uses.
//
// Four of them, and none leads to the next on its own. Checking does not begin
// planning; a plan does not begin writing. Each stage ends and waits, because the
// thing at the end of this is somebody restoring a backup onto a phone they depend
// on, and a flow that carries them along is a flow they can arrive at the end of
// without having read anything.

// handleMigration says how far along a migration is. The only thing the page polls.
func (s *server) handleMigration(w http.ResponseWriter, r *http.Request) {
	write(w, r, s.migration.state())
}

// handleGuide returns what somebody has to be told, and when.
//
// Served rather than written into the page, because these words live in one place —
// now a catalogue per language beside the steps themselves — and a copy in the
// frontend would drift from this one the first time anybody corrected a sentence.
//
// The language is asked for rather than guessed. The page knows what its reader
// reads; this server knows only what a header claims, and a restore instruction in
// the wrong language is worse than one in a language somebody has already accepted.
func (s *server) handleGuide(w http.ResponseWriter, r *http.Request) {
	lang := guide.Spoken(r.URL.Query().Get("lang"))
	// The checks' own sentences ride along with the steps rather than having an
	// endpoint of their own: they are read on the screen that already has this,
	// and a finding names its two sentences the way a step names itself.
	write(w, r, map[string]any{"stages": guidance(lang), "checks": guide.AllChecks(lang)})
}

// handleAdvice returns what to say about every failure somebody can act on.
//
// All of them at once, rather than one when it happens. A page can only ask about a
// failure after the failure, which is the moment it least wants to be waiting on
// another request — and there are two dozen of these, which is a few kilobytes.
//
// Every state that can fail carries an identifier rather than words, and this is the
// other half of that: the identifier says which of these to show, and the language
// is the page's to choose.
func (s *server) handleAdvice(w http.ResponseWriter, r *http.Request) {
	write(w, r, guide.AllAdvice(guide.Spoken(r.URL.Query().Get("lang"))))
}

// handleCheck looks at a backup: whether it can be used at all, and what has to be
// true that nothing here can see.
func (s *server) handleCheck(w http.ResponseWriter, r *http.Request) {
	ask, ok := s.acceptMigration(w, r, func(a MigrationRequest) string {
		if a.Backup == "" {
			return "which backup to look at is needed"
		}
		return ""
	})
	if !ok {
		return
	}

	started := s.migration.begin(MigrationChecking, StepOpening, Saying("lookingAtBackup", "Looking at the backup."), ask)
	if started {
		// The work deliberately outlives the request that asked for it: somebody who
		// closes the tab halfway through should come back to a finished migration.
		//nolint:contextcheck // see above
		go func() {
			ctx := context.WithoutCancel(s.background)
			ready, err := s.migrator.Check(ctx, ask.Backup)
			if err != nil {
				s.migration.failed(sentence(err), s.advise(err))
				return
			}
			s.migration.checked(ready)
		}()
	}
	s.beganMigrating(w, r, started)
}

// handlePlanMigration works out what would move. It writes nothing, and it stops.
func (s *server) handlePlanMigration(w http.ResponseWriter, r *http.Request) {
	ask, ok := s.acceptMigration(w, r, func(a MigrationRequest) string {
		switch {
		case a.Backup == "":
			return "which backup to move into is needed"
		case a.Android == "":
			return "which Android database to move is needed"
		default:
			return ""
		}
	})
	if !ok {
		return
	}

	started := s.migration.begin(MigrationPlanning, StepOpening, Saying("workingOutWhatMoves", "Working out what would move."), ask)
	if started {
		// The work deliberately outlives the request that asked for it: somebody who
		// closes the tab halfway through should come back to a finished migration.
		//nolint:contextcheck // see above
		go func() {
			ctx := context.WithoutCancel(s.background)
			plan, err := s.migrator.Plan(ctx, ask, Progress(s.migration.progress))
			if err != nil {
				s.migration.failed(sentence(err), s.advise(err))
				return
			}
			s.migration.planned(plan)
		}()
	}
	s.beganMigrating(w, r, started)
}

// agreement is what somebody has to send to have anything written.
type agreement struct {
	// Confirm has to be the word, typed out. A button alone is a button somebody
	// clicked through; a word is a word they wrote.
	Confirm string `json:"confirm"`
	// Into is where to write the changed backup.
	Into string `json:"into,omitempty"`
}

// theWord is what has to be typed before anything is written.
const theWord = "migrate"

// handleCarryOut does it: into a copy of the store, checked, then into a copy of the
// backup. Nothing it touches is an original and no device is involved.
func (s *server) handleCarryOut(w http.ResponseWriter, r *http.Request) {
	if s.migrator == nil {
		http.Error(w, "this server was started without the means to migrate anything", http.StatusNotImplemented)
		return
	}

	var said agreement
	if !readRequest(w, r, &said) {
		return
	}
	if said.Confirm != theWord {
		http.Error(w, "type "+theWord+" to go on", http.StatusBadRequest)
		return
	}

	plan, ok := s.migration.agreed()
	if !ok {
		http.Error(w, "there is no plan to carry out; ask for one first", http.StatusConflict)
		return
	}

	ask := s.migration.request()
	if said.Into != "" {
		ask.Into = said.Into
	}

	started := s.migration.begin(MigrationWorking, StepPreparing,
		Saying("movingMessages", "Moving the messages. Nothing is being uploaded."), ask)
	if started {
		// The work deliberately outlives the request that asked for it: somebody who
		// closes the tab halfway through should come back to a finished migration.
		//nolint:contextcheck // see above
		go func() {
			ctx := context.WithoutCancel(s.background)
			result, err := s.migrator.Carry(ctx, ask, plan, Progress(s.migration.progress))
			if err != nil {
				s.migration.failed(sentence(err), s.advise(err))
				return
			}
			s.migration.done(result)
		}()
	}
	s.beganMigrating(w, r, started)
}

// handleForgetMigration goes back to the beginning.
func (s *server) handleForgetMigration(w http.ResponseWriter, r *http.Request) {
	if s.migration.busy() {
		http.Error(w, "something is still running", http.StatusConflict)
		return
	}
	s.migration.forget()
	write(w, r, s.migration.state())
}

// beganMigrating answers a request that asked for migration work to start.
//
// It is not the wizard's begun. That one answers with the import session's state,
// which is a different job on a different screen: a page putting it where it keeps
// the migration would be told the migration was "empty", a stage no migration is
// ever in, and would then stop polling — leaving somebody on the form they had just
// submitted while the server quietly finished the work behind them.
func (s *server) beganMigrating(w http.ResponseWriter, r *http.Request, started bool) {
	if !started {
		http.Error(w, "a migration is already running", http.StatusConflict)
		return
	}
	writeStatus(w, r, http.StatusAccepted, s.migration.state())
}

// acceptMigration reads a request and checks it, answering the caller when it cannot.
func (s *server) acceptMigration(w http.ResponseWriter, r *http.Request,
	check func(MigrationRequest) string,
) (MigrationRequest, bool) {
	if s.migrator == nil {
		http.Error(w, "this server was started without the means to migrate anything", http.StatusNotImplemented)
		return MigrationRequest{}, false
	}

	var ask MigrationRequest
	if !readRequest(w, r, &ask) {
		return MigrationRequest{}, false
	}
	if missing := check(ask); missing != "" {
		http.Error(w, missing, http.StatusBadRequest)
		return MigrationRequest{}, false
	}
	return ask, true
}
