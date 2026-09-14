package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
	"github.com/jferrl/amberkeep/internal/search"
)

// The contract between this server and the page it serves.
//
// `web/src/api/types.ts` is written by hand. Nothing checks it at runtime, on
// purpose: the program that serves the page is the program that built it, so a
// reply that does not match is a bug in this repository rather than something a
// browser should defend against. That is the right trade right up until the two
// halves drift apart, and they did — a backup's date became RFC 3339 while the page
// still expected something already formatted, and a list that is never null was
// written down as possibly null.
//
// So every reply the page can meet is recorded here, from the real handlers over a
// real archive and a real index, and the page's own test assigns each recording to
// the type it declares. A field this server stops sending, starts sending, or
// changes the type of stops the TypeScript build. Neither half can move without the
// other noticing.
//
// The recordings are TypeScript rather than JSON on purpose. A JSON import widens
// every string to `string`, which would leave the unions that matter most — a
// conversation's kind, a message's kind, the wizard's stage — unchecked. `as const`
// keeps them literal, so a kind this server invents and that page has never heard of
// is a compile error rather than a surprise at runtime.
//
// After changing a handler, run:
//
//	go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// and read the diff. It is the page's contract changing, and it belongs in the same
// commit as the change that caused it.

var update = flag.Bool("update", false, "rewrite the recorded replies")

// contractDir is where the recordings live: inside the page's own source, because
// that is what reads them, and it cannot import from outside its own tree.
var contractDir = filepath.Join("..", "..", "web", "src", "api", "contract")

func TestTheRecordedRepliesStillMatch(t *testing.T) {
	// Not parallel: with -update these write files, and the recordings are read by
	// name rather than by test.

	tests := []struct {
		name string
		// file is the recording's name, and what it says of itself in the page.
		file string
		what string
		// reply returns one recorded response body.
		reply func(t *testing.T) []byte
	}{
		{
			name:  "what the archive holds",
			file:  "archive.ts",
			what:  "GET /api/archive — what the archive holds.",
			reply: fetching("/api/archive"),
		},
		{
			name:  "a page of the conversation list",
			file:  "chats.ts",
			what:  "GET /api/chats — a page of the conversation list.",
			reply: fetching("/api/chats?limit=2"),
		},
		{
			// Two messages of two hundred, so there is a page before this one and
			// the field that says where it starts is present. A recording without it
			// would leave the end of a conversation unchecked.
			name: "a page of one conversation",
			file: "messages.ts",
			what: "GET /api/chats/{address}/messages — a page of one conversation, with a\n" +
				"page before it, so the field that says where that one starts is here to be checked.",
			reply: fetching("/api/chats/" + ana.String() + "/messages?limit=2"),
		},
		{
			// Every optional field of a message at once. No real message carries a
			// poll and a call and a deletion together, and the point is not to
			// describe one that could: it is that each field's shape is written down
			// somewhere the page is checked against.
			name: "a message carrying everything a message can carry",
			file: "message-with-everything.ts",
			what: "The same reply, carrying every optional field a message can carry.\n\n" +
				"No real message looks like this. Each field has to appear in some recording or\n" +
				"nothing checks its shape, and one message costs one recording instead of twenty.",
			reply: fetching("/api/chats/" + everything.JID.String() + "/messages?limit=1"),
		},
		{
			name:  "a page of search results",
			file:  "search.ts",
			what:  "GET /api/search — a page of results, each with the matched words marked.",
			reply: fetching("/api/search?q=four&limit=2"),
		},
		{
			name: "nothing is open yet",
			file: "state-empty.ts",
			what: "GET /api/state — nothing is open, which is where somebody with a dead phone\n" +
				"finds this program. Where things will be written is already known.",
			reply: wizardState(&helper{}, nil),
		},
		{
			name: "something is running",
			file: "state-working.ts",
			what: "GET /api/state — work is running. `detail` is the server's own words and is\n" +
				"shown as they are; it changes while the work runs, and is the only thing that does.",
			reply: func(t *testing.T) []byte {
				t.Helper()

				// Held open at the point where it has said what it is doing, which
				// is what somebody watching would be looking at.
				gate := make(chan struct{})
				t.Cleanup(func() { close(gate) })

				bring := &helper{gate: gate, says: []string{"Decrypting 236.4 MB. Nothing is being uploaded."}}
				handler := wizard(t, bring)
				begin(t, handler, "/api/decrypt", map[string]string{
					"file": "/backups/msgstore.db.crypt15", "key": "0123456789abcdef",
				})
				return await(t, handler, "working", func(body map[string]any) bool {
					return body["detail"] == bring.says[0]
				})
			},
		},
		{
			name: "there is an archive to read",
			file: "state-ready.ts",
			what: "GET /api/state — there is an archive. It describes itself here so the page\n" +
				"does not have to ask a second time.",
			reply: wizardState(&helper{produces: "/workspace/msgstore.db"}, func(t *testing.T, handler http.Handler) []byte {
				t.Helper()
				begin(t, handler, "/api/open", map[string]string{"path": "/workspace/msgstore.db"})
				return await(t, handler, "ready", nil)
			}),
		},
		{
			name: "it did not work",
			file: "state-failed.ts",
			what: "GET /api/state — it did not work. Failure is not the end of the road: the\n" +
				"next request works from the same screen.",
			reply: wizardState(&helper{failOpen: errors.New("this file is a database, but not one this version can read")},
				func(t *testing.T, handler http.Handler) []byte {
					t.Helper()
					begin(t, handler, "/api/open", map[string]string{"path": "/workspace/holiday.jpg"})
					return await(t, handler, "failed", nil)
				}),
		},
		{
			name: "the backups on this computer",
			file: "backups.ts",
			what: "GET /api/backups — what this computer has. The last of them is a backup so old\n" +
				"that Apple's index recorded almost nothing about it.",
			reply: recording(&helper{backups: []Backup{
				{
					Path:       "/Users/ana/Library/Application Support/MobileSync/Backup/00008030-0011",
					DeviceName: "Ana's iPhone", ProductType: "iPhone14,2", IOSVersion: "26.0",
					LastBackup: "2026-09-12T20:14:03Z",
				},
				{
					Path:       "/Users/ana/Library/Application Support/MobileSync/Backup/00008020-0022",
					DeviceName: "Old iPhone", LastBackup: "2019-03-02T08:41:00Z", Encrypted: true,
				},
				// A backup so old that Apple's index recorded almost nothing about
				// it. Everything absent here is a field the page must survive
				// without.
				{Path: "/Volumes/Backups/01234567-89ab"},
			}}, "/api/backups"),
		},
		{
			// An empty list and a folder this program was not allowed to look in are
			// the same emptiness and completely different situations.
			name: "the backups could not be looked at",
			file: "backups-refused.ts",
			what: "GET /api/backups — a folder this program was not allowed to look in. An empty\n" +
				"list and a refusal are the same emptiness and completely different situations.",
			reply: recording(&helper{problem: fullDiskAccess}, "/api/backups"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compare(t, tt.file, tt.what, tt.reply(t))
		})
	}
}

