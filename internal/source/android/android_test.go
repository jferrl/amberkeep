package android

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jferrl/amberkeep/internal/model"
)

func TestOpenRejectsWhatItCannotRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		build   func(t *testing.T) string
		wantErr error
	}{
		{
			name:    "a file that does not exist",
			build:   func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.db") },
			wantErr: ErrUnreadable,
		},
		{
			name: "a file that is not a database",
			build: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "notadb.db")
				if err := os.WriteFile(path, []byte("this is not a database"), 0o600); err != nil {
					t.Fatalf("writing the fixture: %v", err)
				}
				return path
			},
			wantErr: ErrUnreadable,
		},
		{
			name: "a database that holds something else",
			build: func(t *testing.T) string {
				return buildBare(t, `CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT)`)
			},
			wantErr: ErrNotAMessageDatabase,
		},
		{
			name: "the pre-2021 layout",
			build: func(t *testing.T) string {
				return buildBare(t, `CREATE TABLE messages (
					_id INTEGER PRIMARY KEY, key_remote_jid TEXT, key_from_me INTEGER, data TEXT)`)
			},
			wantErr: ErrLegacyUnsupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := Open(context.Background(), tt.build(t))
			if err == nil {
				r.Close()
				t.Fatalf("Open() unexpectedly succeeded")
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

func TestReaderIdentifiesItsLayout(t *testing.T) {
	t.Parallel()

	if got := openFixture(t).Layout(); got != "modern" {
		t.Errorf("Layout() = %q, want %q", got, "modern")
	}
}

func TestChats(t *testing.T) {
	t.Parallel()

	chats, err := openFixture(t).Chats(context.Background())
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}

	byID := make(map[int64]model.Chat, len(chats))
	for _, c := range chats {
		byID[c.ID] = c
	}

	tests := []struct {
		name         string
		id           int64
		wantKind     model.ChatKind
		wantTitle    string
		wantMessages int
		wantIncluded bool
	}{
		{
			name:         "a direct conversation is named after the person",
			id:           chatAlice,
			wantKind:     model.ChatDirect,
			wantTitle:    "+34600111222",
			wantMessages: 8,
			wantIncluded: true,
		},
		{
			name:         "a group is named by its subject",
			id:           chatGroup,
			wantKind:     model.ChatGroup,
			wantTitle:    "Weekend plans",
			wantMessages: 3,
			wantIncluded: true,
		},
		{
			name:         "the status feed is recognised and left out by default",
			id:           chatStatus,
			wantKind:     model.ChatStatus,
			wantTitle:    "status@broadcast",
			wantMessages: 1,
			wantIncluded: false,
		},
		{
			// Carol is only ever named against her hidden identifier, yet this
			// conversation is addressed by her phone number. Resolving it proves the
			// two are linked, which is what keeps a modern archive from being full
			// of strangers.
			name:         "an empty conversation is reported but left out by default",
			id:           chatEmpty,
			wantKind:     model.ChatDirect,
			wantTitle:    "~Carol Q",
			wantMessages: 0,
			wantIncluded: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, ok := byID[tt.id]
			if !ok {
				t.Fatalf("conversation %d is missing from the list", tt.id)
			}
			if c.Kind != tt.wantKind {
				t.Errorf("Kind = %v, want %v", c.Kind, tt.wantKind)
			}
			if c.Title() != tt.wantTitle {
				t.Errorf("Title() = %q, want %q", c.Title(), tt.wantTitle)
			}
			if c.Messages != tt.wantMessages {
				t.Errorf("Messages = %d, want %d", c.Messages, tt.wantMessages)
			}
			if c.Includable() != tt.wantIncluded {
				t.Errorf("Includable() = %v, want %v", c.Includable(), tt.wantIncluded)
			}
		})
	}

	t.Run("conversations are ordered by most recent activity", func(t *testing.T) {
		for i := 1; i < len(chats); i++ {
			prev, cur := chats[i-1], chats[i]
			if cur.LastAt.IsZero() {
				continue
			}
			if prev.LastAt.Before(cur.LastAt) {
				t.Fatalf("conversation %d is newer than the one before it", cur.ID)
			}
		}
	})

	t.Run("a group lists its members with resolved names", func(t *testing.T) {
		group := byID[chatGroup]
		if len(group.Participants) != 2 {
			t.Fatalf("Participants = %d, want 2", len(group.Participants))
		}
		var admins int
		names := make(map[string]bool)
		for _, p := range group.Participants {
			names[p.Name] = true
			if p.Admin {
				admins++
			}
		}
		if admins != 1 {
			t.Errorf("admins = %d, want 1", admins)
		}
		// The hidden member resolves to the name they chose for themselves.
		if !names["~Carol Q"] {
			t.Errorf("the hidden member was not resolved; names were %v", names)
		}
	})
}

