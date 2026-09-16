package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/jferrl/amberkeep/internal/api"
	"github.com/jferrl/amberkeep/internal/phone"
)

// Reading a backup off the phone it is on.
//
// This is the seam between the page, which knows what to ask, and internal/phone,
// which knows how to ask a device and refuses to do anything but read it. Nothing
// here composes a command: it passes a device identifier and a path that the package
// underneath checks against the two folders WhatsApp actually uses.

// Phones reads Android phones over adb, when this computer has adb at all.
type Phones struct {
	// Workspace is where a backup copied off a phone lands. A caller-supplied
	// directory this program already owns, never somewhere the phone chose.
	Workspace string
}

// Phones lists what this computer can see.
func (p Phones) Phones(ctx context.Context) ([]api.Phone, error) {
	adb, err := phone.Find()
	if err != nil {
		return nil, err
	}

	found, err := adb.Phones(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]api.Phone, 0, len(found))
	for _, one := range found {
		out = append(out, api.Phone{
			Serial: one.Serial, Name: one.Name, Ready: one.Ready, Trouble: one.Trouble,
		})
	}
	return out, nil
}

// Backups lists the message stores on one phone, newest and largest first.
//
// The order matters more than it looks. The file somebody wants is the whole backup,
// and the ones beside it are fragments of a later one — smaller, newer, and useless
// alone. Putting the largest first puts the right answer at the top.
func (p Phones) Backups(ctx context.Context, serial string) ([]api.PhoneBackup, error) {
	adb, err := phone.Find()
	if err != nil {
		return nil, err
	}

	found, err := adb.Backups(ctx, serial)
	if err != nil {
		return nil, err
	}

	out := make([]api.PhoneBackup, 0, len(found))
	for _, one := range found {
		out = append(out, api.PhoneBackup{
			Path: one.Path, Name: one.Name, Size: one.Size, Partial: one.Partial,
		})
	}
	// Whole backups before fragments, then largest first.
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if betterFirst(out[j], out[i]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// betterFirst reports whether a should be offered before b.
func betterFirst(a, b api.PhoneBackup) bool {
	if a.Partial != b.Partial {
		return !a.Partial
	}
	return a.Size > b.Size
}

// Fetch copies one backup off the phone into the workspace.
func (p Phones) Fetch(ctx context.Context, serial, remote string, say api.Progress) (string, error) {
	adb, err := phone.Find()
	if err != nil {
		return "", err
	}

	// The same place everything else this program writes goes, when the caller did
	// not name one. Two answers to "where did my file go" is one too many.
	into := p.Workspace
	if into == "" {
		into = api.Workspace()
	}
	if into == "" {
		return "", fmt.Errorf("there is nowhere to put the backup")
	}
	// A folder of its own inside the workspace, so what came off a phone is
	// distinguishable from what somebody put there.
	landing := filepath.Join(into, "phone")
	if err := os.MkdirAll(landing, 0o750); err != nil {
		return "", fmt.Errorf("making somewhere to put the backup: %w", err)
	}

	say(api.StepFetching, api.Saying("copyingOffPhoneLong",
		"Copying the backup off the phone. This takes a few minutes."))
	return adb.Fetch(ctx, serial, remote, landing, func(line string) {
		if line != "" {
			// A line from adb itself: a percentage and a path, with no sentence in
			// it to translate.
			say(api.StepFetching, api.Quoting(line))
		}
	})
}

// Files says what the phone's WhatsApp folder holds, without copying any of it.
//
// A real device holds 5.7 GB here. Asking first is what lets a screen say "this will
// take twenty minutes and use six gigabytes" instead of starting and hoping.
func (p Phones) Files(ctx context.Context, serial string) (api.PhoneMedia, error) {
	adb, err := phone.Find()
	if err != nil {
		return api.PhoneMedia{}, err
	}

	found, err := adb.Files(ctx, serial)
	if err != nil {
		return api.PhoneMedia{}, err
	}
	return api.PhoneMedia{Path: found.Path, Bytes: found.Bytes, Kinds: found.Kinds}, nil
}

// FetchFiles copies the phone's photographs, videos and recordings into a folder.
//
// `into` is where the decrypted database already is, so that what lands beside it is
// the `Media` folder the paths inside that database already point at, and the archive
// finds it with nothing else done.
func (p Phones) FetchFiles(ctx context.Context, serial, into string, say api.Progress) (string, error) {
	adb, err := phone.Find()
	if err != nil {
		return "", err
	}
	if into == "" {
		return "", fmt.Errorf("there is nowhere to put the photographs")
	}

	say(api.StepFetching, api.Saying("copyingPhotographs",
		"Copying the photographs off the phone. This is the long part."))

	return adb.FetchFiles(ctx, serial, into, func(kind string, at, total int) {
		// Which part is copying, and how far along: several gigabytes in silence is
		// indistinguishable from a program that has stopped.
		say(api.StepFetching, api.Noted("copyingKind",
			"Copying {kind} — {done} of {kinds}.",
			"kind", kind, "done", strconv.Itoa(at+1), "kinds", strconv.Itoa(total)).
			Counting("done", at+1).
			Counting("kinds", total))
	})
}