// fullDiskAccess is the refusal macOS produces, kept whole because the page shows
// it as it is.
const fullDiskAccess = "the backup could not be read because of a permissions restriction; " +
	"on macOS, grant access in System Settings > Privacy & Security > Full Disk Access\n\n" +
	"Add your terminal, or whichever program is running this, then try again."

// fetching records one reply from a server holding the fixture archive and an index
// over it.
func fetching(path string) func(*testing.T) []byte {
	return func(t *testing.T) []byte {
		t.Helper()
		return get(t, searchable(t), path)
	}
}

// recording answers one request of a server that has nothing open.
func recording(bring Importer, path string) func(*testing.T) []byte {
	return func(t *testing.T) []byte {
		t.Helper()
		return get(t, wizard(t, bring), path)
	}
}

// wizardState records the state a page polls, after doing whatever gets it there.
func wizardState(bring Importer, reach func(*testing.T, http.Handler) []byte) func(*testing.T) []byte {
	return func(t *testing.T) []byte {
		t.Helper()

		handler := wizard(t, bring)
		if reach == nil {
			return get(t, handler, "/api/state")
		}
		return reach(t, handler)
	}
}

// begin sends a wizard request and insists it was taken.
func begin(t *testing.T, handler http.Handler, path string, body map[string]string) {
	t.Helper()

	if status, answer := post(t, handler, path, body); status != http.StatusAccepted {
		t.Fatalf("POST %s returned %d: %v", path, status, answer)
	}
}

