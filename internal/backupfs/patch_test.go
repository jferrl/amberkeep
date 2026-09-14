package backupfs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Putting a changed file back into a backup.
//
// This is the last thing that happens before somebody restores, so what is tested is
// not only that the right thing changes but that nothing else does: a backup is
// thousands of files and one of them is somebody's health record.

const (
	whatsApp = "AppDomainGroup-group.net.whatsapp.WhatsApp.shared"
	store    = "ChatStorage.sqlite"
)

// backupHolding builds a backup carrying a store, and optionally a log beside it.
func backupHolding(t *testing.T, storeBytes, log []byte) string {
	t.Helper()

	files := []fixtureFile{
		{domain: whatsApp, relativePath: store, content: storeBytes},
		{domain: whatsApp, relativePath: "Media", isDir: true},
		{domain: "HomeDomain", relativePath: "Library/Health/healthdb.sqlite", content: []byte("somebody's health")},
	}
	if log != nil {
		files = append(files, fixtureFile{domain: whatsApp, relativePath: store + "-wal", content: log})
	}
	return buildBackup(t, withFiles(files...))
}

// replacement writes a database to put in, and returns where it is.
func replacement(t *testing.T, statements ...string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ChatStorage.migrated.sqlite")
	if err := os.WriteFile(path, buildSQLiteDB(t, statements...), 0o600); err != nil {
		t.Fatalf("writing the replacement: %v", err)
	}
	return path
}

func TestPatchPutsAFileBackWithoutTouchingTheBackup(t *testing.T) {
	t.Parallel()

	from := backupHolding(t, buildSQLiteDB(t, "CREATE TABLE before (a)"), nil)
	with := replacement(t, "CREATE TABLE after (a)", "CREATE TABLE more (b)")
	into := filepath.Join(t.TempDir(), "patched")

	untouched := snapshot(t, from)

	result, err := Patch(context.Background(), from, into,
		Replacement{Domain: whatsApp, RelativePath: store, With: with})
	if err != nil {
		t.Fatalf("Patch() failed: %v", err)
	}

	t.Run("the original is exactly as it was", func(t *testing.T) {
		if changed := compare(t, from, untouched); changed != "" {
			t.Errorf("the original backup changed: %s", changed)
		}
	})

	t.Run("the copy carries the new contents", func(t *testing.T) {
		wanted, err := os.ReadFile(with)
		if err != nil {
			t.Fatalf("reading the replacement: %v", err)
		}
		got := payloadIn(t, into, whatsApp, store)
		if !bytes.Equal(got, wanted) {
			t.Error("the copy does not hold what was put in")
		}
		if result.Now != int64(len(wanted)) {
			t.Errorf("it reports %d bytes, and the file is %d", result.Now, len(wanted))
		}
	})

	t.Run("and its index agrees about the size", func(t *testing.T) {
		// A restore reads the size from the index, not from the file. The two
		// disagreeing is how a restore truncates a database and reports success.
		db := openManifestDB(t, into)
		defer func() { _ = db.Close() }()

		var blob []byte
		if err := db.QueryRow("SELECT file FROM Files WHERE domain = ? AND relativePath = ?",
			whatsApp, store).Scan(&blob); err != nil {
			t.Fatalf("reading the index: %v", err)
		}
		recorded, err := parseMBFile(blob)
		if err != nil {
			t.Fatalf("reading the file's metadata: %v", err)
		}
		if recorded.Size != result.Now {
			t.Errorf("the index says %d bytes and the file is %d", recorded.Size, result.Now)
		}
	})

	t.Run("nothing else in the copy is different", func(t *testing.T) {
		// Every other payload has to arrive byte for byte. A backup holds thousands
		// of files and one of them is somebody's health record.
		health := payloadIn(t, into, "HomeDomain", "Library/Health/healthdb.sqlite")
		if string(health) != "somebody's health" {
			t.Error("a file nobody asked about was changed")
		}
	})

	t.Run("the copy carries a single index, as a backup does", func(t *testing.T) {
		for _, side := range []string{"-wal", "-shm"} {
			if _, err := os.Stat(filepath.Join(into, "Manifest.db"+side)); err == nil {
				t.Errorf("the copy has a %s beside its index", side)
			}
		}
	})
}

