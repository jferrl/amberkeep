package backupfs

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite" // registers the pure-Go SQLite driver
)

const whatsappDomain = "AppDomainGroup-group.net.whatsapp.WhatsApp.shared"

// TestOpenRejectsWhatItShouldNotRead protects the two refusals Open must make before
// ever touching Manifest.db: a folder that is not a backup at all, and one that is a
// backup but encrypted, which is out of scope by design (see ErrEncrypted).
func TestOpenRejectsWhatItShouldNotRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		build   func(t *testing.T) string
		wantErr error
	}{
		{
			name:    "a folder that does not exist",
			build:   func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent") },
			wantErr: ErrNotABackup,
		},
		{
			name: "a folder missing the backup marker files",
			build: func(t *testing.T) string {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "random.txt"), []byte("hi"), 0o600); err != nil {
					t.Fatalf("writing the fixture: %v", err)
				}
				return dir
			},
			wantErr: ErrNotABackup,
		},
		{
			name:    "an encrypted backup",
			build:   func(t *testing.T) string { return buildBackup(t, withEncrypted()) },
			wantErr: ErrEncrypted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a, err := Open(context.Background(), tt.build(t))
			if err == nil {
				_ = a.Close()
				t.Fatal("Open() unexpectedly succeeded")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Open() error = %v, want %v", err, tt.wantErr)
			}
			var e *Error
			if !errors.As(err, &e) || e.Guidance == "" {
				t.Error("Open() error carries no guidance identifier")
			}
		})
	}
}

// TestOpenReadsBackupMetadata protects Archive's promise to expose the device name,
// product type, iOS version and last-backup date without a separate accessor: they
// come straight off the embedded Backup.
func TestOpenReadsBackupMetadata(t *testing.T) {
	t.Parallel()

	dir := buildBackup(t)
	a, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer a.Close()

	if a.DeviceName == "" {
		t.Error("DeviceName is empty")
	}
	if a.ProductType == "" {
		t.Error("ProductType is empty")
	}
	if a.IOSVersion == "" {
		t.Error("IOSVersion is empty")
	}
	if a.LastBackup.IsZero() {
		t.Error("LastBackup is zero")
	}
	if a.Encrypted {
		t.Error("Encrypted = true for a plain backup")
	}
	if a.UDID != filepath.Base(dir) {
		t.Errorf("UDID = %q, want %q", a.UDID, filepath.Base(dir))
	}
}

