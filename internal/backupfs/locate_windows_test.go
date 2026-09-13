//go:build windows

package backupfs

import (
	"strings"
	"testing"
)

// TestCanonicalDirsOnWindows protects the two Windows-specific locations this
// package depends on: the Apple Devices app (and Microsoft Store iTunes) under
// %USERPROFILE%\Apple\MobileSync\Backup, and classic iTunes under
// %APPDATA%\Apple Computer\MobileSync\Backup.
func TestCanonicalDirsOnWindows(t *testing.T) {
	t.Parallel()

	dirs := canonicalDirs()
	if len(dirs) == 0 {
		t.Fatal("canonicalDirs() returned no locations")
	}
	for _, d := range dirs {
		if !strings.Contains(d, "MobileSync\\Backup") {
			t.Errorf("canonicalDirs() entry %q does not end in MobileSync\\Backup", d)
		}
	}
}