// await polls until the server reaches a stage, and returns that reply verbatim.
func await(t *testing.T, handler http.Handler, stage string, also func(map[string]any) bool) []byte {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		raw := get(t, handler, "/api/state")

		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("the state was not JSON: %v", err)
		}
		if body["stage"] == stage && (also == nil || also(body)) {
			return raw
		}
		if time.Now().After(deadline) {
			t.Fatalf("the server never reached %s: %s", stage, raw)
		}
		time.Sleep(time.Millisecond)
	}
}

// get returns one reply's body, insisting it arrived as JSON.
func get(t *testing.T, handler http.Handler, path string) []byte {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, http.NoBody))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s returned %d: %s", path, recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("GET %s came back as %q", path, got)
	}
	return recorder.Body.Bytes()
}

// compare checks one recording, or rewrites it.
func compare(t *testing.T, name, what string, reply []byte) {
	t.Helper()

	// Indented, because the diff is the thing a person reads when this fails.
	var body bytes.Buffer
	if err := json.Indent(&body, reply, "", "  "); err != nil {
		t.Fatalf("%s did not come back as JSON: %v\n%s", name, err, reply)
	}

	pretty := bytes.NewBufferString(preamble(what))
	pretty.Write(bytes.TrimRight(body.Bytes(), "\n"))
	pretty.WriteString(" as const;\n")

	path := filepath.Join(contractDir, name)
	if *update {
		if err := os.MkdirAll(contractDir, 0o750); err != nil {
			t.Fatalf("making room for the recordings: %v", err)
		}
		if err := os.WriteFile(path, pretty.Bytes(), 0o600); err != nil {
			t.Fatalf("recording %s: %v", name, err)
		}
		return
	}

	recorded, err := os.ReadFile(path) // #nosec G304 -- a name from the table above
	if err != nil {
		t.Fatalf("%s has never been recorded: %v\n\n"+
			"run: go test ./internal/api -run TestTheRecordedRepliesStillMatch -update", name, err)
	}
	if !bytes.Equal(recorded, pretty.Bytes()) {
		t.Errorf("what the server sends is no longer what the page is checked against.\n\n"+
			"recorded in %s:\n%s\nsent now:\n%s\n"+
			"If the change is intended, re-record it and update web/src/api/types.ts in the\n"+
			"same commit:\n\n"+
			"  go test ./internal/api -run TestTheRecordedRepliesStillMatch -update",
			path, recorded, pretty.Bytes())
	}
}

// preamble is what each recording says about itself.
//
// It is a generated file in somebody else's language, so it says so at the top: the
// next person to open one should know within a line that editing it achieves nothing.
func preamble(what string) string {
	return "// Recorded by internal/api/contract_test.go. Do not edit; re-record it:\n" +
		"//\n" +
		"//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update\n" +
		"//\n" +
		"// This is what the server actually sent. `as const` keeps every string literal,\n" +
		"// so the unions the page declares are checked rather than widened to `string`.\n" +
		"\n/**\n * " + strings.ReplaceAll(what, "\n", "\n * ") + "\n */\nexport default "
}

