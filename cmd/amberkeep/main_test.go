package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/jferrl/amberkeep/internal/crypt15"
	"github.com/jferrl/amberkeep/internal/export"
	"github.com/jferrl/amberkeep/internal/source/android"
)

func TestParseFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
		fails bool
	}{
		{name: "one format", input: "text", want: []string{"text"}},
		{name: "several", input: "text,json", want: []string{"text", "json"}},
		{name: "spacing and case are forgiven", input: " TEXT , Json ", want: []string{"text", "json"}},
		{name: "the web page", input: "html", want: []string{"html"}},
		{name: "both still means the two it always meant", input: "both", want: []string{"text", "json"}},
		{name: "all means every format there is", input: "all", want: []string{"html", "text", "json"}},
		{name: "a name that does not exist", input: "pdf", fails: true},
		{name: "nothing at all", input: "", fails: true},
		{name: "only separators", input: ",,", fails: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseFormats(tt.input)
			if tt.fails {
				if err == nil {
					t.Fatalf("parseFormats(%q) was accepted", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFormats(%q) failed: %v", tt.input, err)
			}
			names := make([]string, len(got))
			for i, f := range got {
				names[i] = f.name
			}
			if strings.Join(names, ",") != strings.Join(tt.want, ",") {
				t.Errorf("parseFormats(%q) = %v, want %v", tt.input, names, tt.want)
			}
		})
	}
}

// TestParseZoneRefusesTheUnknown guards a quiet way to ruin an archive: reading
// timestamps in the wrong zone shifts every conversation by hours with nothing to
// show it happened, so an unrecognised name is refused rather than ignored.
func TestParseZoneRefusesTheUnknown(t *testing.T) {
	t.Parallel()

	if _, err := parseZone("Europe/Madrid"); err != nil {
		t.Errorf("a real time zone was refused: %v", err)
	}
	if loc, err := parseZone(""); err != nil || loc == nil {
		t.Errorf("the default time zone failed: %v", err)
	}
	if _, err := parseZone("Middle/Earth"); err == nil {
		t.Error("an unknown time zone was accepted, which would shift every timestamp silently")
	}
}

func TestDefaultOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{in: "/tmp/msgstore.db.crypt15", want: "/tmp/msgstore.db"},
		{in: "/tmp/msgstore.db.crypt14", want: "/tmp/msgstore.db"},
		{in: "backup/msgstore.db.crypt12", want: "backup/msgstore.db"},
		{in: "/tmp/something.bin", want: "/tmp/something.bin.decrypted"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if got := defaultOutput(tt.in); got != tt.want {
				t.Errorf("defaultOutput(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHumanSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		n    int64
		want string
	}{
		{n: 0, want: "0 bytes"},
		{n: 512, want: "512 bytes"},
		{n: 2048, want: "2.0 KB"},
		{n: 5 * 1024 * 1024, want: "5.0 MB"},
		{n: 3 * 1024 * 1024 * 1024, want: "3.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := humanSize(tt.n); got != tt.want {
				t.Errorf("humanSize(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

// TestReadKeyPrefersAFile checks both ways of supplying the key, and that a bad
// one never has its contents repeated back, in case the message is logged.
func TestReadKeyPrefersAFile(t *testing.T) {
	t.Parallel()

	const valid = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	t.Run("from a file, however it is spaced", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "key.txt")
		grouped := "0123 4567 89ab cdef 0123 4567 89ab cdef 0123 4567 89ab cdef 0123 4567 89ab cdef\n"
		if err := os.WriteFile(path, []byte(grouped), 0o600); err != nil {
			t.Fatalf("writing the key: %v", err)
		}
		if _, err := readKey(path); err != nil {
			t.Errorf("readKey() failed on a real key file: %v", err)
		}
	})

	t.Run("given directly", func(t *testing.T) {
		t.Parallel()
		if _, err := readKey(valid); err != nil {
			t.Errorf("readKey() failed on a key given directly: %v", err)
		}
	})

	t.Run("a file holding rubbish is reported without repeating it", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "key.txt")
		if err := os.WriteFile(path, []byte("this is not a key at all"), 0o600); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
		_, err := readKey(path)
		if err == nil {
			t.Fatal("readKey() accepted rubbish")
		}
		if !errors.Is(err, crypt15.ErrKeyFormat) {
			t.Errorf("readKey() error = %v, want a key format error", err)
		}
		if strings.Contains(err.Error(), "not a key at all") {
			t.Error("the error repeated the file's contents")
		}
	})

	t.Run("something that is neither is reported", func(t *testing.T) {
		t.Parallel()

		_, err := readKey("abc123")
		if err == nil {
			t.Fatal("readKey() accepted something that is neither a file nor a key")
		}
		if strings.Contains(err.Error(), "abc123") {
			t.Error("the error repeated what was passed, which may be a real key")
		}
	})
}

// TestEveryFailureCarriesAdvice is the guarantee behind the guided experience:
// no problem this tool already understands should send somebody to a search
// engine.
func TestEveryFailureCarriesAdvice(t *testing.T) {
	t.Parallel()

	failures := []error{
		crypt15.ErrKeyFormat,
		crypt15.ErrWrongKey,
		crypt15.ErrPasskeyProtected,
		crypt15.ErrCrypt14,
		crypt15.ErrMalformed,
		crypt15.ErrUnexpectedPlaintext,
		android.ErrNotAMessageDatabase,
		android.ErrLegacyUnsupported,
		android.ErrUnreadable,
		export.ErrExists,
	}

	for _, err := range failures {
		t.Run(err.Error()[:min(40, len(err.Error()))], func(t *testing.T) {
			t.Parallel()

			got := adviseOn(err)
			if got == "" {
				t.Errorf("no advice for %q", err)
			}
			if strings.Count(got, "\n") < 1 {
				t.Errorf("the advice for %q is a single line; it should say what to do", err)
			}
		})
	}

	t.Run("advice survives being wrapped", func(t *testing.T) {
		t.Parallel()

		wrapped := fmt.Errorf("exporting a conversation: %w", export.ErrExists)
		if adviseOn(wrapped) == "" {
			t.Error("wrapping an error lost its advice")
		}
	})

	t.Run("a failure nobody anticipated gets none", func(t *testing.T) {
		t.Parallel()
		if adviseOn(errors.New("the disk caught fire")) != "" {
			t.Error("advice was invented for an unknown failure")
		}
	})
}

func TestCommandDispatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		args  []string
		fails bool
	}{
		{name: "no arguments prints the guide", args: nil},
		{name: "help", args: []string{"help"}},
		{name: "the short help flag", args: []string{"-h"}},
		{name: "version", args: []string{"version"}},
		{name: "a command that does not exist", args: []string{"frobnicate"}, fails: true},
		{name: "decrypt with nothing to work on", args: []string{"decrypt"}, fails: true},
		{name: "inspect with nothing to work on", args: []string{"inspect"}, fails: true},
		{name: "export with nothing to work on", args: []string{"export"}, fails: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := run(context.Background(), tt.args)
			if tt.fails && err == nil {
				t.Errorf("run(%v) was accepted", tt.args)
			}
			if !tt.fails && err != nil {
				t.Errorf("run(%v) failed: %v", tt.args, err)
			}
		})
	}
}

