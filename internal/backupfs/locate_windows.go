//go:build windows

package backupfs

import (
	"os"
	"path/filepath"
)

// canonicalDirs returns where Windows stores device backups. The Apple Devices app,
// and the Microsoft Store build of iTunes, use %USERPROFILE%\Apple\MobileSync\Backup;
// the classic iTunes installer from apple.com uses
// %APPDATA%\Apple Computer\MobileSync\Backup. Both are returned because either, both
// or neither may be installed on a given machine, and Backups looks in whichever
// exist.
func canonicalDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "Apple", "MobileSync", "Backup"))
	}
	if appData := os.Getenv("APPDATA"); appData != "" {
		dirs = append(dirs, filepath.Join(appData, "Apple Computer", "MobileSync", "Backup"))
	}
	return dirs
}
