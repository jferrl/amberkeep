package api

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"runtime"

	"github.com/jferrl/amberkeep/internal/phone"
)

// Reading a backup off the phone it is on.
//
// The wizard's fourth screen asks somebody to open a folder called
// Android/media/com.whatsapp/WhatsApp/Databases on the phone's own storage and copy
// a file out of it. That is a sentence a developer writes. The folder is hidden on
// most launchers, and the file next to the one they want is a fragment that looks
// almost identical and is useless alone — so getting it wrong costs them the twenty
// minutes the backup took, and they find out much later.
//
// The phone is usually plugged into the same computer. This asks it instead.

// Phone is a device this computer can see.
type Phone struct {
	Serial string `json:"serial"`
	Name   string `json:"name"`
	// Ready is whether it can be read yet. A phone that is plugged in but has not
	// had the prompt accepted is visible and useless.
	Ready   bool   `json:"ready"`
	Trouble string `json:"trouble,omitempty"`
}

// PhoneBackup is an encrypted message store on a phone.
type PhoneBackup struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	// Partial marks a fragment of a later backup: useless on its own, and sitting in
	// the same folder looking almost the same as the one that is not.
	Partial bool `json:"partial"`
}

// Phones is what this computer can see, and why it can see nothing when it cannot.
type Phones struct {
	Phones []Phone `json:"phones"`
	// Trouble is the reason there is no list rather than an empty one: usually that
	// the Android tools this borrows are not installed.
	Trouble string `json:"trouble,omitempty"`
	// Why is that reason as a code, so the page can phrase it and can tell the two
	// apart: tools that were never installed are a thing somebody can go and fix,
	// and tools that will not run are not fixed by installing them again.
	Why Why `json:"why,omitempty"`
	// Platform is which system this is, so the page can give the instruction that
	// belongs to it rather than three and a choice.
	Platform string `json:"platform,omitempty"`
}

// Why is the reason no phone can be seen.
type Why string

// The reasons.
const (
	// WhyNoTools is that adb was never installed.
	WhyNoTools Why = "no-tools"
	// WhyUnusable is that it is installed and will not run.
	WhyUnusable Why = "unusable"
)

// Reader talks to a phone over adb, and only reads it.
//
// A separate seam from Importer for the same reason Exportable is separate from
// Archive: a build without the Android tools is still a perfectly good program, and
// the endpoints answer 501 rather than the page offering something that cannot work.
type Reader interface {
	// Phones lists what this computer can see.
	Phones(ctx context.Context) ([]Phone, error)

	// Backups lists the message stores on one phone, and never writes to it.
	Backups(ctx context.Context, serial string) ([]PhoneBackup, error)

	// Fetch copies one off the phone into the workspace and reports where it landed.
	Fetch(ctx context.Context, serial, remote string, say Progress) (string, error)

	// Files says what the phone's WhatsApp folder holds, without copying any of it.
	// Asked before anything is offered, because the answer on a real device is
	// several gigabytes and that is not a thing to start without saying so.
	Files(ctx context.Context, serial string) (PhoneMedia, error)

	// FetchFiles copies that folder into a directory here and reports where it
	// landed. It is the long one: minutes to an hour, depending on the phone.
	FetchFiles(ctx context.Context, serial, into string, say Progress) (string, error)
}

// PhoneMedia is the phone's folder of photographs, videos and recordings.
//
// A message database records where every one of them was and holds at most a
// thumbnail, so this folder is the difference between an archive that shows about
// one picture in eight and one that shows all of them.
type PhoneMedia struct {
	// Path is where it is on the phone, for a screen that wants to name it.
	Path string `json:"path"`
	// Bytes is how much is in it, as the phone counts it. Zero when the phone would
	// not say, which makes the wait unpredictable rather than impossible.
	Bytes int64 `json:"bytes"`
	// Kinds are the folders inside it: WhatsApp Images, WhatsApp Voice Notes, and so
	// on. How many there are is how the copying reports its progress.
	Kinds []string `json:"kinds"`
}

// alsoTheFiles copies the phone's photographs in beside the database that was just
// decrypted, so the archive that opens next finds them without being told.
//
// A failure here is said and not raised. The messages are already out by this point,
// and somebody whose photographs would not copy should be reading their conversations
// while they work out why rather than being handed nothing at all.
func (s *server) alsoTheFiles(ctx context.Context, serial, db string, say Progress) {
	if _, err := s.phones.FetchFiles(ctx, serial, filepath.Dir(db), say); err != nil {
		say(StepFetching, Noted("photographsDidNotCopy",
			"The messages are out, but the photographs would not copy: {why}",
			"why", sentence(err)))
	}
}