// TestFindAndDomain protects the two ways of locating entries once an Archive is
// open: a direct lookup by domain and relative path, and a listing of everything
// under one domain.
func TestFindAndDomain(t *testing.T) {
	t.Parallel()

	dir := buildBackup(t, withFiles(
		fixtureFile{domain: whatsappDomain, relativePath: "ChatStorage.sqlite", content: []byte("main")},
		fixtureFile{domain: whatsappDomain, relativePath: "Message/Media", isDir: true},
		fixtureFile{domain: whatsappDomain, relativePath: "Message/Media/1.jpg", content: []byte("jpeg bytes")},
	))
	a, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	// t.Cleanup, not defer: the subtests below are parallel, so this function's body
	// returns (and a plain defer would fire) before any of them actually run.
	t.Cleanup(func() { a.Close() })
	ctx := context.Background()

	t.Run("Find locates a file that exists", func(t *testing.T) {
		t.Parallel()

		f, ok, err := a.Find(ctx, whatsappDomain, "ChatStorage.sqlite")
		if err != nil {
			t.Fatalf("Find() unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("Find() ok = false, want true")
		}
		if f.Size != 4 {
			t.Errorf("Size = %d, want 4", f.Size)
		}
	})

	t.Run("Find reports absence without an error", func(t *testing.T) {
		t.Parallel()

		_, ok, err := a.Find(ctx, whatsappDomain, "does/not/exist")
		if err != nil {
			t.Fatalf("Find() unexpected error: %v", err)
		}
		if ok {
			t.Fatal("Find() ok = true, want false")
		}
	})

	t.Run("Domain lists every entry under it and nothing else", func(t *testing.T) {
		t.Parallel()

		files, err := a.Domain(ctx, whatsappDomain)
		if err != nil {
			t.Fatalf("Domain() unexpected error: %v", err)
		}
		if len(files) != 3 {
			t.Fatalf("Domain() = %d files, want 3", len(files))
		}
	})
}

// TestExtract protects the single-file copy path: bytes arrive unchanged, and a
// directory entry is refused rather than producing an empty or garbage file.
func TestExtract(t *testing.T) {
	t.Parallel()

	content := []byte("the exact bytes Apple stored for this file")
	dir := buildBackup(t, withFiles(
		fixtureFile{domain: whatsappDomain, relativePath: "Message/Media/photo.jpg", content: content},
		fixtureFile{domain: whatsappDomain, relativePath: "Message/Media", isDir: true},
	))
	a, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	// t.Cleanup, not defer: the subtests below are parallel, so this function's body
	// returns (and a plain defer would fire) before any of them actually run.
	t.Cleanup(func() { a.Close() })
	ctx := context.Background()

	t.Run("a file's bytes are copied out unchanged", func(t *testing.T) {
		t.Parallel()

		f, ok, err := a.Find(ctx, whatsappDomain, "Message/Media/photo.jpg")
		if err != nil || !ok {
			t.Fatalf("Find() = %+v, %v, %v", f, ok, err)
		}

		dst := filepath.Join(t.TempDir(), "photo.jpg")
		if err := a.Extract(ctx, dst, f); err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("reading the extracted file: %v", err)
		}
		if !bytes.Equal(got, content) {
			t.Errorf("Extract() copied %q, want %q", got, content)
		}
	})

	t.Run("a directory entry cannot be extracted", func(t *testing.T) {
		t.Parallel()

		f, ok, err := a.Find(ctx, whatsappDomain, "Message/Media")
		if err != nil || !ok {
			t.Fatalf("Find() = %+v, %v, %v", f, ok, err)
		}

		err = a.Extract(ctx, filepath.Join(t.TempDir(), "media"), f)
		if !errors.Is(err, ErrNotAFile) {
			t.Fatalf("Extract() error = %v, want %v", err, ErrNotAFile)
		}
	})
}

