//go:build darwin

package backupfs

import (
	"os"
	"path/filepath"
)

// canonicalDirs returns where Finder, the Apple Devices app and iTunes on macOS store
// device backups: there is exactly one location, and reading it needs Full Disk
// Access (see ErrPermissionDenied).
func canonicalDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(home, "Library", "Application Support", "MobileSync", "Backup")}
}