func TestMessages(t *testing.T) {
	t.Parallel()

	r := openFixture(t)
	chats, err := r.Chats(context.Background())
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}
	var direct, group model.Chat
	for _, c := range chats {
		switch c.ID {
		case chatAlice:
			direct = c
		case chatGroup:
			group = c
		}
	}

	collect := func(chat model.Chat) []model.Message {
		t.Helper()
		var out []model.Message
		for m, err := range r.Messages(context.Background(), chat) {
			if err != nil {
				t.Fatalf("Messages() failed: %v", err)
			}
			out = append(out, m)
		}
		return out
	}

	msgs := collect(direct)
	if len(msgs) != 8 {
		t.Fatalf("read %d messages, want 8", len(msgs))
	}

	byID := make(map[int64]model.Message, len(msgs))
	for _, m := range msgs {
		byID[m.ID] = m
	}

	t.Run("messages arrive in the order they were written", func(t *testing.T) {
		for i := 1; i < len(msgs); i++ {
			if msgs[i].SentAt.Before(msgs[i-1].SentAt) {
				t.Fatalf("message %d is older than the one before it", msgs[i].ID)
			}
		}
	})

	tests := []struct {
		name  string
		id    int64
		check func(t *testing.T, m model.Message)
	}{
		{
			name: "an incoming message names its sender",
			id:   1,
			check: func(t *testing.T, m model.Message) {
				if m.IsFromMe() {
					t.Error("IsFromMe() = true, want false")
				}
				if m.Sender.User != "34600111222" {
					t.Errorf("Sender = %q, want the contact's number", m.Sender.User)
				}
				if m.Text != "first message" {
					t.Errorf("Text = %q", m.Text)
				}
				if m.Key == "" {
					t.Error("Key is empty; merging depends on it")
				}
			},
		},
		{
			name: "an outgoing message has no sender",
			id:   2,
			check: func(t *testing.T, m model.Message) {
				if !m.IsFromMe() {
					t.Error("IsFromMe() = false, want true")
				}
				if !m.Sender.IsZero() {
					t.Errorf("Sender = %v, want none", m.Sender)
				}
			},
		},
		{
			name: "a reply carries what it answered",
			id:   2,
			check: func(t *testing.T, m model.Message) {
				if !m.IsReply() {
					t.Fatal("IsReply() = false, want true")
				}
				if m.Quote.Text != "first message" {
					t.Errorf("quoted text = %q", m.Quote.Text)
				}
				if m.Quote.Sender.User != "34600111222" {
					t.Errorf("quoted sender = %q", m.Quote.Sender.User)
				}
			},
		},
		{
			name: "a photo keeps its caption as the message text",
			id:   3,
			check: func(t *testing.T, m model.Message) {
				if m.Kind != model.KindImage {
					t.Errorf("Kind = %v, want image", m.Kind)
				}
				if m.Attachment == nil {
					t.Fatal("Attachment is missing")
				}
				if m.Attachment.FileName != "IMG-0001.jpg" {
					t.Errorf("FileName = %q", m.Attachment.FileName)
				}
				if m.Text != "at the beach" {
					t.Errorf("Text = %q, want the caption", m.Text)
				}
				if m.Attachment.Width != 1200 || m.Attachment.Height != 900 {
					t.Errorf("dimensions = %dx%d", m.Attachment.Width, m.Attachment.Height)
				}
			},
		},
		{
			name: "a recorded voice message is told apart from an audio file",
			id:   4,
			check: func(t *testing.T, m model.Message) {
				if m.Kind != model.KindVoice {
					t.Errorf("Kind = %v, want voice", m.Kind)
				}
				if m.Attachment == nil || m.Attachment.Duration.Seconds() != 7 {
					t.Errorf("duration was not carried across")
				}
			},
		},
		{
			name: "an attached audio file is not mistaken for a voice message",
			id:   5,
			check: func(t *testing.T, m model.Message) {
				if m.Kind != model.KindAudio {
					t.Errorf("Kind = %v, want audio", m.Kind)
				}
			},
		},
		{
			name: "a deleted message is marked rather than dropped",
			id:   6,
			check: func(t *testing.T, m model.Message) {
				if m.Kind != model.KindDeleted {
					t.Errorf("Kind = %v, want deleted", m.Kind)
				}
			},
		},
		{
			name: "a notice is classified as one",
			id:   7,
			check: func(t *testing.T, m model.Message) {
				if !m.Kind.IsNotice() {
					t.Errorf("Kind = %v, want a notice", m.Kind)
				}
			},
		},
		{
			name: "an edited message says so",
			id:   8,
			check: func(t *testing.T, m model.Message) {
				if !m.WasEdited() {
					t.Error("WasEdited() = false, want true")
				}
				if !m.EditedAt.After(m.SentAt) {
					t.Error("the edit is not recorded as later than the message")
				}
			},
		},
		{
			name: "reactions are attached without duplicating the message",
			id:   1,
			check: func(t *testing.T, m model.Message) {
				if len(m.Reactions) != 1 {
					t.Fatalf("Reactions = %d, want 1 (empty ones must be ignored)", len(m.Reactions))
				}
				if m.Reactions[0].Emoji != "👍" {
					t.Errorf("emoji = %q", m.Reactions[0].Emoji)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, ok := byID[tt.id]
			if !ok {
				t.Fatalf("message %d is missing", tt.id)
			}
			tt.check(t, m)
		})
	}

	t.Run("a hidden sender in a group resolves to a real person", func(t *testing.T) {
		for _, m := range collect(group) {
			if m.ID != 9 {
				continue
			}
			if name := r.Directory().NameOf(m.Sender); name != "~Carol Q" {
				t.Errorf("sender name = %q, want the name they chose", name)
			}
			return
		}
		t.Fatal("the hidden-sender message is missing")
	})

	t.Run("mentions are resolved", func(t *testing.T) {
		for _, m := range collect(group) {
			if m.ID != 10 {
				continue
			}
			if len(m.Mentions) != 1 {
				t.Fatalf("Mentions = %d, want 1", len(m.Mentions))
			}
			return
		}
		t.Fatal("the mentioning message is missing")
	})

	t.Run("iteration can stop early", func(t *testing.T) {
		var seen int
		for range r.Messages(context.Background(), direct) {
			seen++
			break
		}
		if seen != 1 {
			t.Errorf("read %d messages after breaking, want 1", seen)
		}
	})

	t.Run("a cancelled context stops the read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		for _, err := range r.Messages(ctx, direct) {
			if err == nil {
				t.Fatal("Messages() yielded a message after the context was cancelled")
			}
			return
		}
	})
}

