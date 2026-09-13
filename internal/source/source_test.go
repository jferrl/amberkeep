package source

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

	path := filepath.Join(t.TempDir(), "archive.db")
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
