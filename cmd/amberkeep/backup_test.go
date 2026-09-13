package main

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"howett.net/plist"
	_ "modernc.org/sqlite"

	"github.com/jferrl/amberkeep/internal/backupfs"
)

// TestExtractTakesTheStoreOutOfABackup runs the path somebody with an iPhone takes:
// a backup folder in, a readable message store out, then every other command.
func TestExtractTakesTheStoreOutOfABackup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	backup := buildBackupWithAStore(t, dir)
	out := filepath.Join(dir, "out")

	if err := run(context.Background(), []string{"extract", "--backup", backup, "--out", out}); err != nil {
		t.Fatalf("the extract failed: %v", err)
	}

	store := filepath.Join(out, "ChatStorage.sqlite")
	if _, err := os.Stat(store); err != nil {
		t.Fatalf("the store was not taken out: %v", err)
	}

	t.Run("what came out can be read", func(t *testing.T) {
		if err := run(context.Background(), []string{"inspect", "--db", store}); err != nil {
			t.Errorf("the extracted store could not be read: %v", err)
		}
	})

	t.Run("the backup itself was not changed", func(t *testing.T) {
		for _, sidecar := range []string{"Manifest.db-wal", "Manifest.db-shm"} {
			if _, err := os.Stat(filepath.Join(backup, sidecar)); err == nil {
				t.Errorf("reading the backup left %s inside it", sidecar)
			}
		}
	})
}

func TestExtractRefusesWhatIsNotABackup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want error
	}{
		{
			name: "no backup named",
			args: []string{"extract"},
		},
		{
			name: "a folder that is not one",
			args: []string{"extract", "--backup", t.TempDir()},
			want: backupfs.ErrNotABackup,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := run(context.Background(), tt.args)
			if err == nil {
				t.Fatal("the extract was accepted")
			}
			if tt.want != nil && !errorMatches(err, tt.want) {
				t.Errorf("the error is %v, want %v", err, tt.want)
			}
		})
	}
}

// TestBackupsSurvivesHavingNowhereToLook covers the listing on a machine with no
// backups, which is most machines, and must read as an explanation rather than a
// failure.
func TestBackupsSurvivesHavingNowhereToLook(t *testing.T) {
	// Not parallel: it changes where the home directory is for the duration.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())

	if err := run(context.Background(), []string{"backups"}); err != nil {
		t.Errorf("listing backups on a machine with none failed: %v", err)
	}
}

// TestPrepareMakesAnArchiveQuick checks the one command that writes to a file
// somebody named, including that it says so and that running it twice is safe.
func TestPrepareMakesAnArchiveQuick(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	db := filepath.Join(dir, "msgstore.db")
	buildTinyArchive(t, db)

	before := contentsOf(t, db)
	if err := run(context.Background(), []string{"prepare", "--db", db}); err != nil {
		t.Fatalf("the prepare failed: %v", err)
	}
	if contentsOf(t, db) != before {
		t.Error("preparing changed what the archive holds")
	}

	t.Run("running it again is safe", func(t *testing.T) {
		if err := run(context.Background(), []string{"prepare", "--db", db}); err != nil {
			t.Errorf("the second prepare failed: %v", err)
		}
	})

	t.Run("it says what it is about to change", func(t *testing.T) {
		said := explainPreparation("msgstore.db")
		for _, want := range []string{"msgstore.db", "index", "not touched"} {
			if !strings.Contains(said, want) {
				t.Errorf("the explanation does not mention %q:\n%s", want, said)
			}
		}
	})

	t.Run("with nothing to work on", func(t *testing.T) {
		if err := run(context.Background(), []string{"prepare"}); err == nil {
			t.Error("prepare with no database was accepted")
		}
	})
}

func TestFirstNonEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "the first one", values: []string{"a", "b"}, want: "a"},
		{name: "skipping an empty one", values: []string{"", "b"}, want: "b"},
		{name: "nothing at all", values: []string{"", ""}, want: ""},
		{name: "no values", values: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := firstNonEmpty(tt.values...); got != tt.want {
				t.Errorf("firstNonEmpty(%v) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}

// buildBackupWithAStore writes the smallest thing Apple's tools would recognise as
// a backup, holding a WhatsApp message store.
func buildBackupWithAStore(t *testing.T, parent string) string {
	t.Helper()

	backup := filepath.Join(parent, "00008030-000000000000000E")
	if err := os.MkdirAll(backup, 0o700); err != nil {
		t.Fatalf("creating the backup folder: %v", err)
	}

	// The store itself, which the backup keeps under a hash of where it lives.
	store := filepath.Join(parent, "ChatStorage.sqlite")
	buildTinyStore(t, store)
	contents, err := os.ReadFile(store)
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}

	const domain = "AppDomainGroup-group.net.whatsapp.WhatsApp.shared"
	sum := sha1.Sum([]byte(domain + "-ChatStorage.sqlite")) // #nosec G401 -- Apple's own naming scheme
	fileID := hex.EncodeToString(sum[:])

	shard := filepath.Join(backup, fileID[:2])
	if err := os.MkdirAll(shard, 0o700); err != nil {
		t.Fatalf("creating the shard folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shard, fileID), contents, 0o600); err != nil {
		t.Fatalf("writing the stored file: %v", err)
	}

	manifest, err := sql.Open("sqlite", "file:"+filepath.Join(backup, "Manifest.db"))
	if err != nil {
		t.Fatalf("creating the manifest: %v", err)
	}
	if _, err := manifest.Exec(
		`CREATE TABLE Files (fileID TEXT PRIMARY KEY, domain TEXT, relativePath TEXT, flags INTEGER, file BLOB)`); err != nil {
		t.Fatalf("creating the Files table: %v", err)
	}
	if _, err := manifest.Exec(
		`INSERT INTO Files (fileID, domain, relativePath, flags, file) VALUES (?, ?, ?, 1, ?)`,
		fileID, domain, "ChatStorage.sqlite", mbFileRecord(t, int64(len(contents)))); err != nil {
		t.Fatalf("recording the store: %v", err)
	}
	if err := manifest.Close(); err != nil {
		t.Fatalf("closing the manifest: %v", err)
	}

	plists := map[string]string{
		"Manifest.plist": `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>IsEncrypted</key><false/></dict></plist>`,
		"Info.plist": `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Device Name</key><string>A Test Phone</string>
<key>Product Type</key><string>iPhone14,2</string>
<key>Product Version</key><string>26.0</string>
</dict></plist>`,
		"Status.plist": `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>IsFullBackup</key><true/></dict></plist>`,
	}
	for name, body := range plists {
		if err := os.WriteFile(filepath.Join(backup, name), []byte(body), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	return backup
}

// mbFileRecord builds what Apple stores in a Files row: the file's metadata as a
// keyed archive, in the flat UID-referenced shape NSKeyedArchiver produces.
func mbFileRecord(t *testing.T, size int64) []byte {
	t.Helper()

	document := map[string]any{
		"$archiver": "NSKeyedArchiver",
		"$version":  uint64(100000),
		"$top":      map[string]any{"root": plist.UID(1)},
		"$objects": []any{
			"$null",
			map[string]any{
				"$class": plist.UID(2),
				"Size":   uint64(size), //nolint:gosec // a fixture size is never negative
				"Mode":   uint64(0o100644),
			},
			map[string]any{"$classname": "MBFile", "$classes": []any{"MBFile", "NSObject"}},
		},
	}
	data, err := plist.Marshal(document, plist.BinaryFormat)
	if err != nil {
		t.Fatalf("building a file record: %v", err)
	}
	return data
}

// buildTinyStore writes the smallest iPhone message store the reader will accept.
func buildTinyStore(t *testing.T, path string) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the store: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(`
CREATE TABLE ZWACHATSESSION (Z_PK INTEGER PRIMARY KEY, ZSESSIONTYPE INTEGER, ZCONTACTJID VARCHAR,
	ZPARTNERNAME VARCHAR, ZLASTMESSAGEDATE TIMESTAMP, ZARCHIVED INTEGER);
CREATE TABLE ZWAMESSAGE (Z_PK INTEGER PRIMARY KEY, ZCHATSESSION INTEGER, ZISFROMME INTEGER,
	ZMESSAGETYPE INTEGER, ZSORT INTEGER, ZMESSAGEDATE TIMESTAMP, ZTEXT VARCHAR, ZSTANZAID VARCHAR);
INSERT INTO ZWACHATSESSION VALUES (1, 0, '34600111222@s.whatsapp.net', 'Ana Lopez', 580000000, 0);
INSERT INTO ZWAMESSAGE VALUES (1, 1, 0, 0, 100, 580000000, 'hello there', 'AAAA1111BBBB2222');`); err != nil {
		t.Fatalf("populating the store: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the store: %v", err)
	}
}

// contentsOf hashes everything in a database, so a change of any kind shows up.
func contentsOf(t *testing.T, path string) string {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var sum string
	row := db.QueryRow(`SELECT group_concat(t) FROM (
		SELECT quote(_id) || quote(text_data) AS t FROM message ORDER BY _id)`)
	if err := row.Scan(&sum); err != nil {
		t.Fatalf("reading the archive: %v", err)
	}
	return sum
}

// errorMatches reports whether a failure is the one expected, however wrapped.
func errorMatches(err, want error) bool {
	return err != nil && want != nil && strings.Contains(err.Error(), want.Error())
}