// searchable returns a server over the fixture archive with a real index over it.
//
// A real one rather than a stub: the search reply is assembled in the handler from
// what the index returns, so a recording made without one would be describing the
// test rather than the program.
func searchable(t *testing.T) http.Handler {
	t.Helper()

	archive := fixture()
	archive.chats = append(archive.chats, everything)
	archive.messages[everything.ID] = []model.Message{loaded}

	index, err := search.Build(t.Context(), archive,
		filepath.Join(t.TempDir(), "index"), "", search.Options{
			Names: archive.Directory(), Me: "You",
		})
	if err != nil {
		t.Fatalf("building the index: %v", err)
	}
	t.Cleanup(func() { _ = index.Close() })

	handler, err := New(context.Background(), archive, Options{
		Names: archive.Directory(), Location: time.UTC, Me: "You",
		Title: "Archive", Index: index,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	return handler
}

// everything is the conversation holding the message below.
var everything = model.Chat{
	ID: 4, JID: model.ParseJID("120363999@g.us"), Kind: model.ChatGroup,
	Name: "Everything at once", Messages: 1, LastAt: start.Add(2 * time.Hour),
	Description:  "A conversation that exists to be written down",
	Participants: []model.Participant{{JID: ana, Name: "Ana Lopez", Admin: true}},
	CreatedAt:    start.Add(-24 * time.Hour),
}

// loaded is a message carrying every optional field at once.
//
// Nothing real looks like this. That is the point: each field's shape has to be
// recorded somewhere for the page to be checked against it, and one message that
// carries all of them costs one recording instead of twenty.
var loaded = model.Message{
	ID: 7, ChatID: everything.ID, Key: "3EB0C431C26A1D4F", Kind: model.KindImage,
	SentAt: start.Add(2 * time.Hour), Sender: ana, PushName: "Ana",
	Text: "look at this",
	Attachment: &model.Attachment{
		MediaType: "image/jpeg", FileName: "beach.jpg", Size: 184320,
		Duration: 0, Width: 1600, Height: 1200, Caption: "look at this",
		Preview: model.Thumbnail{Data: []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0xff, 0xd9}},
	},
	Quote: &model.Quote{
		Sender: luis, Kind: model.KindText, Text: "where are you?",
	},
	Reactions: []model.Reaction{{Sender: luis, Emoji: "❤️", At: start.Add(2*time.Hour + time.Minute)}},
	Mentions:  []model.JID{luis},
	Place: &model.Place{
		Latitude: 41.3874, Longitude: 2.1686, Name: "Bogatell",
		Address: "Passeig Maritim", URL: "https://example.invalid/map", Live: true,
	},
	Poll: &model.Poll{
		Question: "Vermut on Saturday?",
		Options:  []model.PollOption{{Name: "Yes", Votes: 2}, {Name: "No", Votes: 0}},
		Closed:   true,
	},
	Call: &model.Call{Video: true, Group: true, Outcome: model.CallMissed, Duration: 92 * time.Second},
	Link: &model.LinkPreview{
		URL: "https://example.invalid/a", Title: "A page", Description: "That may no longer exist",
	},
	Contacts: []model.ContactCard{{
		Name:  "Luis",
		VCard: "BEGIN:VCARD\nVERSION:3.0\nFN:Luis\nEND:VCARD\n",
		JIDs:  []model.JID{luis},
	}},
	Invite:  &model.GroupInvite{GroupName: "Vermut", Group: group, ExpiresAt: start.Add(72 * time.Hour)},
	Deleted: &model.Deletion{At: start.Add(3 * time.Hour), By: luis},
	Notice: &model.Notice{
		Action: 12, Actor: ana, Targets: []model.JID{luis},
		Old: "Vermut", New: "Vermut del sabado", Subject: "Vermut del sabado",
	},
	SystemText:   "Ana Lopez changed the subject",
	AlbumSize:    3,
	Expires:      7 * 24 * time.Hour,
	Starred:      true,
	Forwarded:    true,
	ForwardScore: 5,
	EditedAt:     start.Add(2*time.Hour + 5*time.Minute),
	SourceType:   1,
}

// TestEveryFieldThePageDeclaresIsRecordedSomewhere is the gap this whole file could
// otherwise leave: a recording only checks the fields that are in it, so a field
// nothing exercises would be free to be wrong.
func TestEveryFieldThePageDeclaresIsRecordedSomewhere(t *testing.T) {
	t.Parallel()

	// The optional fields of a message, which is the shape with by far the most of
	// them and the one a reader actually looks at.
	want := []string{
		"key", "sender", "sender_name", "text", "attachment", "reply_to", "reactions",
		"mentions", "place", "poll", "call", "link", "contact_cards", "group_invite",
		"notice", "deleted", "album_size", "expires_after", "starred", "forwarded",
		"forward_score", "edited_at",
	}

	recorded, err := os.ReadFile(filepath.Join(contractDir, "message-with-everything.ts"))
	if err != nil {
		t.Skipf("nothing recorded yet: %v", err)
	}

	// The recording is TypeScript wrapped around the reply; the reply is the part
	// between the first brace and the last.
	body := recorded[bytes.IndexByte(recorded, '{') : bytes.LastIndexByte(recorded, '}')+1]

	var page struct {
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("the recording is not a page of messages: %v", err)
	}
	if len(page.Messages) != 1 {
		t.Fatalf("the recording holds %d messages, want the one carrying everything", len(page.Messages))
	}

	for _, field := range want {
		if _, present := page.Messages[0][field]; !present {
			t.Errorf("%q is declared by the page and appears in no recording, so nothing checks it", field)
		}
	}
}
