package backupfs

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite" // registers the pure-Go SQLite driver
)

// TestFileIDMatchesKnownVectors protects the one calculation everything else in this
// package depends on: get the hash formula, separator or hex casing wrong and Find
// and ExtractDatabase would silently look in the wrong place instead of failing
// loudly. The first two vectors are long-published examples from iOS backup
// forensics (HomeDomain's SMS and address book databases) rather than values this
// package invented, so this test would still catch a mistake even if it were the
// only place that formula was ever written down.
func TestFileIDMatchesKnownVectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		domain       string
		relativePath string
		want         string
	}{
		{
			name:         "HomeDomain SMS database",
			domain:       "HomeDomain",
			relativePath: "Library/SMS/sms.db",
			want:         "3d0d7e5fb2ce288813306e4d4636395e047a3d28",
		},
		{
			name:         "HomeDomain address book",
			domain:       "HomeDomain",
			relativePath: "Library/AddressBook/AddressBook.sqlitedb",
			want:         "31bb7ba8914766d4ba40d6dfb6113c8b614be442",
		},
		{
			name:         "WhatsApp's shared chat database",
			domain:       "AppDomainGroup-group.net.whatsapp.WhatsApp.shared",
			relativePath: "ChatStorage.sqlite",
			want:         "7c7fba66680ef796b916b067077cc246adacf01d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := fileIDOf(tt.domain, tt.relativePath); got != tt.want {
				t.Errorf("fileIDOf(%q, %q) = %q, want %q", tt.domain, tt.relativePath, got, tt.want)
			}
		})
	}
}

// TestVerifyFilesTableRejectsWrongShape protects Open against a Manifest.db that
// parses fine as SQLite but does not have the table this package depends on: a wrong
// file was handed to it, or Apple changes the schema in a way this build has never
// seen, and either way a clear guidance error beats a confusing SQL error three
// layers down.
func TestVerifyFilesTableRejectsWrongShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup string
	}{
		{name: "no Files table at all", setup: `CREATE TABLE other (id INTEGER PRIMARY KEY)`},
		{name: "Files table missing required columns", setup: `CREATE TABLE Files (fileID TEXT PRIMARY KEY, domain TEXT)`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/Manifest.db")
			if err != nil {
				t.Fatalf("opening a fixture database: %v", err)
			}
			defer db.Close()
			if _, err := db.Exec(tt.setup); err != nil {
				t.Fatalf("fixture setup: %v", err)
			}

			err = verifyFilesTable(context.Background(), db)
			if !errors.Is(err, ErrCorruptManifest) {
				t.Fatalf("verifyFilesTable() error = %v, want %v", err, ErrCorruptManifest)
			}
		})
	}
}

// TestVerifyFilesTableAcceptsTheRealShape is the positive case behind the rejection
// test above: a Files table built the way buildManifestDB (and therefore a genuine
// backup) builds it must pass.
func TestVerifyFilesTableAcceptsTheRealShape(t *testing.T) {
	t.Parallel()

	dir := buildBackup(t)
	db := openManifestDB(t, dir)
	defer db.Close()

	if err := verifyFilesTable(context.Background(), db); err != nil {
		t.Fatalf("verifyFilesTable() unexpected error: %v", err)
	}
}

