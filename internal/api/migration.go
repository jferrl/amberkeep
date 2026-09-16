package api

import (
	"context"
	"sync"

	"github.com/jferrl/amberkeep/internal/guide"
	"github.com/jferrl/amberkeep/internal/migrate"
)

// Moving a history onto a phone, from a browser.
//
// The same thing the command does, arranged for somebody who would not open a
// terminal — which is most of the people this exists for. The shape follows the
// import wizard: work runs in the background, one thing says what is happening, and
// the page polls that one thing.
//
// What is different is that this one ends with somebody restoring a backup onto a
// phone they depend on. So it will not move from one stage to the next on its own.
// Checking does not lead to planning, and planning does not lead to writing: each is
// asked for, and the last one needs a word typed out, because a button somebody
// clicked through without reading is exactly what this must not have.

// MigrationStage is how far along a migration is.
type MigrationStage string

// The stages. Nothing moves between them without being asked.
const (
	// MigrationIdle is nothing started.
	MigrationIdle MigrationStage = "idle"
	// MigrationChecking is looking at the backup.
	MigrationChecking MigrationStage = "checking"
	// MigrationChecked is the checks done, waiting to be asked for a plan.
	MigrationChecked MigrationStage = "checked"
	// MigrationPlanning is working out what would move.
	MigrationPlanning MigrationStage = "planning"
	// MigrationPlanned is a plan to read, waiting for somebody to agree to it.
	MigrationPlanned MigrationStage = "planned"
	// MigrationWorking is writing.
	MigrationWorking MigrationStage = "working"
	// MigrationDone is a backup on disk, and a phone that has not been touched.
	MigrationDone MigrationStage = "done"
	// MigrationFailed is what went wrong, and what to do about it.
	MigrationFailed MigrationStage = "failed"
)

// Migrator does the work the migration asks for.
//
// The seam, as with importing: this package knows what to ask and when to say so. It
// does not know how a plan is made, what an iPhone store looks like inside, or how a
// backup is copied. That lives where those packages already meet.
type Migrator interface {
	// Check looks at a backup and reports what can be checked before anything else.
	Check(ctx context.Context, backup string) (migrate.Readiness, error)

	// Plan works out what would move, writing nothing.
	Plan(ctx context.Context, ask MigrationRequest, say Progress) (migrate.Plan, error)

	// Carry out the plan: into a copy of the store, checked, then into a copy of the
	// backup. Nothing it touches is an original.
	Carry(ctx context.Context, ask MigrationRequest, plan migrate.Plan, say Progress) (Migrated, error)
}

// MigrationRequest is everything a migration needs to be told.
type MigrationRequest struct {
	// Backup is the iPhone backup folder.
	Backup string `json:"backup"`
	// Android is the decrypted Android database.
	Android string `json:"android"`
	// Contacts is an address book, so conversations arrive with names.
	Contacts string `json:"contacts,omitempty"`
	// Pairing is WhatsApp's LID.sqlite from the same backup. Without it, somebody
	// the two phones know by different names arrives twice.
	Pairing string `json:"pairing,omitempty"`
	// Into is where to write the changed backup.
	Into string `json:"into,omitempty"`

	// Groups and Hidden widen what is moved, and both default to off for reasons
	// the plan explains in its own warnings.
	Groups bool `json:"groups,omitempty"`
	Hidden bool `json:"hidden,omitempty"`
	// Only limits the migration to these conversations, which is how somebody tries
	// one before trusting the rest.
	Only []string `json:"only,omitempty"`
}

// Migrated is what a finished migration produced.
type Migrated struct {
	// Backup is the changed copy, ready for Finder to restore.
	Backup string `json:"backup"`
	// Added, Merged and Created are what happened.
	Added   int `json:"added"`
	Merged  int `json:"merged"`
	Created int `json:"created"`
	// Checks is how many consistency checks the result passed.
	Checks int `json:"checks"`
	// Files is how many files the copied backup holds, unchanged from the original.
	Files int `json:"files"`
}

// migration is where a migration has got to.
type migration struct {
	mu sync.RWMutex

	stage    MigrationStage
	step     Step
	said     Note
	guidance string

	ask      MigrationRequest
	checks   *migrate.Readiness
	plan     *migrate.Plan
	finished *Migrated
}

func newMigration() *migration { return &migration{stage: MigrationIdle} }

// busy reports whether something is already running, so a second request is turned
// away rather than two migrations writing to the same place.
func (m *migration) busy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stage == MigrationChecking || m.stage == MigrationPlanning || m.stage == MigrationWorking
}

