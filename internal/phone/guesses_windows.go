//go:build windows

package phone

import (
	"os"
	"path/filepath"
)

// guesses are where the Android tools end up on Windows when somebody installed
// Android Studio and never added it to PATH.
func guesses() []string {
	var out []string
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		out = append(out, filepath.Join(local, "Android", "Sdk", "platform-tools", "adb.exe"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, "AppData", "Local", "Android", "Sdk", "platform-tools", "adb.exe"))
	}
	return out
}
