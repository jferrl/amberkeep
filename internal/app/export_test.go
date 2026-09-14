package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/api"
	"github.com/jferrl/amberkeep/internal/fixture"
	"github.com/jferrl/amberkeep/internal/source"
)

// Writing an archive out, against a real archive on disk.
//
// The API test above this covers the endpoint with a stand-in. This covers the work
// itself: that the formats asked for are the files produced, that an index appears
// only when there are pages for it to link to, and that a folder full of files is
// what somebody actually ends up with.

// archiveFor opens the small invented archive, wrapped the way the server holds one.
func archiveFor(t *testing.T) readable {
	t.Helper()

	path := filepath.Join(t.TempDir(), "msgstore.db")
	fixture.TinyArchive(t, path)

	reader, err := source.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })

	return readable{Archive: reader}
}

// wrote lists what landed in a folder, by extension.
func wrote(t *testing.T, dir string) map[string]int {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading what was written: %v", err)
	}
	by := map[string]int{}
	for _, e := range entries {
		by[filepath.Ext(e.Name())]++
	}
	return by
}

// twoPeopleAndAGroup writes an archive with enough in it to tell the switches apart.
//
// Local to these tests rather than added to internal/fixture: the shared archive is
// what several other suites measure against — the migration's among them — and
// giving it a group would move their numbers for a reason that has nothing to do
// with them. Everything here is invented and belongs to nobody.
func twoPeopleAndAGroup(t *testing.T) readable {
	t.Helper()

	path := filepath.Join(t.TempDir(), "msgstore.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the database: %v", err)
	}

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
INSERT INTO jid (_id, user, server, raw_string) VALUES
	(1, '34600111222', 's.whatsapp.net', '34600111222@s.whatsapp.net'),
	(2, '34600333444', 's.whatsapp.net', '34600333444@s.whatsapp.net'),
	(3, '120363001-1500000000', 'g.us', '120363001-1500000000@g.us');
INSERT INTO chat (_id, jid_row_id, subject) VALUES
	(1, 1, NULL), (2, 2, NULL), (3, 3, 'Vermut del sabado');
