//go:build !windows

package phone

import (
	"os"
	"path/filepath"
)

// guesses are where adb ends up, in the order it is most likely to be.
//
// Two kinds of place. The first is where a package manager or Android Studio puts
// it, which covers anybody who already had the tooling. The second is where a person
// following this program's own instructions puts it: they are told to download a zip
// and unzip it, and nobody unzips a thing into ~/Library/Android/sdk. They unzip it
// into Downloads, or Applications, or the desktop.
//
// Telling somebody to put a folder somewhere and then not looking there is the kind
// of promise that makes a program feel broken. These are the places the instructions
// name, and a couple either side of them.
func guesses() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var out []string
	// Already had it.
	out = append(out,
		"/opt/homebrew/bin/adb",
		"/usr/local/bin/adb",
		filepath.Join(home, "Library", "Android", "sdk", "platform-tools", "adb"),
		filepath.Join(home, "Android", "Sdk", "platform-tools", "adb"),
	)
	// Unzipped it, the way this program asks.
	for _, dir := range []string{
		filepath.Join(home, "Applications"),
		"/Applications",
		filepath.Join(home, "Downloads"),
		filepath.Join(home, "Desktop"),
		home,
	} {
		out = append(out, filepath.Join(dir, "platform-tools", "adb"))
	}
	return out
}