// begin marks the start of a piece of work, and hands back the state it just set.
//
// The state comes back from here rather than being read afterwards because the work
// runs in a goroutine and a quick one is finished before the request that asked for
// it has answered. Read afterwards, the reply would say "checked" — a stage the page
// is not waiting for, at the moment it starts waiting — and the page would sit there
// while the answer it wanted had already gone past.
func (m *migration) begin(stage MigrationStage, step Step, said Note, ask MigrationRequest) (map[string]any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stage == MigrationChecking || m.stage == MigrationPlanning || m.stage == MigrationWorking {
		return nil, false
	}
	m.stage, m.step, m.said, m.guidance = stage, step, said, ""
	if ask.Backup != "" {
		m.ask = ask
	}
	return m.report(), true
}

// progress says what is happening now.
func (m *migration) progress(step Step, said Note) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.working() {
		return
	}
	m.step, m.said = step, said
}

// working reports whether something is running. The caller holds the lock.
func (m *migration) working() bool {
	return m.stage == MigrationChecking || m.stage == MigrationPlanning || m.stage == MigrationWorking
}

// checked records what the checks found. It never moves on by itself: somebody has
// to read them and ask for the next thing.
func (m *migration) checked(ready migrate.Readiness) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.checks = &ready
	m.stage, m.step, m.said = MigrationChecked, "", Note{}
}

// planned records what would move. Also a full stop: this is what somebody reads
// before agreeing to anything.
func (m *migration) planned(plan migrate.Plan) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.plan = &plan
	m.stage, m.step, m.said = MigrationPlanned, "", Note{}
}

// done records a finished migration.
func (m *migration) done(result Migrated) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.finished = &result
	m.stage, m.step, m.said = MigrationDone, "", Note{}
}

// failed records why something did not work, with the advice that goes with it.
func (m *migration) failed(detail, guidance string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stage, m.step = MigrationFailed, ""
	m.said, m.guidance = Note{Text: detail}, guidance
}

// forget goes back to the beginning, keeping nothing.
func (m *migration) forget() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stage, m.step, m.said, m.guidance = MigrationIdle, "", Note{}, ""
	m.ask, m.checks, m.plan, m.finished = MigrationRequest{}, nil, nil, nil
}

// request is what the migration was asked to do.
func (m *migration) request() MigrationRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ask
}

// agreed is the plan somebody read, or nothing when there is none to act on.
func (m *migration) agreed() (migrate.Plan, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.plan == nil || m.stage != MigrationPlanned {
		return migrate.Plan{}, false
	}
	return *m.plan, true
}

// state is what the page polls.
func (m *migration) state() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.report()
}

// report is state with the lock already held.
func (m *migration) report() map[string]any {
	out := map[string]any{"stage": string(m.stage)}
	if m.step != "" {
		out["step"] = string(m.step)
	}
	if m.said.Text != "" {
		out["detail"] = m.said.Text
	}
	if m.said.Name != "" {
		out["note"] = m.said.Name
	}
	if len(m.said.Values) > 0 {
		out["values"] = m.said.Values
	}
	if len(m.said.Counts) > 0 {
		out["counts"] = m.said.Counts
	}
	if m.guidance != "" {
		out["guidance"] = m.guidance
	}
	if m.ask.Backup != "" {
		out["backup"] = m.ask.Backup
	}
	if m.checks != nil {
		out["checks"] = m.checks
	}
	if m.plan != nil {
		out["plan"] = m.plan
	}
	if m.finished != nil {
		out["result"] = m.finished
	}
	return out
}

// guidance is the whole guide, shaped for a page.
//
// Sent rather than duplicated in the frontend, because it is the one place these
// words live and a copy in TypeScript would drift from the copy in Go the first time
// somebody corrected a sentence.
func guidance(lang guide.Language) []map[string]any {
	stages := []guide.Stage{guide.Before, guide.Restoring, guide.After, guide.Wrong}

	out := make([]map[string]any, 0, len(stages))
	for _, stage := range stages {
		steps := make([]map[string]any, 0, 8)
		for _, step := range guide.At(stage, lang) {
			one := map[string]any{"id": step.ID, "title": step.Title, "body": step.Body}
			if step.Expect != "" {
				one["expect"] = step.Expect
			}
			if step.Takes > 0 {
				one["minutes"] = int(step.Takes.Minutes())
			}
			if step.Critical {
				one["critical"] = true
			}
			steps = append(steps, one)
		}
		out = append(out, map[string]any{
			"stage": string(stage), "heading": guide.Heading(stage, lang), "steps": steps,
		})
	}
	return out
}
