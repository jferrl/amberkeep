package backupfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// chatStorage is the file inside WhatsApp's shared container that all of this is
// for. The domain it lives in is named beside the other fixtures.
const chatStorage = "ChatStorage.sqlite"

// TestAgainstARealBackup reads a backup Finder or the Apple Devices app actually
// made, because a synthetic one proves only that this package agrees with itself.
//
// It is skipped by default and the path comes from the environment: a real backup
// is somebody's whole phone and must never be committed.
//
//	AMBERKEEP_REAL_BACKUP=/path/to/Backup/<udid> go test ./internal/backupfs/
func TestAgainstARealBackup(t *testing.T) {
	dir := os.Getenv("AMBERKEEP_REAL_BACKUP")
	if dir == "" {
		t.Skip("set AMBERKEEP_REAL_BACKUP to check against a backup Apple made")
	}

	ctx := context.Background()
	started := time.Now()

	archive, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open() failed on a real backup: %v", err)
	}
	defer func() { _ = archive.Close() }()

	t.Logf("device %q, %s, iOS %s, last backed up %s",
		archive.DeviceName, archive.ProductType, archive.IOSVersion,
		archive.LastBackup.Format(time.RFC3339))
	t.Logf("opened in %s", time.Since(started).Round(time.Millisecond))

	t.Run("WhatsApp's own files are found", func(t *testing.T) {
		files, err := archive.Domain(ctx, whatsappDomain)
		if err != nil {
			t.Fatalf("Domain() failed: %v", err)
		}
		if len(files) == 0 {
			t.Fatal("the backup holds none of WhatsApp's files, so either the domain is wrong or this phone had no WhatsApp")
		}
		t.Logf("%d files in WhatsApp's shared container", len(files))
	})

	t.Run("the message store is where it should be", func(t *testing.T) {
		file, found, err := archive.Find(ctx, whatsappDomain, chatStorage)
		if err != nil {
			t.Fatalf("Find() failed: %v", err)
		}
		if !found {
			t.Fatal("ChatStorage.sqlite was not found, which is the one file this is all for")
		}
		t.Logf("the store is %d bytes", file.Size)

		if file.Size <= 0 {
			t.Error("the store's size was not read from its record")
		}
	})

	t.Run("the store comes out whole, with what its sidecars held", func(t *testing.T) {
		out := t.TempDir()
		at := time.Now()

		path, err := archive.ExtractDatabase(ctx, out, whatsappDomain, chatStorage)
		if err != nil {
			t.Fatalf("ExtractDatabase() failed: %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("the extracted store is not there: %v", err)
		}
		t.Logf("extracted %d bytes to %s in %s",
			info.Size(), filepath.Base(path), time.Since(at).Round(time.Millisecond))

		if info.Size() == 0 {
			t.Fatal("the extracted store is empty")
		}
		// A checkpointed copy carries everything: the write-ahead log should have
		// been folded in and left with nothing to replay.
		if wal, err := os.Stat(path + "-wal"); err == nil && wal.Size() > 0 {
			t.Errorf("the extracted store still has %d bytes of write-ahead log to replay", wal.Size())
		}
	})

	t.Run("nothing in the backup was changed", func(t *testing.T) {
		// The one rule this package must never break.
		manifest := filepath.Join(dir, "Manifest.db")
		before, err := os.Stat(manifest)
		if err != nil {
			t.Fatalf("looking at the backup: %v", err)
		}

		out := t.TempDir()
		if _, err := archive.ExtractDatabase(ctx, out, whatsappDomain, chatStorage); err != nil {
			t.Fatalf("ExtractDatabase() failed: %v", err)
		}

		after, err := os.Stat(manifest)
		if err != nil {
			t.Fatalf("looking at the backup: %v", err)
		}
		if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			t.Error("reading the backup changed it")
		}
		for _, sidecar := range []string{manifest + "-wal", manifest + "-shm"} {
			if _, err := os.Stat(sidecar); err == nil {
				t.Errorf("reading the backup left %s beside it", filepath.Base(sidecar))
			}
		}
	})
}
