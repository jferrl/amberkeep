package api

import (
	"context"
	"sync"

	"github.com/jferrl/amberkeep/internal/export"
	"github.com/jferrl/amberkeep/internal/model"
	"github.com/jferrl/amberkeep/internal/search"
)

// What the program is doing, and whether there is anything to read yet.
//
// Until now the server was handed an archive before it started and served that one
// archive until it stopped. It has to be able to start with nothing, because the
// person this is for does not have a readable archive: they have a phone that died,
// or a backup they have never heard of, and getting from there to an archive is the
// part they need help with.

// Stage is what the server is doing.
type Stage string

// The stages, in the order somebody passes through them.
const (
	// StageEmpty means no archive is open and the wizard is what to show.
	StageEmpty Stage = "empty"
	// StageWorking means something is being decrypted, extracted or opened.
	StageWorking Stage = "working"
	// StageReady means there is an archive to read.
	StageReady Stage = "ready"
	// StageFailed means the last attempt did not work, and why is worth saying.
	StageFailed Stage = "failed"
)

// Step names the part of the work that is running, for a person watching it.
type Step string

// The steps. They are separate from the stage because "working" tells somebody
// nothing and "decrypting" tells them what to expect.
const (
	StepExtracting Step = "extracting"
	StepDecrypting Step = "decrypting"
	StepPreparing  Step = "preparing"
	StepIndexing   Step = "indexing"
	StepOpening    Step = "opening"
	// StepWriting is producing files from an archive that is already open.
	StepWriting Step = "writing"
	// StepFetching is copying a backup off a phone that is plugged in.
	StepFetching Step = "fetching"
)

// Progress is how work already running says what it is doing.
//
// Decrypting a backup and indexing a million messages take minutes, and a person
// watching a bar that does not move assumes the program has hung. The two long
// operations therefore report as they go, and the only thing the page polls is the
// state this writes into.
type Progress func(step Step, detail string, counts ...Count)

// Count is a number the work has reached, for a page to phrase itself.
//
// The sentence used to be built here and sent as prose, which made every progress
// line English — and unformatted, so a Spanish reader watching an index build was
// told about "595236 messages" rather than 595.236. The server knows the number and
// the page knows the reader, so the number travels and the sentence is composed
// where the language is.
//
// The prose is still sent beside it, because the command prints exactly that and a
// page meeting a count it does not recognise should say something rather than
// nothing.
type Count struct {
	// Of is what was counted: "conversations", "messages".
	Of string `json:"of"`
	N  int    `json:"n"`
}

// opened is an archive and everything derived from it that is needed on every
// request.
type opened struct {
	archive Archive
	chats   []model.Chat
	byJID   map[string]model.Chat
	export  export.Options

	// index is the full-text index, when this archive came with one. Searching says
	// it is unavailable rather than returning nothing when it did not.
	index *search.Index
}

// close releases the archive.
//
// Only the archive: whatever it brought with it, an index included, is its own to
// let go of, and an index handed in separately at startup belongs to whoever handed
// it in. The failure is not worth reporting, because by this point the thing being
// closed is already out of reach of anybody who could act on it.
func (o *opened) close() {
	if o == nil || o.archive == nil {
		return
	}
	_ = o.archive.Close()
}

// session is what the server currently holds.
//
// Every field is guarded, because the work runs in its own goroutine while requests
// keep arriving: a page polls to watch progress, and the archive it is waiting for
// appears underneath it.
type session struct {
	mu sync.RWMutex

	stage    Stage
	step     Step
	detail   string
	guidance string
	counts   []Count

	// workspace is where anything this program writes will go. It is shown before
	// anything is written, because somebody has to be able to find their archive
	// afterwards.
	workspace string

	open *opened
}

// newSession returns a session holding whatever was passed at startup.
func newSession(workspace string, open *opened) *session {
	s := &session{workspace: workspace, open: open, stage: StageEmpty}
	if open != nil {
		s.stage = StageReady
	}
	return s
}

// archive returns what is open, or nothing when the wizard has not finished.
func (s *session) archive() *opened {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.open
}

// begin marks the start of a piece of work, and refuses when one is already
// running.
//
// Whether something is running is not asked separately anywhere, on purpose: a
// caller that looked first and started afterwards would leave a gap for a second
// request to land in, and two decryptions writing to the same file would ruin both.
func (s *session) begin(step Step, detail string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stage == StageWorking {
		return false
	}
	s.stage, s.step, s.detail, s.guidance = StageWorking, step, detail, ""
	return true
}

