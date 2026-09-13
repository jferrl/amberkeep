//go:build !darwin && !windows

package backupfs

// canonicalDirs reports no locations on platforms none of Finder, iTunes or the
// Apple Devices app target. Open still works against any folder the caller names,
// such as one copied over from a Mac or Windows machine.
func canonicalDirs() []string { return nil }
