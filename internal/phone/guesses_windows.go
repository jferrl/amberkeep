//go:build windows

package phone

import (
	"os"
	"path/filepath"
)

// guesses are where adb ends up, in the order it is most likely to be.
//
// Two kinds of place. The first is where a package manager or Android Studio puts
// it. The second is where a person following this program's own instructions puts
// it: they are told to download a zip and unzip it, and nobody unzips a thing into
// AppData\Local\Android\Sdk. They unzip it into Downloads, or the desktop, or the
// root of the C: drive.
//
// Telling somebody to put a folder somewhere and then not looking there is the kind
// of promise that makes a program feel broken.
func guesses() []string {
	var out []string

	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		out = append(out,
			filepath.Join(local, "Android", "Sdk", "platform-tools", "adb.exe"),
			// Where winget puts a package's links.
			filepath.Join(local, "Microsoft", "WinGet", "Links", "adb.exe"),
		)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return out
	}
	for _, dir := range []string{
		filepath.Join(home, "Downloads"),
		filepath.Join(home, "Desktop"),
		home,
		`C:\`,
	} {
		out = append(out, filepath.Join(dir, "platform-tools", "adb.exe"))
	}
	return out
}
