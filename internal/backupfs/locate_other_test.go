//go:build !darwin && !windows

package backupfs

import "testing"

// TestCanonicalDirsOnOtherPlatforms protects the deliberate no-op: Linux and every
// other platform have no canonical Apple backup location, so Backups should simply
// find nothing there, not fail.
func TestCanonicalDirsOnOtherPlatforms(t *testing.T) {
	t.Parallel()

	if dirs := canonicalDirs(); dirs != nil {
		t.Errorf("canonicalDirs() = %v, want nil", dirs)
	}
}