// handlePhoneFiles says what the phone's WhatsApp folder holds, before anybody
// commits to waiting for it.
func (s *server) handlePhoneFiles(w http.ResponseWriter, r *http.Request) {
	if s.phones == nil {
		http.Error(w, "this server was started without the means to read a phone", http.StatusNotImplemented)
		return
	}

	found, err := s.phones.Files(r.Context(), r.PathValue("serial"))
	if err != nil {
		// Not an error to the page: a phone with no such folder is an ordinary
		// phone, and the screen says so rather than offering something that cannot
		// work. The sentence goes with it, because "no photographs" and "this build
		// does not know where your phone keeps them" read very differently.
		write(w, r, PhoneMedia{Kinds: []string{}, Path: sentence(err)})
		return
	}
	if found.Kinds == nil {
		found.Kinds = []string{}
	}
	write(w, r, found)
}

// handlePhones says what is plugged in.
func (s *server) handlePhones(w http.ResponseWriter, r *http.Request) {
	if s.phones == nil {
		http.Error(w, "this server was started without the means to read a phone", http.StatusNotImplemented)
		return
	}

	found, err := s.phones.Phones(r.Context())
	if err != nil {
		// Not an error to the page. A computer with no Android tools installed is an
		// ordinary computer, and the screen says so and offers the other way in.
		why := WhyUnusable
		if errors.Is(err, phone.ErrNoADB) {
			why = WhyNoTools
		}
		write(w, r, Phones{
			Phones: []Phone{}, Trouble: sentence(err), Why: why, Platform: runtime.GOOS,
		})
		return
	}
	if found == nil {
		found = []Phone{}
	}
	write(w, r, Phones{Phones: found})
}

// handlePhoneBackups says what is on one phone.
func (s *server) handlePhoneBackups(w http.ResponseWriter, r *http.Request) {
	if s.phones == nil {
		http.Error(w, "this server was started without the means to read a phone", http.StatusNotImplemented)
		return
	}

	serial := r.PathValue("serial")
	found, err := s.phones.Backups(r.Context(), serial)
	if err != nil {
		http.Error(w, sentence(err), http.StatusBadRequest)
		return
	}
	if found == nil {
		found = []PhoneBackup{}
	}
	write(w, r, map[string]any{"backups": found})
}

// handleFetch copies a backup off the phone and then unlocks it, which is the whole
// point: the file on its own is still encrypted, and a person who wanted a file
// wanted their messages.
func (s *server) handleFetch(w http.ResponseWriter, r *http.Request) {
	if s.phones == nil || s.importer == nil {
		http.Error(w, "this server was started without the means to read a phone", http.StatusNotImplemented)
		return
	}

	var ask struct {
		Serial string `json:"serial"`
		Path   string `json:"path"`
		Key    string `json:"key"`
		Into   string `json:"into"`
		// Media copies the phone's photographs across as well. Several gigabytes and
		// the difference between an archive of text and an archive of somebody's
		// life, so it is asked for rather than assumed either way.
		Media bool `json:"media"`
	}
	if !readRequest(w, r, &ask) {
		return
	}
	switch {
	case ask.Serial == "":
		http.Error(w, "which phone is needed", http.StatusBadRequest)
		return
	case ask.Path == "":
		http.Error(w, "which backup is needed", http.StatusBadRequest)
		return
	case ask.Key == "":
		http.Error(w, "the key is needed: a backup is no use without it", http.StatusBadRequest)
		return
	}

	// The work outlives the request, as every long piece of work here does: this is
	// minutes of copying and somebody who closes the tab should come back to a
	// finished file rather than half of one. start hands it the background context.
	//nolint:contextcheck // deliberate; see above
	started := s.start(StepFetching, Saying("copyingOffPhone", "Copying the backup off the phone."),
		func(ctx context.Context, say Progress) (string, error) {
			file, err := s.phones.Fetch(ctx, ask.Serial, ask.Path, say)
			if err != nil {
				return "", err
			}
			// The key never reaches this package's memory for longer than the
			// request that carried it, and never reaches a log or a file.
			db, err := s.importer.Decrypt(ctx, file, ask.Key, ask.Into, say)
			if err != nil {
				return "", err
			}
			if ask.Media {
				s.alsoTheFiles(ctx, ask.Serial, db, say)
			}
			return db, nil
		}, Opening{})

	s.begun(w, r, started)
}