// TestEndToEnd runs the whole path a person actually takes: a database in, an
// archive out, with names applied. It is the only test that proves the pieces fit
// together rather than merely working alone.
func TestEndToEnd(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	db := filepath.Join(dir, "msgstore.db")
	buildTinyArchive(t, db)

	book := filepath.Join(dir, "contacts.vcf")
	if err := os.WriteFile(book,
		[]byte("BEGIN:VCARD\nVERSION:3.0\nFN:Ana Lopez\nTEL:+34600111222\nEND:VCARD\n"), 0o600); err != nil {
		t.Fatalf("writing the address book: %v", err)
	}

	out := filepath.Join(dir, "archive")
	err := run(context.Background(), []string{
		"export", "--db", db, "--contacts", book, "--country", "34",
		"--timezone", "Europe/Madrid", "--out", out, "--format", "both",
	})
	if err != nil {
		t.Fatalf("the export failed: %v", err)
	}

	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatalf("reading the archive: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("the archive holds %d files, want a text and a structured one", len(entries))
	}

	var text string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".txt") {
			data, err := os.ReadFile(filepath.Join(out, e.Name()))
			if err != nil {
				t.Fatalf("reading the text archive: %v", err)
			}
			text = string(data)
		}
	}
	if text == "" {
		t.Fatal("no text archive was written")
	}

	t.Run("the address book named the conversation", func(t *testing.T) {
		if !strings.Contains(text, "Ana Lopez") {
			t.Errorf("the archive does not name the person\n---\n%s", text)
		}
	})
	t.Run("the messages are there", func(t *testing.T) {
		if !strings.Contains(text, "hello there") {
			t.Errorf("a message is missing\n---\n%s", text)
		}
	})
	t.Run("timestamps are in the zone that was asked for", func(t *testing.T) {
		// The message was sent at 18:46 universal time, which is 20:46 in Madrid.
		if !strings.Contains(text, "20:46") {
			t.Errorf("the timestamp was not converted\n---\n%s", text)
		}
	})

	t.Run("running again refuses to overwrite", func(t *testing.T) {
		// The same options matter: a conversation's file is named after the person,
		// so exporting without the address book would write a differently named
		// file rather than colliding with this one.
		err := run(context.Background(), []string{
			"export", "--db", db, "--contacts", book, "--country", "34",
			"--timezone", "Europe/Madrid", "--out", out, "--format", "both",
		})
		if !errors.Is(err, export.ErrExists) {
			t.Errorf("the second export error = %v, want it to refuse", err)
		}
		if adviseOn(err) == "" {
			t.Error("the refusal carried no advice about --force")
		}
	})

	t.Run("the web archive has an index that links to real files", func(t *testing.T) {
		// The whole archive again, as web pages, into a directory of its own.
		web := filepath.Join(dir, "web")
		err := run(context.Background(), []string{
			"export", "--db", db, "--contacts", book, "--country", "34",
			"--timezone", "Europe/Madrid", "--out", web, "--format", "html",
		})
		if err != nil {
			t.Fatalf("the web export failed: %v", err)
		}

		index, err := os.ReadFile(filepath.Join(web, export.IndexName))
		if err != nil {
			t.Fatalf("the archive has no index: %v", err)
		}
		page := string(index)

		if !strings.Contains(page, "Ana Lopez") {
			t.Error("the index does not name the conversation")
		}
		links := regexp.MustCompile(`href="([^"]+)"`).FindAllStringSubmatch(page, -1)
		if len(links) != 1 {
			t.Fatalf("the index has %d links, want one per conversation", len(links))
		}
		target, err := url.PathUnescape(html.UnescapeString(links[0][1]))
		if err != nil {
			t.Fatalf("the index wrote an unusable link: %q", links[0][1])
		}
		conversation, err := os.ReadFile(filepath.Join(web, target))
		if err != nil {
			t.Fatalf("the index links to a file that is not there: %v", err)
		}
		if !strings.Contains(string(conversation), "hello there") {
			t.Error("the conversation page is missing a message")
		}
	})

	t.Run("inspect reports without writing anything", func(t *testing.T) {
		before, err := os.ReadDir(out)
		if err != nil {
			t.Fatalf("reading the archive: %v", err)
		}
		if err := run(context.Background(), []string{"inspect", "--db", db, "--full"}); err != nil {
			t.Fatalf("inspect failed: %v", err)
		}
		after, err := os.ReadDir(out)
		if err != nil {
			t.Fatalf("reading the archive: %v", err)
		}
		if len(before) != len(after) {
			t.Error("inspect changed the archive")
		}
	})
}

// buildTinyArchive writes the smallest database the reader will accept, with two
// messages in one conversation.
func buildTinyArchive(t *testing.T, path string) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the database: %v", err)
	}
	defer db.Close()

	schema := `
CREATE TABLE jid (_id INTEGER PRIMARY KEY, user TEXT, server TEXT, raw_string TEXT);
CREATE TABLE chat (_id INTEGER PRIMARY KEY, jid_row_id INTEGER, subject TEXT);
CREATE TABLE message (
	_id INTEGER PRIMARY KEY, chat_row_id INTEGER, from_me INTEGER, key_id TEXT,
	sender_jid_row_id INTEGER, timestamp INTEGER, message_type INTEGER, text_data TEXT);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("creating the schema: %v", err)
	}

	at := time.Date(2026, 9, 12, 18, 46, 9, 0, time.UTC).UnixMilli()
	data := fmt.Sprintf(`
INSERT INTO jid (_id, user, server, raw_string)
	VALUES (1, '34600111222', 's.whatsapp.net', '34600111222@s.whatsapp.net');
INSERT INTO chat (_id, jid_row_id) VALUES (1, 1);
INSERT INTO message (_id, chat_row_id, from_me, key_id, sender_jid_row_id, timestamp, message_type, text_data)
	VALUES (1, 1, 0, 'K1', 1, %d, 0, 'hello there'),
	       (2, 1, 1, 'K2', NULL, %d, 0, 'hello back');`, at, at+60000)
	if _, err := db.Exec(data); err != nil {
		t.Fatalf("populating the database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the database: %v", err)
	}
}