// TestLookupFile covers Find's underlying primary-key lookup: a file is found with
// its size, a directory is found and marked as one, and an absent entry is reported
// as ok == false rather than as an error, because "this app was never opened on the
// phone" is the normal case, not a failure.
func TestLookupFile(t *testing.T) {
	t.Parallel()

	const domain = "AppDomainGroup-group.net.whatsapp.WhatsApp.shared"
	content := []byte("pretend this is ChatStorage.sqlite")
	dir := buildBackup(t, withFiles(
		fixtureFile{domain: domain, relativePath: "ChatStorage.sqlite", content: content},
		fixtureFile{domain: domain, relativePath: "Message/Media", isDir: true},
	))
	db := openManifestDB(t, dir)
	// t.Cleanup, not defer: the subtests below are parallel, so this function's body
	// returns (and a plain defer would fire) before any of them actually run.
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	t.Run("an entry that exists is found with its size", func(t *testing.T) {
		t.Parallel()

		f, ok, err := lookupFile(ctx, db, domain, "ChatStorage.sqlite")
		if err != nil {
			t.Fatalf("lookupFile() unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("lookupFile() ok = false, want true")
		}
		if f.IsDir {
			t.Error("IsDir = true, want false")
		}
		if f.Size != int64(len(content)) {
			t.Errorf("Size = %d, want %d", f.Size, len(content))
		}
	})

	t.Run("a directory entry is reported as one", func(t *testing.T) {
		t.Parallel()

		f, ok, err := lookupFile(ctx, db, domain, "Message/Media")
		if err != nil {
			t.Fatalf("lookupFile() unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("lookupFile() ok = false, want true")
		}
		if !f.IsDir {
			t.Error("IsDir = false, want true")
		}
	})

	t.Run("an absent entry is not an error", func(t *testing.T) {
		t.Parallel()

		_, ok, err := lookupFile(ctx, db, domain, "does/not/exist")
		if err != nil {
			t.Fatalf("lookupFile() unexpected error: %v", err)
		}
		if ok {
			t.Fatal("lookupFile() ok = true, want false")
		}
	})

	t.Run("a corrupt MBFile blob is an error, not a panic", func(t *testing.T) {
		t.Parallel()

		rawDir := t.TempDir()
		buildRawManifestDB(t, rawDir, []rawRow{
			{domain: "d", relativePath: "p", flags: 1, blob: []byte("not a plist at all")},
		})
		rawDB := openManifestDB(t, rawDir)
		defer rawDB.Close()

		_, _, err := lookupFile(ctx, rawDB, "d", "p")
		if !errors.Is(err, ErrCorruptManifest) {
			t.Fatalf("lookupFile() error = %v, want %v", err, ErrCorruptManifest)
		}
	})
}

// TestListFilesInDomain protects the other half of "query what you need": only rows
// matching the requested domain come back, never rows from an unrelated app.
func TestListFilesInDomain(t *testing.T) {
	t.Parallel()

	dir := buildBackup(t, withFiles(
		fixtureFile{domain: "domainA", relativePath: "a.txt", content: []byte("a")},
		fixtureFile{domain: "domainA", relativePath: "b.txt", content: []byte("bb")},
		fixtureFile{domain: "domainB", relativePath: "c.txt", content: []byte("ccc")},
	))
	db := openManifestDB(t, dir)
	defer db.Close()

	files, err := listFilesInDomain(context.Background(), db, "domainA")
	if err != nil {
		t.Fatalf("listFilesInDomain() unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("listFilesInDomain() = %d files, want 2", len(files))
	}
	for _, f := range files {
		if f.Domain != "domainA" {
			t.Errorf("listFilesInDomain(%q) returned a file from domain %q", "domainA", f.Domain)
		}
	}
}

// TestListFilesInDomainWithCorruptBlob protects Domain the same way the lookupFile
// subtest above protects Find: one row with an unparseable MBFile blob is reported as
// a corrupt manifest, not a panic, even when it is discovered through a listing
// rather than a single lookup.
func TestListFilesInDomainWithCorruptBlob(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	buildRawManifestDB(t, dir, []rawRow{
		{domain: "d", relativePath: "p", flags: 1, blob: []byte("not a plist at all")},
	})
	db := openManifestDB(t, dir)
	defer db.Close()

	_, err := listFilesInDomain(context.Background(), db, "d")
	if !errors.Is(err, ErrCorruptManifest) {
		t.Fatalf("listFilesInDomain() error = %v, want %v", err, ErrCorruptManifest)
	}
}

// TestVerifyFilesTableOnAFailedQuery protects the error path that has nothing to do
// with the schema itself: whatever the underlying database.QueryContext failure is
// (here, the connection is simply already closed), it must come back as a typed,
// guidance-carrying error.
func TestVerifyFilesTableOnAFailedQuery(t *testing.T) {
	t.Parallel()

	dir := buildBackup(t)
	db := openManifestDB(t, dir)
	if err := db.Close(); err != nil {
		t.Fatalf("closing the database: %v", err)
	}

	err := verifyFilesTable(context.Background(), db)
	if !errors.Is(err, ErrCorruptManifest) {
		t.Fatalf("verifyFilesTable() error = %v, want %v", err, ErrCorruptManifest)
	}
}
