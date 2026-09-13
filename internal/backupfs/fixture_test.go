package backupfs

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite" // registers the pure-Go SQLite driver

	"howett.net/plist"
)

// fixtureFile is one entry to seed into a synthetic backup's Manifest.db, and, for a
// file (not a directory), onto disk at the content-addressed path a real backup would
// use.
type fixtureFile struct {
	domain       string
	relativePath string
	isDir        bool
	content      []byte
}

// backupConfig customises buildBackup. The zero value already describes a plausible,
// unencrypted backup; options change only what a given test cares about.
type backupConfig struct {
	encrypted      bool
	deviceName     string
	productType    string
	productVersion string
	lastBackup     time.Time
	files          []fixtureFile
}

type backupOption func(*backupConfig)

func withEncrypted() backupOption { return func(c *backupConfig) { c.encrypted = true } }

func withFiles(files ...fixtureFile) backupOption {
	return func(c *backupConfig) { c.files = append(c.files, files...) }
}

// buildBackup assembles a synthetic backup folder in a fresh temporary directory: a
// real Manifest.plist and Info.plist (binary property lists, produced by the same
// library production code uses to read them), a Status.plist placeholder, and a real
// Manifest.db built with modernc.org/sqlite, with any requested files both recorded
// in the Files table and, for non-directory entries, written to their
// content-addressed path on disk.
func buildBackup(t *testing.T, opts ...backupOption) string {
	t.Helper()
	return buildBackupIn(t, t.TempDir(), opts...)
}

// buildBackupIn is buildBackup for a caller that already has a specific directory the
// backup must live in, such as a UDID-named subdirectory of a synthetic canonical
// root in the Backups tests.
func buildBackupIn(t *testing.T, dir string, opts ...backupOption) string {
	t.Helper()

	cfg := backupConfig{
		deviceName:     "Jorge's iPhone",
		productType:    "iPhone14,2",
		productVersion: "17.4",
		lastBackup:     time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	writePlist(t, filepath.Join(dir, "Manifest.plist"), map[string]any{
		"IsEncrypted": cfg.encrypted,
	})
	writePlist(t, filepath.Join(dir, "Info.plist"), map[string]any{
		"Device Name":      cfg.deviceName,
		"Product Type":     cfg.productType,
		"Product Version":  cfg.productVersion,
		"Last Backup Date": cfg.lastBackup,
	})
	// Status.plist only ever needs to exist, per describeBackup; its content is never
	// parsed by this package.
	writePlist(t, filepath.Join(dir, "Status.plist"), map[string]any{"BackupState": "new"})

	buildManifestDB(t, dir, cfg.files)
	return dir
}

// writePlist marshals v as a binary property list, the format Apple's own tools use,
// and writes it to path.
func writePlist(t *testing.T, path string, v any) {
	t.Helper()
	data, err := plist.Marshal(v, plist.BinaryFormat)
	if err != nil {
		t.Fatalf("marshalling %s: %v", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing %s: %v", filepath.Base(path), err)
	}
}

// buildManifestDB creates dir/Manifest.db with the real Files table shape and one row
// per fixtureFile. A non-directory entry also gets its content written to
// <dir>/<fileID[:2]>/<fileID>, exactly where the production code looks for it.
func buildManifestDB(t *testing.T, dir string, files []fixtureFile) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "Manifest.db"))
	if err != nil {
		t.Fatalf("opening Manifest.db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE Files (
		fileID TEXT PRIMARY KEY,
		domain TEXT,
		relativePath TEXT,
		flags INTEGER,
		file BLOB
	)`); err != nil {
		t.Fatalf("creating the Files table: %v", err)
	}

	for _, f := range files {
		id := fileIDOf(f.domain, f.relativePath)

		flags := 1
		var blob []byte
		switch {
		case f.isDir:
			flags = flagDirectory
		default:
			blob = buildMBFileArchive(t, int64(len(f.content)))
			shard := filepath.Join(dir, id[:2])
			if err := os.MkdirAll(shard, 0o700); err != nil {
				t.Fatalf("creating shard directory: %v", err)
			}
			if err := os.WriteFile(filepath.Join(shard, id), f.content, 0o600); err != nil {
				t.Fatalf("writing %s %s to disk: %v", f.domain, f.relativePath, err)
			}
		}

		if _, err := db.Exec(
			`INSERT INTO Files (fileID, domain, relativePath, flags, file) VALUES (?, ?, ?, ?, ?)`,
			id, f.domain, f.relativePath, flags, blob); err != nil {
			t.Fatalf("inserting %s %s: %v", f.domain, f.relativePath, err)
		}
	}
}

// rawRow is one Files row built without the usual MBFile encoding, for tests that
// need to inject a specific (possibly malformed) blob rather than a well-formed one.
type rawRow struct {
	domain       string
	relativePath string
	flags        int64
	blob         []byte
}

// buildRawManifestDB creates a standalone Manifest.db, outside of a full backup
// folder, with exactly the rows given. It exists for tests that need to control the
// "file" blob's bytes directly.
func buildRawManifestDB(t *testing.T, dir string, rows []rawRow) string {
	t.Helper()

	path := filepath.Join(dir, "Manifest.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("opening Manifest.db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE Files (
		fileID TEXT PRIMARY KEY,
		domain TEXT,
		relativePath TEXT,
		flags INTEGER,
		file BLOB
	)`); err != nil {
		t.Fatalf("creating the Files table: %v", err)
	}

	for _, r := range rows {
		id := fileIDOf(r.domain, r.relativePath)
		if _, err := db.Exec(
			`INSERT INTO Files (fileID, domain, relativePath, flags, file) VALUES (?, ?, ?, ?, ?)`,
			id, r.domain, r.relativePath, r.flags, r.blob); err != nil {
			t.Fatalf("inserting %s %s: %v", r.domain, r.relativePath, err)
		}
	}
	return path
}