// TestPatchEmptiesAStaleLog is the failure that reports success and loses everything.
//
// A backup records a database's write-ahead log as a file of its own. Replace the
// database and leave the log, and the phone opens the new contents and replays a log
// written against the old ones over the top.
func TestPatchEmptiesAStaleLog(t *testing.T) {
	t.Parallel()

	main, log, _ := buildWALPair(t)
	from := backupHolding(t, main, log)
	with := replacement(t, "CREATE TABLE after (a)")
	into := filepath.Join(t.TempDir(), "patched")

	result, err := Patch(context.Background(), from, into,
		Replacement{Domain: whatsApp, RelativePath: store, With: with})
	if err != nil {
		t.Fatalf("Patch() failed: %v", err)
	}

	if !result.LogNeutralised {
		t.Fatal("it did not say it had dealt with the log")
	}
	if got := payloadIn(t, into, whatsApp, store+"-wal"); len(got) != 0 {
		t.Errorf("the log is still %d bytes, and would be replayed over the new store", len(got))
	}

	db := openManifestDB(t, into)
	defer func() { _ = db.Close() }()

	var blob []byte
	if err := db.QueryRow("SELECT file FROM Files WHERE domain = ? AND relativePath = ?",
		whatsApp, store+"-wal").Scan(&blob); err != nil {
		t.Fatalf("reading the index: %v", err)
	}
	recorded, err := parseMBFile(blob)
	if err != nil {
		t.Fatalf("reading the log's metadata: %v", err)
	}
	if recorded.Size != 0 {
		t.Errorf("the index still says the log is %d bytes", recorded.Size)
	}
}

func TestPatchRefuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setup returns the backup, the replacement and the destination.
		setup func(t *testing.T) (from, with, into string)
		want  error
	}{
		{
			name: "an encrypted backup",
			setup: func(t *testing.T) (string, string, string) {
				t.Helper()
				dir := t.TempDir()
				buildBackupIn(t, dir, withEncrypted())
				return dir, replacement(t, "CREATE TABLE a (b)"), filepath.Join(t.TempDir(), "out")
			},
			want: ErrEncrypted,
		},
		{
			name: "somewhere that already holds something",
			setup: func(t *testing.T) (string, string, string) {
				t.Helper()
				into := t.TempDir()
				return backupHolding(t, buildSQLiteDB(t, "CREATE TABLE a (b)"), nil),
					replacement(t, "CREATE TABLE a (b)"), into
			},
			want: ErrWouldOverwrite,
		},
		{
			name: "something that is not a database",
			setup: func(t *testing.T) (string, string, string) {
				t.Helper()
				with := filepath.Join(t.TempDir(), "holiday.jpg")
				if err := os.WriteFile(with, []byte{0xff, 0xd8, 0xff, 0xe0}, 0o600); err != nil {
					t.Fatalf("writing the fixture: %v", err)
				}
				return backupHolding(t, buildSQLiteDB(t, "CREATE TABLE a (b)"), nil),
					with, filepath.Join(t.TempDir(), "out")
			},
			want: ErrNotADatabase,
		},
		{
			name: "a database whose most recent changes are not in it yet",
			setup: func(t *testing.T) (string, string, string) {
				t.Helper()
				with := replacement(t, "CREATE TABLE a (b)")
				if err := os.WriteFile(with+"-wal", []byte("changes not folded in"), 0o600); err != nil {
					t.Fatalf("writing the fixture: %v", err)
				}
				return backupHolding(t, buildSQLiteDB(t, "CREATE TABLE a (b)"), nil),
					with, filepath.Join(t.TempDir(), "out")
			},
			want: ErrUnfinished,
		},
		{
			name: "a file the backup does not hold",
			setup: func(t *testing.T) (string, string, string) {
				t.Helper()
				return buildBackup(t), replacement(t, "CREATE TABLE a (b)"),
					filepath.Join(t.TempDir(), "out")
			},
			want: ErrFileNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			from, with, into := tt.setup(t)
			_, err := Patch(context.Background(), from, into,
				Replacement{Domain: whatsApp, RelativePath: store, With: with})
			if !errors.Is(err, tt.want) {
				t.Fatalf("Patch() returned %v, want %v", err, tt.want)
			}
		})
	}
}

// TestPatchLeavesNothingBehindWhenItFails covers the state worth fearing most: half
// a backup, which looks exactly like a backup.
//
// The failure is arranged after the copy has been made, which is the only part where
// there is anything to leave behind. The first version of this test made the
// replacement unreadable, which fails before a single byte is copied — and on Windows
// does not fail at all, because chmod there only toggles the read-only bit. CI on an
// operating system nobody develops on found a test that was not testing what it said.
func TestPatchLeavesNothingBehindWhenItFails(t *testing.T) {
	t.Parallel()

	from := backupHolding(t, buildSQLiteDB(t, "CREATE TABLE a (b)"), nil)
	into := filepath.Join(t.TempDir(), "abandoned")

	// A backup whose index lists the store but whose contents are not on disk, which
	// is a damaged backup and is found only once the copy has been made.
	id := fileIDOf(whatsApp, store)
	if err := os.Remove(filepath.Join(from, id[:2], id)); err != nil {
		t.Fatalf("preparing the fixture: %v", err)
	}

	_, err := Patch(context.Background(), from, into,
		Replacement{Domain: whatsApp, RelativePath: store, With: replacement(t, "CREATE TABLE a (b)")})
	if err == nil {
		t.Fatal("it succeeded against a backup missing the file it was replacing")
	}
	if _, err := os.Stat(into); err == nil {
		t.Error("it left half a backup behind, which looks exactly like a backup")
	}
}
