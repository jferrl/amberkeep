package backupfs

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Against a real backup, which is several gigabytes and thousands of files.
//
// Nothing here runs without being pointed at one. What is checked is the property
// that matters and that a synthetic fixture cannot really test: that out of tens of
// thousands of files, exactly the two that were supposed to change did.
//
//	AMBERKEEP_REAL_BACKUP    a backup folder, the one named after a device identifier
//	AMBERKEEP_REAL_STORE_IN  the database to put into it
func TestPatchingARealBackupChangesTwoFiles(t *testing.T) {
	from := os.Getenv("AMBERKEEP_REAL_BACKUP")
	with := os.Getenv("AMBERKEEP_REAL_STORE_IN")
	if from == "" || with == "" {
		t.Skip("set AMBERKEEP_REAL_BACKUP and AMBERKEEP_REAL_STORE_IN to run this")
	}

	before := fingerprint(t, from)
	t.Logf("the backup holds %d files", len(before))

	into := filepath.Join(t.TempDir(), "patched")
	started := time.Now()
	result, err := Patch(t.Context(), from, into,
		Replacement{
			Domain:       "AppDomainGroup-group.net.whatsapp.WhatsApp.shared",
			RelativePath: "ChatStorage.sqlite",
			With:         with,
		})
	if err != nil {
		t.Fatalf("Patch() failed: %v", err)
	}
	t.Logf("copied and patched in %s: the store went from %s to %s, log neutralised: %v",
		time.Since(started).Round(time.Second), megabytes(result.Was), megabytes(result.Now), result.LogNeutralised)

	after := fingerprint(t, into)

	// The original is where it was, as it was.
	if now := fingerprint(t, from); !same(before, now) {
		t.Error("the original backup changed")
	}

	if len(after) != len(before) {
		t.Errorf("the copy holds %d files and the original holds %d", len(after), len(before))
	}

	var changed []string
	for name, was := range before {
		if now, still := after[name]; !still {
			changed = append(changed, name+" (gone)")
		} else if now != was {
			changed = append(changed, name)
		}
	}

	// Exactly two: the file that was replaced, and the index that records its size.
	if len(changed) != 2 {
		t.Fatalf("%d files differ, want the store and the index: %v", len(changed), changed)
	}
	var sawIndex bool
	for _, name := range changed {
		if filepath.Base(name) == "Manifest.db" {
			sawIndex = true
		}
	}
	if !sawIndex {
		t.Errorf("the index is not among what changed: %v", changed)
	}
}

// fingerprint is every file in a tree and a hash of its contents.
func fingerprint(t *testing.T, root string) map[string]string {
	t.Helper()

	out := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !entry.Type().IsRegular() {
			return err
		}
		f, err := os.Open(path) // #nosec G304,G703 -- a path under the tree being read
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		sum := sha256.New()
		if _, err := io.Copy(sum, f); err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[relative] = string(sum.Sum(nil))
		return nil
	}); err != nil {
		t.Fatalf("reading the backup: %v", err)
	}
	return out
}

func same(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for name, hash := range a {
		if b[name] != hash {
			return false
		}
	}
	return true
}

// megabytes renders a size the way people talk about files.
func megabytes(n int64) string {
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}
