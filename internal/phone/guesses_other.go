//go:build !windows

package phone

import (
	"os"
	"path/filepath"
)

// guesses are where the Android tools end up when somebody installed Android Studio
// and never touched a shell profile, which is most people who have adb at all.
func guesses() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, "Library", "Android", "sdk", "platform-tools", "adb"),
		filepath.Join(home, "Android", "Sdk", "platform-tools", "adb"),
		"/opt/homebrew/bin/adb",
		"/usr/local/bin/adb",
	}
}