// openManifestDB opens dir/Manifest.db the same way production code does: read-only.
func openManifestDB(t *testing.T, dir string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "Manifest.db")+"?mode=ro")
	if err != nil {
		t.Fatalf("opening Manifest.db: %v", err)
	}
	return db
}

// buildKeyedArchive wraps fields as the root object of a minimal NSKeyedArchiver
// document: the same flat, UID-referenced shape ("$archiver", "$version", "$top",
// "$objects") Apple's own archiver produces, so parseMBFile is exercised against its
// real decode path rather than a shortcut.
func buildKeyedArchive(tb testing.TB, fields map[string]any) []byte {
	tb.Helper()

	root := map[string]any{"$class": plist.UID(2)}
	for k, v := range fields {
		root[k] = v
	}
	doc := map[string]any{
		"$archiver": "NSKeyedArchiver",
		"$version":  uint64(100000),
		"$top":      map[string]any{"root": plist.UID(1)},
		"$objects": []any{
			"$null",
			root,
			map[string]any{"$classname": "MBFile", "$classes": []any{"MBFile", "NSObject"}},
		},
	}
	data, err := plist.Marshal(doc, plist.BinaryFormat)
	if err != nil {
		tb.Fatalf("marshalling a synthetic keyed archive: %v", err)
	}
	return data
}

// buildMBFileArchive builds the bytes Manifest.db stores in a Files row's "file"
// column for a file of the given size.
func buildMBFileArchive(tb testing.TB, size int64) []byte {
	tb.Helper()
	return buildKeyedArchive(tb, map[string]any{
		"Size":    uint64(size), //nolint:gosec // a test fixture size is never negative
		"Mode":    uint64(0o100644),
		"UserID":  uint64(501),
		"GroupID": uint64(501),
	})
}

// buildSQLiteDB creates a throwaway SQLite database, runs statements against it, and
// returns its on-disk bytes: a real database file, for tests that need one as a
// fixture's content rather than as something they open directly.
func buildSQLiteDB(t *testing.T, statements ...string) []byte {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fixture.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("opening a fixture database: %v", err)
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("executing fixture SQL: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing a fixture database: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading a fixture database: %v", err)
	}
	return data
}

// buildWALPair builds the three files a SQLite database in WAL mode looks like when
// it is copied without being checkpointed first, which is exactly what a real iPhone
// backup does: it copies whatever files exist, without asking WhatsApp to close its
// database first. main holds one row, committed and checkpointed before wal activity
// began; wal (and its shm index) hold a second row that exists nowhere else. A reader
// of main alone would see only the first row — that gap is what ExtractDatabase's
// checkpoint step exists to close.
func buildWALPair(t *testing.T) (mainBytes, walBytes, shmBytes []byte) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "chat.sqlite")

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("opening the fixture database: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE msg (id INTEGER PRIMARY KEY, body TEXT)`); err != nil {
		t.Fatalf("creating the fixture table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO msg (body) VALUES ('checkpointed before the backup was taken')`); err != nil {
		t.Fatalf("inserting the fixture's first row: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the fixture database: %v", err)
	}

	// wal_autocheckpoint(0) stops SQLite from folding the log back on its own, which
	// is exactly the state a backup can catch a live app's database in.
	db2, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=wal_autocheckpoint(0)")
	if err != nil {
		t.Fatalf("reopening the fixture database in WAL mode: %v", err)
	}
	if _, err := db2.Exec(`INSERT INTO msg (body) VALUES ('written after the last checkpoint')`); err != nil {
		t.Fatalf("inserting the fixture's second row: %v", err)
	}

	var readErr error
	mainBytes, readErr = os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading the fixture's main file: %v", readErr)
	}
	walBytes, readErr = os.ReadFile(path + "-wal")
	if readErr != nil {
		t.Fatalf("reading the fixture's -wal file: %v", readErr)
	}
	shmBytes, readErr = os.ReadFile(path + "-shm")
	if readErr != nil {
		t.Fatalf("reading the fixture's -shm file: %v", readErr)
	}

	if err := db2.Close(); err != nil {
		t.Fatalf("closing the fixture database: %v", err)
	}
	return mainBytes, walBytes, shmBytes
}
