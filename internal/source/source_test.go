package source

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/jferrl/amberkeep/internal/source/android"
	"github.com/jferrl/amberkeep/internal/source/ios"
)

// TestOpenRecognisesWhatItWasHanded is what lets everything above this package be
// written once. Recognition is by content, because the name of a file says nothing:
// both stores arrive called whatever the user chose.
func TestOpenRecognisesWhatItWasHanded(t *testing.T) {
	t.Parallel()

	const androidSchema = `
CREATE TABLE jid (_id INTEGER PRIMARY KEY, user TEXT, server TEXT, raw_string TEXT);
CREATE TABLE chat (_id INTEGER PRIMARY KEY, jid_row_id INTEGER, subject TEXT);
CREATE TABLE message (_id INTEGER PRIMARY KEY, chat_row_id INTEGER, from_me INTEGER,
	key_id TEXT, sender_jid_row_id INTEGER, timestamp INTEGER, message_type INTEGER, text_data TEXT);`

	const iphoneSchema = `
CREATE TABLE ZWACHATSESSION (Z_PK INTEGER PRIMARY KEY, ZSESSIONTYPE INTEGER,
	ZCONTACTJID VARCHAR, ZPARTNERNAME VARCHAR, ZLASTMESSAGEDATE TIMESTAMP, ZARCHIVED INTEGER);
CREATE TABLE ZWAMESSAGE (Z_PK INTEGER PRIMARY KEY, ZCHATSESSION INTEGER, ZISFROMME INTEGER,
	ZMESSAGETYPE INTEGER, ZSORT INTEGER, ZMESSAGEDATE TIMESTAMP, ZTEXT VARCHAR, ZSTANZAID VARCHAR);`

	tests := []struct {
		name     string
		schema   string
		platform string
		fails    error
	}{
		{name: "an Android message database", schema: androidSchema, platform: "android"},
		{name: "an iPhone message store", schema: iphoneSchema, platform: "iphone"},
		{
			name:   "some other database entirely",
			schema: `CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT);`,
			fails:  ErrUnrecognised,
		},
		{
			name:   "a database with nothing in it",
			schema: `CREATE TABLE placeholder (x); DROP TABLE placeholder;`,
			fails:  ErrUnrecognised,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			archive, err := Open(context.Background(), build(t, tt.schema))
			if tt.fails != nil {
				if !errors.Is(err, tt.fails) {
					t.Fatalf("Open() error = %v, want %v", err, tt.fails)
				}
				return
			}
			if err != nil {
				t.Fatalf("Open() failed: %v", err)
			}
			defer func() { _ = archive.Close() }()

			if got := archive.Platform(); got != tt.platform {
				t.Errorf("the archive says it came from %q, want %q", got, tt.platform)
			}
			if archive.Layout() == "" {
				t.Error("the archive does not say what shape it is")
			}
			if _, err := archive.Chats(context.Background()); err != nil {
				t.Errorf("Chats() failed on a recognised archive: %v", err)
			}
		})
	}

	t.Run("a file that is not a database", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "something.db")
		if err := os.WriteFile(path, []byte("not a database"), 0o600); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
		if _, err := Open(context.Background(), path); !errors.Is(err, ErrUnrecognised) {
			t.Errorf("Open() error = %v, want it unrecognised", err)
		}
	})

	t.Run("a file that is not there", func(t *testing.T) {
		t.Parallel()
		if _, err := Open(context.Background(), filepath.Join(t.TempDir(), "absent.db")); err == nil {
			t.Error("Open() accepted a path with no file at it")
		}
	})
}

// TestFailuresFromEitherReaderSurviveTheJourney: a reader's own guidance must not
// be swallowed by the layer that chose it, or the advice attached to it is lost.
func TestFailuresFromEitherReaderSurviveTheJourney(t *testing.T) {
	t.Parallel()

	t.Run("an iPhone store missing its conversations", func(t *testing.T) {
		t.Parallel()

		path := build(t, `CREATE TABLE ZWAMESSAGE (Z_PK INTEGER PRIMARY KEY, ZCHATSESSION INTEGER);`)
		_, err := Open(context.Background(), path)
		if !errors.Is(err, ios.ErrNotAMessageStore) {
			t.Errorf("Open() error = %v, want the iPhone reader's own failure", err)
		}
	})

	t.Run("an Android database that is not the one with messages in it", func(t *testing.T) {
		t.Parallel()

		path := build(t, `CREATE TABLE messages (_id INTEGER PRIMARY KEY, data TEXT);`)
		_, err := Open(context.Background(), path)
		if !errors.Is(err, android.ErrLegacyUnsupported) && !errors.Is(err, android.ErrNotAMessageDatabase) {
			t.Errorf("Open() error = %v, want the Android reader's own failure", err)
		}
	})
}