INSERT INTO message (_id, chat_row_id, from_me, key_id, sender_jid_row_id, timestamp, message_type, text_data)
	VALUES (1, 1, 0, 'K1', 1, %d, 0, 'hello there'),
	       (2, 1, 1, 'K2', NULL, %d, 0, 'hello back'),
	       (3, 2, 0, 'K3', 2, %d, 0, 'and another'),
	       (4, 3, 0, 'K4', 1, %d, 0, 'who is coming');`, at, at+60000, at+120000, at+180000)
	if _, err := db.Exec(data); err != nil {
		t.Fatalf("populating the database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the database: %v", err)
	}

	reader, err := source.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })

	return readable{Archive: reader}
}

func TestWritingAnArchiveOut(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		formats []string
		// want is how many files of each extension should appear.
		want map[string]int
		// index is whether an index page belongs in the answer.
		index bool
	}{
		{
			// Three conversations and the index that reaches them.
			name:    "web pages, with an index to reach them by",
			formats: []string{"html"},
			want:    map[string]int{".html": 4},
			index:   true,
		},
		{
			name:    "plain text, and no index because there is nothing to link to",
			formats: []string{"text"},
			want:    map[string]int{".txt": 3},
		},
		{
			name:    "all three at once, counted once each",
			formats: []string{"html", "text", "json"},
			want:    map[string]int{".html": 4, ".txt": 3, ".json": 3},
			index:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			archive := twoPeopleAndAGroup(t)
			into := filepath.Join(t.TempDir(), "archive")

			result, err := archive.Export(context.Background(), api.ExportRequest{
				Into: into, Formats: tt.formats, Groups: true, Me: "You", Location: time.UTC,
			}, func(api.Step, string, ...api.Count) {})
			if err != nil {
				t.Fatalf("Export() failed: %v", err)
			}

			got := wrote(t, into)
			for ext, count := range tt.want {
				if got[ext] != count {
					t.Errorf("%s files = %d, want %d (all of it: %v)", ext, got[ext], count, got)
				}
			}
			if result.Conversations == 0 || result.Messages == 0 {
				t.Errorf("it reported nothing written: %+v", result)
			}
			if result.Into != into {
				t.Errorf("into = %q, want %q", result.Into, into)
			}

			if tt.index {
				if _, err := os.Stat(filepath.Join(into, "index.html")); err != nil {
					t.Errorf("no index page to reach the conversations by: %v", err)
				}
			}
		})
	}
}

// TestOnlyWritesTheConversationsAskedFor covers somebody wanting one conversation
// rather than a whole history.
func TestOnlyWritesTheConversationsAskedFor(t *testing.T) {
	t.Parallel()

	archive := twoPeopleAndAGroup(t)
	chats, err := archive.Chats(context.Background())
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}
	if len(chats) < 2 {
		t.Skipf("the fixture has %d conversations; this needs two", len(chats))
	}

	into := filepath.Join(t.TempDir(), "one")
	result, err := archive.Export(context.Background(), api.ExportRequest{
		Into: into, Formats: []string{"text"}, Groups: true,
		Only: []string{chats[0].JID.String()}, Me: "You", Location: time.UTC,
	}, func(api.Step, string, ...api.Count) {})
	if err != nil {
		t.Fatalf("Export() failed: %v", err)
	}

	if result.Conversations != 1 {
		t.Errorf("it wrote %d conversations, want 1", result.Conversations)
	}
	if got := wrote(t, into)[".txt"]; got != 1 {
		t.Errorf("%d text files, want 1", got)
	}
}

// TestGroupsCanBeLeftOut covers the switch, because a group is the thing most likely
// to be enormous and least likely to be wanted.
func TestGroupsCanBeLeftOut(t *testing.T) {
	t.Parallel()

	archive := twoPeopleAndAGroup(t)
	with := filepath.Join(t.TempDir(), "with")
	without := filepath.Join(t.TempDir(), "without")

	all, err := archive.Export(context.Background(), api.ExportRequest{
		Into: with, Formats: []string{"text"}, Groups: true, Me: "You", Location: time.UTC,
	}, func(api.Step, string, ...api.Count) {})
	if err != nil {
		t.Fatalf("Export() with groups failed: %v", err)
	}

	alone, err := archive.Export(context.Background(), api.ExportRequest{
		Into: without, Formats: []string{"text"}, Groups: false, Me: "You", Location: time.UTC,
	}, func(api.Step, string, ...api.Count) {})
	if err != nil {
		t.Fatalf("Export() without groups failed: %v", err)
	}

	if alone.Conversations >= all.Conversations {
		t.Errorf("leaving groups out wrote %d of %d conversations; it changed nothing",
			alone.Conversations, all.Conversations)
	}
}

// TestAFormatThisDoesNotWriteIsRefused covers a page asking for something that does
// not exist, which should be a sentence rather than an empty folder.
func TestAFormatThisDoesNotWriteIsRefused(t *testing.T) {
	t.Parallel()

	archive := archiveFor(t)

	tests := []struct{ name, format string }{
		{"a format nobody has", "pdf"},
		{"an empty name", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := archive.Export(context.Background(), api.ExportRequest{
				Into: t.TempDir(), Formats: []string{tt.format}, Me: "You", Location: time.UTC,
			}, func(api.Step, string, ...api.Count) {})
			if err == nil {
				t.Fatal("it accepted a format it cannot write")
			}
			if !strings.Contains(err.Error(), "html") {
				t.Errorf("the refusal does not say what it can write: %v", err)
			}
		})
	}
}

// TestNoFormatsIsRefused covers the caller that asked for nothing at all.
func TestNoFormatsIsRefused(t *testing.T) {
	t.Parallel()

	archive := archiveFor(t)
	if _, err := archive.Export(context.Background(), api.ExportRequest{
		Into: t.TempDir(), Me: "You", Location: time.UTC,
	}, func(api.Step, string, ...api.Count) {}); err == nil {
		t.Fatal("it accepted a request naming no formats")
	}
}

// TestASayingNothingZoneIsTheMachineOwn covers the default that would otherwise
// silently shift every timestamp in an archive by hours.
func TestASayingNothingZoneIsTheMachineOwn(t *testing.T) {
	t.Parallel()

	archive := archiveFor(t)
	into := filepath.Join(t.TempDir(), "zoneless")

	if _, err := archive.Export(context.Background(), api.ExportRequest{
		Into: into, Formats: []string{"text"}, Groups: true, Me: "You",
	}, func(api.Step, string, ...api.Count) {}); err != nil {
		t.Fatalf("Export() with no zone failed: %v", err)
	}
	if got := wrote(t, into)[".txt"]; got == 0 {
		t.Error("nothing was written")
	}
}
