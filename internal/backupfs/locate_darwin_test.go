//go:build darwin

package backupfs

import (
	"strings"
	"testing"
)

// TestCanonicalDirsOnDarwin protects the one macOS-specific fact this package
// depends on: Finder, iTunes and the Apple Devices app all write backups under
// ~/Library/Application Support/MobileSync/Backup.
func TestCanonicalDirsOnDarwin(t *testing.T) {
	t.Parallel()

	dirs := canonicalDirs()
	if len(dirs) != 1 {
		t.Fatalf("canonicalDirs() = %v, want exactly one macOS location", dirs)
	}
	if !strings.HasSuffix(dirs[0], "Library/Application Support/MobileSync/Backup") {
		t.Errorf("canonicalDirs()[0] = %q, want it to end in .../MobileSync/Backup", dirs[0])
	}
}