// build writes a database with the given schema and returns its path.
func build(t *testing.T, schema string) string {
	t.Helper()
	return buildAt(t, filepath.Join(t.TempDir(), "archive.db"), schema)
}

// buildAt writes a database with the given schema where the caller wants it, for
// the tests that care what is around it.
func buildAt(t *testing.T, path, schema string) string {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the database: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("creating the schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the database: %v", err)
	}
	return path
}

// TestErrorCarriesItsGuidance keeps the advice attached to a failure from being
// lost when it is wrapped on the way out.
func TestErrorCarriesItsGuidance(t *testing.T) {
	t.Parallel()

	cause := errors.New("the underlying reason")
	wrapped := fmt.Errorf("while opening: %w", ErrUnrecognised.withCause(cause))

	if !errors.Is(wrapped, ErrUnrecognised) {
		t.Error("a wrapped failure no longer matches the sentinel it came from")
	}
	if !errors.Is(wrapped, cause) {
		t.Error("the underlying reason was lost")
	}
	if ErrUnrecognised.Error() == "" {
		t.Error("the sentinel has no message of its own")
	}
	if !strings.Contains(wrapped.Error(), "the underlying reason") {
		t.Errorf("the message does not say why: %q", wrapped.Error())
	}
	if errors.Is(errors.New("something else"), ErrUnrecognised) {
		t.Error("an unrelated error matched the sentinel")
	}
}

// TestAnArchiveFindsTheFilesBesideIt: a database on its own can show a thumbnail;
// a database with the phone's folder beside it can show the photograph. Nobody
// configures that, so the places it is looked for are the places people produce.
func TestAnArchiveFindsTheFilesBesideIt(t *testing.T) {
	t.Parallel()

	const androidSchema = `
CREATE TABLE jid (_id INTEGER PRIMARY KEY, user TEXT, server TEXT, raw_string TEXT);
CREATE TABLE chat (_id INTEGER PRIMARY KEY, jid_row_id INTEGER, subject TEXT);
CREATE TABLE message (_id INTEGER PRIMARY KEY, chat_row_id INTEGER, from_me INTEGER,
	key_id TEXT, sender_jid_row_id INTEGER, timestamp INTEGER, message_type INTEGER, text_data TEXT);`

	tests := []struct {
		name  string
		put   string // where to lay the phone's Media folder, from the database's own folder
		finds bool
	}{
		{name: "the database decrypted inside the WhatsApp folder", put: "Media", finds: true},
		{name: "the WhatsApp folder copied beside it", put: "WhatsApp/Media", finds: true},
		{name: "the database left in the Databases folder the phone keeps it in", put: "../Media", finds: true},
		{name: "a folder somewhere else entirely", put: "elsewhere/WhatsApp/Media", finds: false},
		{name: "nothing beside it at all", finds: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The database goes one level down, so a fixture can also put the
			// folder above it — which is where the phone itself keeps them.
			at := filepath.Join(t.TempDir(), "Databases")
			if err := os.MkdirAll(at, 0o750); err != nil {
				t.Fatalf("laying out the workspace: %v", err)
			}
			path := buildAt(t, filepath.Join(at, "msgstore.db"), androidSchema)
			beside := filepath.Dir(path)
			if tt.put != "" {
				// A Media folder is what makes a directory the phone's folder, and
				// the file inside it is what the archive would be asked for.
				under := filepath.Join(beside, filepath.FromSlash(tt.put), "WhatsApp Images")
				if err := os.MkdirAll(under, 0o750); err != nil {
					t.Fatalf("laying out the folder: %v", err)
				}
				if err := os.WriteFile(filepath.Join(under, "IMG-1.jpg"), []byte("a photograph"), 0o600); err != nil {
					t.Fatalf("writing the file: %v", err)
				}
			}

			archive, err := Open(context.Background(), path)
			if err != nil {
				t.Fatalf("Open() failed: %v", err)
			}
			defer func() { _ = archive.Close() }()

			files, can := archive.(interface {
				OpenMedia(string) (io.ReadSeekCloser, string, error)
			})
			if !can {
				t.Fatal("an Android archive cannot be asked for its files at all")
			}

			file, kind, err := files.OpenMedia("Media/WhatsApp Images/IMG-1.jpg")
			if !tt.finds {
				if err == nil {
					_ = file.Close()
					t.Fatal("the archive produced a file from a folder it should not have found")
				}
				return
			}
			if err != nil {
				t.Fatalf("OpenMedia() failed: %v", err)
			}
			defer func() { _ = file.Close() }()

			if kind != "image/jpeg" {
				t.Errorf("the file arrived as %q, want image/jpeg", kind)
			}
			body, err := io.ReadAll(file)
			if err != nil {
				t.Fatalf("reading the file: %v", err)
			}
			if string(body) != "a photograph" {
				t.Errorf("the file read as %q", body)
			}
		})
	}
}
