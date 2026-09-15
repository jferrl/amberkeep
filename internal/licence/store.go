package licence

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Where a key is kept.
//
// One file, in the place the operating system keeps a program's settings, holding one
// line. Not in the workspace beside somebody's archive: an archive folder gets copied
// to an external disk and handed to a relative, and a licence key is the one thing in
// this program that belongs to the buyer rather than to the archive.
//
// It is written with owner-only permissions, like everything else this program
// writes. Not because a licence key is a secret worth much — it unlocks an export,
// not a bank — but because the rule here is that nothing this program writes is
// readable by everybody, and a rule with an exception is not a rule.

// ErrNoLicence is that there is no key on this computer.
var ErrNoLicence = errors.New("there is no licence on this computer")

// kept is the file a key lives in.
func kept() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding where settings are kept: %w", err)
	}
	return filepath.Join(dir, "amberkeep", "licence"), nil
}

// Held returns the licence on this computer, if there is one.
func Held() (Licence, error) {
	path, err := kept()
	if err != nil {
		return Licence{}, err
	}

	// #nosec G304 -- the path is this program's own settings file, not a caller's.
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Licence{}, ErrNoLicence
	case err != nil:
		return Licence{}, fmt.Errorf("reading the licence: %w", err)
	}

	key := strings.TrimSpace(string(raw))
	if key == "" {
		return Licence{}, ErrNoLicence
	}
	return Read(key)
}

// Keep writes a key to this computer, having checked that it is one.
//
// The licence it says is returned, so that whatever asked can say what was bought
// rather than only that something was.
func Keep(key string) (Licence, error) {
	licence, err := Read(key)
	if err != nil {
		return Licence{}, err
	}

	path, err := kept()
	if err != nil {
		return Licence{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Licence{}, fmt.Errorf("making somewhere to keep the licence: %w", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(key)+"\n"), 0o600); err != nil {
		return Licence{}, fmt.Errorf("writing the licence: %w", err)
	}
	return licence, nil
}

// Forget removes the key from this computer, for somebody moving to another one.
//
// No key to remove is not a failure: the end state is the same either way, and
// telling somebody off for tidying something that was already tidy helps nobody.
func Forget() error {
	path, err := kept()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing the licence: %w", err)
	}
	return nil
}

// Where says which file a key would be kept in, for a program that has to tell
// somebody where to look.
func Where() string {
	path, err := kept()
	if err != nil {
		return ""
	}
	return path
}