// progress says what is happening now, without changing what stage it is in.
func (s *session) progress(step Step, detail string, counts ...Count) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stage != StageWorking {
		return
	}
	s.step, s.detail, s.counts = step, detail, counts
}

// ready hands over a finished archive, and lets go of whatever was open before.
//
// Closing the previous one matters because the wizard can be used more than once:
// somebody trying two backups in turn would otherwise leave a database open on each.
// Closing waits for anything already reading it, so a request in flight finishes
// rather than being cut off.
func (s *session) ready(open *opened) {
	s.mu.Lock()
	previous := s.open
	s.open = open
	s.stage, s.step, s.detail, s.guidance = StageReady, "", "", ""
	s.mu.Unlock()

	previous.close()
}

// failed records why something did not work, along with whatever advice goes with
// it. Nobody should be left at a dead end with a sentence they cannot act on.
func (s *session) failed(detail, guidance string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stage, s.step = StageFailed, ""
	s.detail, s.guidance = detail, guidance
}

// close forgets the archive and goes back to the beginning.
func (s *session) close() {
	s.mu.Lock()
	previous := s.open
	s.open = nil
	s.stage, s.step, s.detail, s.guidance = StageEmpty, "", "", ""
	s.mu.Unlock()

	previous.close()
}

// setWorkspace records where the person asked for things to be written.
func (s *session) setWorkspace(where string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspace = where
}

// state is what the page is told, and the only thing it polls.
func (s *session) state() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := map[string]any{"stage": string(s.stage), "workspace": s.workspace}
	if s.step != "" {
		out["step"] = string(s.step)
	}
	if s.detail != "" {
		out["detail"] = s.detail
	}
	if s.guidance != "" {
		out["guidance"] = s.guidance
	}
	if len(s.counts) > 0 {
		out["counts"] = s.counts
	}
	return out
}

// Importer does the work the wizard asks for.
//
// The server knows what to ask and when to say so. It does not know how a backup is
// decrypted, where Apple keeps one, or which of two databases it has been handed:
// that lives where those packages already meet, which is the command rather than
// the web layer. This interface is the seam.
type Importer interface {
	// Backups lists the iPhone backups on this computer. A problem reaching them,
	// such as a permission this program has not been granted, is returned as a
	// sentence rather than an error, because an empty list and a locked folder mean
	// very different things to somebody looking at the screen.
	Backups() (backups []Backup, problem string)

	// Locations reports where this platform's Apple software puts backups. Empty
	// means there is nowhere to look, not that nothing was found: Apple ships no
	// Finder, iTunes or Apple Devices for Linux, so a page told only that the list
	// was empty would go on to explain how to make one in Finder.
	Locations() []string

	// Open reads an archive that is already readable, working out for itself which
	// kind it is. Building the search index happens here, which is why it reports
	// progress: it is the longest wait in the whole wizard.
	Open(ctx context.Context, path, contacts string, say Progress) (Archive, error)

	// Extract takes the message store and its pictures out of an iPhone backup and
	// returns where the store landed.
	Extract(ctx context.Context, backup, into string, say Progress) (string, error)

	// Decrypt turns an encrypted Android backup into a readable database and
	// returns where it landed. The key is never logged, stored or echoed.
	Decrypt(ctx context.Context, file, key, into string, say Progress) (string, error)
}

// Backup is one iPhone backup, as the wizard lists it.
//
// Everything but the path and whether it is encrypted is omitted when empty, which
// is about what an old backup knows rather than about what this program sends:
// Apple's index records a device name, a model, a version and a date, and a backup
// written years ago by an old iTunes is missing half of them. Encryption is not
// omitted, because it decides whether the backup can be used at all and silence is
// a poor way to say "no".
//
// LastBackup is RFC 3339 in universal time rather than a date already formatted.
// Whoever shows it knows the reader's language and time zone; this does not.
type Backup struct {
	Path        string `json:"path"`
	DeviceName  string `json:"device_name,omitempty"`
	ProductType string `json:"product_type,omitempty"`
	IOSVersion  string `json:"ios_version,omitempty"`
	LastBackup  string `json:"last_backup,omitempty"`
	Encrypted   bool   `json:"encrypted"`
}