// TestExtractDatabaseCarriesOverUncheckpointedWrites is the test for the behaviour
// this package exists to get right: a row that, at backup time, lived only in
// ChatStorage.sqlite's write-ahead log must still be there in the extracted copy, and
// the backup's own files must be untouched afterwards.
func TestExtractDatabaseCarriesOverUncheckpointedWrites(t *testing.T) {
	t.Parallel()

	mainBytes, walBytes, shmBytes := buildWALPair(t)

	dir := buildBackup(t, withFiles(
		fixtureFile{domain: whatsappDomain, relativePath: "ChatStorage.sqlite", content: mainBytes},
		fixtureFile{domain: whatsappDomain, relativePath: "ChatStorage.sqlite-wal", content: walBytes},
		fixtureFile{domain: whatsappDomain, relativePath: "ChatStorage.sqlite-shm", content: shmBytes},
	))

	a, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer a.Close()

	dstDir := t.TempDir()
	got, err := a.ExtractDatabase(context.Background(), dstDir, whatsappDomain, "ChatStorage.sqlite")
	if err != nil {
		t.Fatalf("ExtractDatabase() failed: %v", err)
	}
	if filepath.Dir(got) != dstDir {
		t.Errorf("ExtractDatabase() returned %q, not inside dstDir %q", got, dstDir)
	}

	db, err := sql.Open("sqlite", "file:"+got+"?mode=ro")
	if err != nil {
		t.Fatalf("opening the extracted copy: %v", err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM msg`).Scan(&count); err != nil {
		t.Fatalf("querying the extracted copy: %v", err)
	}
	if count != 2 {
		t.Errorf("extracted copy has %d rows, want 2 (one only ever lived in the -wal file)", count)
	}

	id := fileIDOf(whatsappDomain, "ChatStorage.sqlite")
	stillThere, err := os.ReadFile(filepath.Join(dir, id[:2], id))
	if err != nil {
		t.Fatalf("reading the backup's own copy: %v", err)
	}
	if !bytes.Equal(stillThere, mainBytes) {
		t.Error("the backup's own ChatStorage.sqlite blob changed; ExtractDatabase must never write to the original")
	}
}

// TestExtractDatabaseWithoutSidecars protects the common case: most databases in a
// real backup were never caught mid-write, and ExtractDatabase must still succeed
// with nothing to carry over.
func TestExtractDatabaseWithoutSidecars(t *testing.T) {
	t.Parallel()

	content := buildSQLiteDB(t, `CREATE TABLE t (id INTEGER PRIMARY KEY)`)
	dir := buildBackup(t, withFiles(
		fixtureFile{domain: "SomeDomain", relativePath: "plain.sqlite", content: content},
	))
	a, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer a.Close()

	got, err := a.ExtractDatabase(context.Background(), t.TempDir(), "SomeDomain", "plain.sqlite")
	if err != nil {
		t.Fatalf("ExtractDatabase() failed: %v", err)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("the extracted file is missing: %v", err)
	}
}

// TestExtractDatabaseReportsAMissingFile protects callers against a silent empty
// result when the domain and relative path they asked for simply is not in this
// backup — an app that was never opened on the phone, most often.
func TestExtractDatabaseReportsAMissingFile(t *testing.T) {
	t.Parallel()

	dir := buildBackup(t)
	a, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer a.Close()

	_, err = a.ExtractDatabase(context.Background(), t.TempDir(), whatsappDomain, "ChatStorage.sqlite")
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("ExtractDatabase() error = %v, want %v", err, ErrFileNotFound)
	}
}

// TestExtractDatabaseRefusesADirectory protects against asking for a domain and
// relative path that names a directory entry in the manifest: there is no database
// to extract, and the failure must say so rather than trying to copy it anyway.
func TestExtractDatabaseRefusesADirectory(t *testing.T) {
	t.Parallel()

	dir := buildBackup(t, withFiles(
		fixtureFile{domain: whatsappDomain, relativePath: "Message/Media", isDir: true},
	))
	a, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer a.Close()

	_, err = a.ExtractDatabase(context.Background(), t.TempDir(), whatsappDomain, "Message/Media")
	if !errors.Is(err, ErrNotAFile) {
		t.Fatalf("ExtractDatabase() error = %v, want %v", err, ErrNotAFile)
	}
}

// TestExtractDatabaseReportsACheckpointFailure protects against a database that
// copies fine but is not, in fact, a valid SQLite file: checkpointing must fail
// loudly rather than silently handing back a copy nothing can open.
func TestExtractDatabaseReportsACheckpointFailure(t *testing.T) {
	t.Parallel()

	dir := buildBackup(t, withFiles(
		fixtureFile{domain: "SomeDomain", relativePath: "garbage.sqlite", content: []byte("not a real sqlite file")},
	))
	a, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer a.Close()

	_, err = a.ExtractDatabase(context.Background(), t.TempDir(), "SomeDomain", "garbage.sqlite")
	if !errors.Is(err, ErrUnreadable) {
		t.Fatalf("ExtractDatabase() error = %v, want %v", err, ErrUnreadable)
	}
}

// TestOpenRejectsACorruptManifestDB protects Open against a Manifest.db that is a
// genuine, readable SQLite file but does not have the Files table this package
// depends on.
func TestOpenRejectsACorruptManifestDB(t *testing.T) {
	t.Parallel()

	dir := buildBackup(t)
	// buildBackup already created a well-formed Manifest.db; replace it with one
	// that is a valid database but the wrong shape.
	manifestPath := filepath.Join(dir, "Manifest.db")
	if err := os.Remove(manifestPath); err != nil {
		t.Fatalf("removing the fixture Manifest.db: %v", err)
	}
	bad, err := sql.Open("sqlite", "file:"+manifestPath)
	if err != nil {
		t.Fatalf("opening a replacement Manifest.db: %v", err)
	}
	if _, err := bad.Exec(`CREATE TABLE something_else (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("writing a replacement Manifest.db: %v", err)
	}
	if err := bad.Close(); err != nil {
		t.Fatalf("closing a replacement Manifest.db: %v", err)
	}

	_, err = Open(context.Background(), dir)
	if !errors.Is(err, ErrCorruptManifest) {
		t.Fatalf("Open() error = %v, want %v", err, ErrCorruptManifest)
	}
}

// TestExtractRespectsCancelledContext protects Extract's context-awareness: a copy
// that has already been told to stop must report that, rather than running a large
// file to completion after its caller has given up.
func TestExtractRespectsCancelledContext(t *testing.T) {
	t.Parallel()

	dir := buildBackup(t, withFiles(
		fixtureFile{domain: whatsappDomain, relativePath: "big.dat", content: []byte("some bytes to copy")},
	))
	a, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer a.Close()

	f, ok, err := a.Find(context.Background(), whatsappDomain, "big.dat")
	if err != nil || !ok {
		t.Fatalf("Find() = %+v, %v, %v", f, ok, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = a.Extract(ctx, filepath.Join(t.TempDir(), "big.dat"), f)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Extract() error = %v, want it to wrap context.Canceled", err)
	}
}

// TestExtractFailsWhenDestinationCannotBeCreated protects the write side of Extract:
// a destination that cannot be created — here, a path that already exists as a
// directory — must be reported as an error, not silently ignored.
func TestExtractFailsWhenDestinationCannotBeCreated(t *testing.T) {
	t.Parallel()

	dir := buildBackup(t, withFiles(
		fixtureFile{domain: whatsappDomain, relativePath: "small.dat", content: []byte("bytes")},
	))
	a, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer a.Close()

	f, ok, err := a.Find(context.Background(), whatsappDomain, "small.dat")
	if err != nil || !ok {
		t.Fatalf("Find() = %+v, %v, %v", f, ok, err)
	}

	dstIsADirectory := t.TempDir()
	err = a.Extract(context.Background(), dstIsADirectory, f)
	if !errors.Is(err, ErrUnreadable) {
		t.Fatalf("Extract() error = %v, want %v", err, ErrUnreadable)
	}
}

// TestPathOfRejectsAMalformedFileID protects pathOf's own defensiveness: a File
// value with a fileID too short to split into a shard directory must not panic on
// the slice operation that builds the on-disk path.
func TestPathOfRejectsAMalformedFileID(t *testing.T) {
	t.Parallel()

	a := &Archive{Backup: Backup{Path: t.TempDir()}}
	_, err := a.pathOf(File{Domain: "d", RelativePath: "p", id: "x"})
	if !errors.Is(err, ErrCorruptManifest) {
		t.Fatalf("pathOf() error = %v, want %v", err, ErrCorruptManifest)
	}
}

// TestOpenLeavesNothingBesideTheManifest is a regression test for a bug no
// synthetic fixture could have found.
//
// Apple writes Manifest.db in write-ahead-log mode, and SQLite creates a -wal and a
// -shm beside any database it opens that way, read-only or not. Two files then
// appear inside somebody's backup that were not there before. Every fixture here
// used the default journal mode, where that does not happen, so it took a real
// backup to show it.
func TestOpenLeavesNothingBesideTheManifest(t *testing.T) {
	t.Parallel()

	dir := buildBackup(t)
	manifest := filepath.Join(dir, "Manifest.db")

	// Put the manifest into the mode a real one is in.
	db, err := sql.Open("sqlite", "file:"+manifest)
	if err != nil {
		t.Fatalf("opening the manifest: %v", err)
	}
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode = WAL`).Scan(&mode); err != nil {
		t.Fatalf("switching the manifest to write-ahead logging: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the manifest: %v", err)
	}
	if mode != "wal" {
		t.Skipf("this SQLite build would not switch to write-ahead logging (%q)", mode)
	}
	// Whatever switching the mode left behind is not what this test is about.
	for _, sidecar := range []string{manifest + "-wal", manifest + "-shm"} {
		_ = os.Remove(sidecar)
	}

	archive, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	if _, _, err := archive.Find(context.Background(), whatsappDomain, "ChatStorage.sqlite"); err != nil {
		t.Fatalf("Find() failed: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	for _, sidecar := range []string{manifest + "-wal", manifest + "-shm"} {
		if _, err := os.Stat(sidecar); err == nil {
			t.Errorf("reading the backup left %s inside it", filepath.Base(sidecar))
		}
	}
}