func TestDirectoryResolution(t *testing.T) {
	t.Parallel()

	d := openFixture(t).Directory()

	tests := []struct {
		name string
		jid  model.JID
		want string
	}{
		{
			name: "a hidden identifier resolves to the name its owner chose",
			jid:  model.ParseJID("99887766554433@lid"),
			want: "~Carol Q",
		},
		{
			name: "the phone address behind it resolves to the same name",
			jid:  model.ParseJID("34600333444@s.whatsapp.net"),
			want: "~Carol Q",
		},
		{
			name: "someone with no name at all falls back to their number",
			jid:  model.ParseJID("34600111222@s.whatsapp.net"),
			want: "+34600111222",
		},
		{
			name: "an address the database never saw names itself",
			jid:  model.ParseJID("34699999999@s.whatsapp.net"),
			want: "+34699999999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := d.NameOf(tt.jid); got != tt.want {
				t.Errorf("NameOf() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("the archive owner is named", func(t *testing.T) {
		if got := d.Owner().DisplayName(); got != "Owner Name" {
			t.Errorf("owner = %q, want %q", got, "Owner Name")
		}
	})
}

// TestAddressBookNamesWin checks the rule that matters most to how an archive
// reads: a name the owner saved in their phone outranks the one the sender chose.
func TestAddressBookNamesWin(t *testing.T) {
	t.Parallel()

	r := openFixture(t)
	carol := model.ParseJID("34600333444@s.whatsapp.net")

	if got := r.Directory().NameOf(carol); got != "~Carol Q" {
		t.Fatalf("before the address book, NameOf() = %q", got)
	}
	r.Directory().Add(model.Contact{JID: carol, Name: "Carol from work"})
	if got := r.Directory().NameOf(carol); got != "Carol from work" {
		t.Errorf("after the address book, NameOf() = %q, want the saved name", got)
	}
}

// TestReadOnly is the guard behind the promise that a source database is never
// modified. Users hand this code the only copy of their history.
func TestReadOnly(t *testing.T) {
	t.Parallel()

	path := buildFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}

	r, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	chats, err := r.Chats(context.Background())
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}
	for _, c := range chats {
		for _, err := range r.Messages(context.Background(), c) {
			if err != nil {
				t.Fatalf("Messages() failed: %v", err)
			}
		}
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-reading the fixture: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("the source database changed while it was being read")
	}
	for _, sidecar := range []string{path + "-wal", path + "-shm", path + "-journal"} {
		if _, err := os.Stat(sidecar); err == nil {
			t.Errorf("reading left %s behind", filepath.Base(sidecar))
		}
	}
}

// buildBare writes a database containing only the given schema, for the cases where
// a reader must refuse to open something.
func buildBare(t *testing.T, schema string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "bare.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("creating the schema: %v", err)
	}
	return path
}
