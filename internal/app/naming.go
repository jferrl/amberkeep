package app

import (
	"path/filepath"
	"strings"
)

// Small decisions about names, shared by everything that puts a file somewhere.
//
// They live here rather than beside one command because both front doors make them
// and have to make them identically: a file the terminal would call one thing and
// the window another is a file somebody cannot find twice.

// FirstNonEmpty returns the first value with anything in it.
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// DefaultOutput picks a name beside the backup, so the simple case needs no
// second path.
func DefaultOutput(in string) string {
	base := filepath.Base(in)
	for _, suffix := range []string{".crypt15", ".crypt14", ".crypt12"} {
		if trimmed, found := strings.CutSuffix(base, suffix); found {
			base = trimmed
			break
		}
	}
	if base == filepath.Base(in) {
		base += ".decrypted"
	}
	return filepath.Join(filepath.Dir(in), base)
}
