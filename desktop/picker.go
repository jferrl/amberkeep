package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Picker is the part of this program a browser cannot be.
//
// Every path the wizard needs — a backup folder, a decrypted database, WhatsApp's
// pairing file — is something a person has to find on their own disk. In a browser
// the only honest way to ask is a text field, and the sentence that goes with it
// begins "the folder named after a long string of letters and numbers". Here it is a
// button that opens the picker they already know.
//
// The page reaches these through Wails' generated bindings and feature-detects them:
// in a browser they are simply absent and the text field stays. Neither side has a
// second implementation of anything.
type Picker struct {
	// ctx is the running application, which the dialogs need in order to sit over
	// the right window. It arrives at startup rather than at construction because
	// Wails only has it once there is a window to be modal to.
	ctx context.Context
}

// opened records the running application. Wails calls it once, before the window
// appears.
func (p *Picker) opened(ctx context.Context) { p.ctx = ctx }

// ChooseFolder asks for a directory and returns where it is.
//
// An empty string means somebody changed their mind, which is not a failure and is
// not reported as one: the page simply leaves the field as it was.
func (p *Picker) ChooseFolder(title string) string {
	if p.ctx == nil {
		return ""
	}
	chosen, err := runtime.OpenDirectoryDialog(p.ctx, runtime.OpenDialogOptions{
		Title: title,
		// Apple keeps backups inside Library, which the picker hides by default, so
		// somebody looking for one would be shown a folder that appears to be empty.
		ShowHiddenFiles: true,
	})
	if err != nil {
		return ""
	}
	return chosen
}

// ChooseFile asks for one file and returns where it is.
//
// The filters are suggestions rather than restrictions: every one of them offers
// "any file" as well, because a database somebody has already renamed is still the
// database they want and a picker that refuses to show it is a dead end.
func (p *Picker) ChooseFile(title, kind string) string {
	if p.ctx == nil {
		return ""
	}
	chosen, err := runtime.OpenFileDialog(p.ctx, runtime.OpenDialogOptions{
		Title:           title,
		Filters:         filtersFor(kind),
		ShowHiddenFiles: true,
	})
	if err != nil {
		return ""
	}
	return chosen
}

// filtersFor is what the picker offers to show, by the kind of thing being asked for.
func filtersFor(kind string) []runtime.FileFilter {
	anything := runtime.FileFilter{DisplayName: "Any file", Pattern: "*.*"}

	switch kind {
	case "database":
		return []runtime.FileFilter{
			{DisplayName: "Message databases (*.db, *.sqlite)", Pattern: "*.db;*.sqlite"},
			anything,
		}
	case "encrypted":
		return []runtime.FileFilter{
			{DisplayName: "Encrypted WhatsApp backups (*.crypt15)", Pattern: "*.crypt15;*.crypt14;*.crypt12"},
			anything,
		}
	case "contacts":
		return []runtime.FileFilter{
			{DisplayName: "Address books (*.vcf)", Pattern: "*.vcf"},
			anything,
		}
	default:
		return []runtime.FileFilter{anything}
	}
}
