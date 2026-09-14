package ios

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

func TestOpenRefusesWhatIsNotAMessageStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		schema string
		want   error
	}{
		{
			name:   "an empty database",
			schema: "",
			want:   ErrNotAMessageStore,
		},
		{
			name:   "some other Core Data store",
			schema: `CREATE TABLE ZNOTE (Z_PK INTEGER PRIMARY KEY, ZTEXT VARCHAR);`,
			want:   ErrNotAMessageStore,
		},
		{
			name:   "the message table without its conversations",
			schema: `CREATE TABLE ZWAMESSAGE (Z_PK INTEGER PRIMARY KEY, ZCHATSESSION INTEGER);`,
			want:   ErrNotAMessageStore,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Open(context.Background(), buildBare(t, tt.schema))
			if !errors.Is(err, tt.want) {
				t.Errorf("Open() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("a file that is not a database at all", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "ChatStorage.sqlite")
		if err := os.WriteFile(path, []byte("this is not a database"), 0o600); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
		if _, err := Open(context.Background(), path); err == nil {
			t.Error("Open() accepted something that is not a database")
		}
	})

	t.Run("a file that is not there", func(t *testing.T) {
		t.Parallel()
		if _, err := Open(context.Background(), filepath.Join(t.TempDir(), "absent.sqlite")); err == nil {
			t.Error("Open() accepted a path with no file at it")
		}
	})
}

func TestChats(t *testing.T) {
	t.Parallel()

	r := openFixture(t)
	chats, err := r.Chats(context.Background())
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}

	byID := make(map[int64]model.Chat, len(chats))
	for _, c := range chats {
		byID[c.ID] = c
	}

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "every conversation is listed, including the ones an archive leaves out",
			check: func(t *testing.T) {
				if len(chats) != 4 {
					t.Errorf("Chats() returned %d conversations, want 4", len(chats))
				}
			},
		},
		{
			name: "a conversation is classified by its address, not by the store's own code",
			check: func(t *testing.T) {
				want := map[int64]model.ChatKind{
					sessionAna:   model.ChatDirect,
					sessionGroup: model.ChatGroup,
					sessionEmpty: model.ChatDirect,
					sessionMuted: model.ChatStatus,
				}
				for id, kind := range want {
					if got := byID[id].Kind; got != kind {
						t.Errorf("conversation %d is %v, want %v", id, got, kind)
					}
				}
			},
		},
		{
			name: "messages are counted rather than believed",
			check: func(t *testing.T) {
				// The fixture claims 99 for this conversation, as a real device does.
				if got := byID[sessionAna].Messages; got != 4 {
					t.Errorf("the conversation holds %d messages, want 4", got)
				}
				if got := byID[sessionEmpty].Messages; got != 0 {
					t.Errorf("an empty conversation holds %d messages, want 0", got)
				}
			},
		},
		{
			name: "an empty conversation and the status feed stay out of an archive",
			check: func(t *testing.T) {
				if byID[sessionEmpty].Includable() {
					t.Error("a conversation with no messages would be archived")
				}
				if byID[sessionMuted].Includable() {
					t.Error("the status feed would be archived")
				}
				if !byID[sessionAna].Includable() {
					t.Error("a real conversation would be left out")
				}
			},
		},
		{
			name: "the name the phone's address book gave somebody is used",
			check: func(t *testing.T) {
				if got := byID[sessionAna].Title(); got != "Ana Lopez" {
					t.Errorf("the conversation is called %q, want the saved name", got)
				}
			},
		},
		{
			name: "a group carries its members, its subject and when it was made",
			check: func(t *testing.T) {
				group := byID[sessionGroup]
				if got := len(group.Participants); got != 3 {
					t.Errorf("the group lists %d members, want 3", got)
				}
				if group.Title() != "Vermut del sabado" {
					t.Errorf("the group is called %q", group.Title())
				}
				if group.CreatedAt.Year() != 2017 {
					t.Errorf("the group was made in %v, want 2017", group.CreatedAt)
				}
				if !group.Archived {
					t.Error("an archived group is not marked as one")
				}
			},
		},
		{
			name: "conversations arrive most recently used first",
			check: func(t *testing.T) {
				for i := 1; i < len(chats); i++ {
					if chats[i].LastAt.After(chats[i-1].LastAt) {
						t.Fatalf("conversation %d is newer than the one before it", i)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { tt.check(t) })
	}
}

// TestDirectoryPrefersTheSavedName: an iPhone carries the phone's address book
// inside the store, which is why an archive read from one needs no separate export
// of contacts. The name somebody chose for themselves must not win over it.
func TestDirectoryPrefersTheSavedName(t *testing.T) {
	t.Parallel()

	r := openFixture(t)
	if _, err := r.Chats(context.Background()); err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}

	tests := []struct {
		name string
		jid  string
		want string
	}{
		{name: "a saved name beats a chosen one", jid: anaJID, want: "Ana Lopez"},
		{name: "a member's saved name", jid: luisJID, want: "Luis"},
		{name: "a first name is used when nothing better was saved", jid: hiddenID, want: "Marta"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.Directory().NameOf(model.ParseJID(tt.jid)); got != tt.want {
				t.Errorf("the name for %s is %q, want %q", tt.jid, got, tt.want)
			}
		})
	}
}

func TestMessages(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := openFixture(t)
	chats, err := r.Chats(ctx)
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}

	var direct, group model.Chat
	for _, c := range chats {
		switch c.ID {
		case sessionAna:
			direct = c
		case sessionGroup:
			group = c
		}
	}

	collect := func(chat model.Chat) map[int64]model.Message {
		t.Helper()
		out := make(map[int64]model.Message)
		for m, err := range r.Messages(ctx, chat) {
			if err != nil {
				t.Fatalf("Messages() failed: %v", err)
			}
			out[m.ID] = m
		}
		return out
	}

	one := collect(direct)
	many := collect(group)

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "every message is read",
			check: func(t *testing.T) {
				if len(one) != 4 {
					t.Errorf("the conversation yielded %d messages, want 4", len(one))
				}
				if len(many) != 8 {
					t.Errorf("the group yielded %d messages, want 8", len(many))
				}
			},
		},
		{
			name: "who sent what",
			check: func(t *testing.T) {
				if one[1].IsFromMe() {
					t.Error("an incoming message is marked as mine")
				}
				if !one[2].IsFromMe() {
					t.Error("an outgoing message is not marked as mine")
				}
				// In a one-to-one conversation the store often records no sender,
				// because there is only one person it could be.
				if got := one[1].Sender.String(); got != anaJID {
					t.Errorf("the sender is %q, want %q", got, anaJID)
				}
			},
		},
		{
			name: "a group message's sender comes from its member row",
			check: func(t *testing.T) {
				if got := many[5].Sender.String(); got != luisJID {
					t.Errorf("the sender is %q, want %q", got, luisJID)
				}
				if got := many[5].PushName; got != "Luisito" {
					t.Errorf("the name the sender had at the time is %q", got)
				}
			},
		},
		{
			name: "timestamps are read in the store's own epoch",
			check: func(t *testing.T) {
				want := time.Date(2019, 6, 14, 9, 12, 0, 0, time.UTC)
				if !one[1].SentAt.Equal(want) {
					t.Errorf("the message is dated %v, want %v", one[1].SentAt.UTC(), want)
				}
			},
		},
		{
			name: "a reply is recovered from the protobuf beside it",
			check: func(t *testing.T) {
				reply := one[3]
				if !reply.IsReply() {
					t.Fatal("a reply was not recognised, which is what ZPARENTMESSAGE would have missed")
				}
				if got := reply.Quote.Text; got != "are you up?" {
					t.Errorf("it answers %q, want the quoted words", got)
				}
				if got := reply.Quote.Sender.String(); got != anaJID {
					t.Errorf("it answers %q, want the quoted sender", got)
				}
			},
		},
		{
			name: "a photograph keeps what is known about the file",
			check: func(t *testing.T) {
				photo := one[4]
				if photo.Kind != model.KindImage {
					t.Errorf("a photograph is a %v", photo.Kind)
				}
				if photo.Attachment == nil {
					t.Fatal("the photograph carries nothing about its file")
				}
				if got := photo.Attachment.FileName; got != "IMG-0007.jpg" {
					t.Errorf("the file is called %q, want just the name", got)
				}
				if photo.Attachment.Size != 51234 {
					t.Errorf("the file size is %d", photo.Attachment.Size)
				}
				if photo.Attachment.Caption != "the beach" {
					t.Errorf("the caption is %q", photo.Attachment.Caption)
				}
			},
		},
		{
			name: "a shared place",
			check: func(t *testing.T) {
				place := many[6]
				if place.Place == nil {
					t.Fatal("a shared place was lost")
				}
				if place.Place.Name != "Casa Blanca" || !place.Place.HasCoordinates() {
					t.Errorf("the place is %+v", place.Place)
				}
			},
		},
		{
			name: "a shared card is kept exactly as it was sent",
			check: func(t *testing.T) {
				card := many[7]
				if len(card.Contacts) != 1 {
					t.Fatalf("the card was lost: %+v", card.Contacts)
				}
				if card.Contacts[0].Name != "Marta Ruiz" {
					t.Errorf("the card names %q", card.Contacts[0].Name)
				}
				if card.Contacts[0].VCard == "" {
					t.Error("the card itself was not kept")
				}
			},
		},
		{
			name: "a type number this build does not know is read from its media type",
			check: func(t *testing.T) {
				doc := many[8]
				if doc.Kind != model.KindDocument {
					t.Errorf("a PDF of an unknown type is a %v, want a document", doc.Kind)
				}
				if doc.SourceType != 91 {
					t.Errorf("the original type number was lost: %d", doc.SourceType)
				}
				if doc.Attachment == nil || doc.Attachment.FileName != "plan.pdf" {
					t.Errorf("the document's name was lost: %+v", doc.Attachment)
				}
			},
		},
		{
			name: "a deletion records that something was said, and by whom it was removed",
			check: func(t *testing.T) {
				gone := many[9]
				if !gone.WasDeleted() {
					t.Fatal("a deleted message is not marked as one")
				}
				if !gone.Deleted.ByAdmin() {
					t.Error("who removed it was lost")
				}
			},
		},
		{
			name: "a notice keeps the store's own code rather than an invented sentence",
			check: func(t *testing.T) {
				notice := many[10]
				if notice.Kind != model.KindSystem {
					t.Errorf("a notice is a %v", notice.Kind)
				}
				if notice.Notice == nil || notice.Notice.Action != 12 {
					t.Errorf("the notice's code was lost: %+v", notice.Notice)
				}
				if notice.SystemText == "" {
					t.Error("the words the store wrote were lost")
				}
			},
		},
		{
			name: "what a page said when it was shared",
			check: func(t *testing.T) {
				link := many[11]
				if link.Link == nil {
					t.Fatal("the link preview was lost")
				}
				if link.Link.Title != "Casa Blanca" || link.Link.URL == "" {
					t.Errorf("the preview is %+v", link.Link)
				}
			},
		},
		{
			name: "a starred message stays starred",
			check: func(t *testing.T) {
				if !one[2].Starred {
					t.Error("a starred message lost its star")
				}
			},
		},
		{
			name: "messages arrive in the order they were written",
			check: func(t *testing.T) {
				var last time.Time
				for m, err := range r.Messages(ctx, group) {
					if err != nil {
						t.Fatalf("Messages() failed: %v", err)
					}
					if m.SentAt.Before(last) {
						t.Fatalf("message %d is older than the one before it", m.ID)
					}
					last = m.SentAt
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { tt.check(t) })
	}
}

// TestPagingMatchesStreaming is the guarantee a viewer rests on: reading a
// conversation backwards a page at a time must produce exactly what reading it
// straight through produces.
func TestPagingMatchesStreaming(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := openFixture(t)
	chats, err := r.Chats(ctx)
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}

	var group model.Chat
	for _, c := range chats {
		if c.ID == sessionGroup {
			group = c
		}
	}

	var streamed []int64
	for m, err := range r.Messages(ctx, group) {
		if err != nil {
			t.Fatalf("Messages() failed: %v", err)
		}
		streamed = append(streamed, m.ID)
	}

	for _, size := range []int{1, 2, 3, 100} {
		t.Run("pages of "+string(rune('0'+min(size, 9))), func(t *testing.T) {
			var (
				paged  []int64
				cursor model.Cursor
			)
			for range len(streamed) + 2 {
				page, next, err := r.Page(ctx, group, cursor, size)
				if err != nil {
					t.Fatalf("Page() failed: %v", err)
				}
				if len(page) == 0 {
					break
				}
				ids := make([]int64, 0, len(page))
				for _, m := range page {
					ids = append(ids, m.ID)
				}
				paged = append(ids, paged...)
				if next.IsZero() {
					break
				}
				cursor = next
			}

			if len(paged) != len(streamed) {
				t.Fatalf("paging saw %d messages, streaming saw %d", len(paged), len(streamed))
			}
			for i := range paged {
				if paged[i] != streamed[i] {
					t.Errorf("message %d is %d when paged and %d when streamed", i, paged[i], streamed[i])
				}
			}
		})
	}

	t.Run("a page carries the same content as the stream", func(t *testing.T) {
		page, _, err := r.Page(ctx, group, model.Cursor{}, len(streamed))
		if err != nil {
			t.Fatalf("Page() failed: %v", err)
		}
		var withContent int
		for _, m := range page {
			if m.HasRecoveredContent() {
				withContent++
			}
		}
		if withContent == 0 {
			t.Error("a page recovered nothing beyond words, though the stream does")
		}
	})
}

// TestReadOnly is the promise that matters most: a store handed to this package is
// never changed, even by the driver's own bookkeeping.
func TestReadOnly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := buildFixture(t)

	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("looking at the store: %v", err)
	}

	r, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	chats, err := r.Chats(ctx)
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}
	for _, chat := range chats {
		for _, err := range r.Messages(ctx, chat) {
			if err != nil {
				t.Fatalf("Messages() failed: %v", err)
			}
		}
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("looking at the store: %v", err)
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		t.Error("reading the store changed it")
	}

	for _, sidecar := range []string{path + "-wal", path + "-shm", path + "-journal"} {
		if _, err := os.Stat(sidecar); err == nil {
			t.Errorf("reading the store left %s beside it", filepath.Base(sidecar))
		}
	}
}

// TestErrorsCarryTheirGuidance: a failure is matched by what it means rather than
// by its wording, so wrapping one never loses the advice attached to it.
func TestErrorsCarryTheirGuidance(t *testing.T) {
	t.Parallel()

	sentinels := []*Error{ErrNotAMessageStore, ErrUnreadable, ErrLocked}

	for _, sentinel := range sentinels {
		t.Run(sentinel.Guidance, func(t *testing.T) {
			t.Parallel()

			cause := errors.New("the underlying reason")
			wrapped := fmt.Errorf("while reading: %w", sentinel.withCause(cause))

			if !errors.Is(wrapped, sentinel) {
				t.Error("a wrapped failure no longer matches the sentinel it came from")
			}
			if !errors.Is(wrapped, cause) {
				t.Error("the underlying reason was lost")
			}
			if sentinel.Error() == "" {
				t.Error("the sentinel has no message of its own")
			}
			if !strings.Contains(wrapped.Error(), "the underlying reason") {
				t.Errorf("the message does not say why: %q", wrapped.Error())
			}
		})
	}

	t.Run("two unrelated failures do not compare equal", func(t *testing.T) {
		t.Parallel()
		if errors.Is(ErrLocked, ErrUnreadable) {
			t.Error("an encrypted file matched an unreadable one")
		}
		if errors.Is(errors.New("something else"), ErrUnreadable) {
			t.Error("an unrelated error matched a sentinel")
		}
	})
}

// TestPagingWhenTheStoreCannotBeOrderedQuickly: ZSORT is what makes reading a
// conversation backwards cheap, and a store without it must still be readable,
// just slower. Nothing about what comes back may change.
func TestPagingWhenTheStoreCannotBeOrderedQuickly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := openFixture(t)
	chats, err := r.Chats(ctx)
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}

	var group model.Chat
	for _, c := range chats {
		if c.ID == sessionGroup {
			group = c
		}
	}

	withSort, _, err := r.Page(ctx, group, model.Cursor{}, 3)
	if err != nil {
		t.Fatalf("Page() failed: %v", err)
	}

	// Pretend the store has no sort column, which is the fallback path.
	delete(r.schema.columns[tableMessage], "ZSORT")
	withoutSort, _, err := r.Page(ctx, group, model.Cursor{}, 3)
	if err != nil {
		t.Fatalf("Page() failed without a sort column: %v", err)
	}

	if len(withSort) != len(withoutSort) {
		t.Fatalf("the fallback returned %d messages, the fast path %d", len(withoutSort), len(withSort))
	}
	for i := range withSort {
		if withSort[i].ID != withoutSort[i].ID {
			t.Errorf("message %d is %d on the fast path and %d on the fallback",
				i, withSort[i].ID, withoutSort[i].ID)
		}
	}
}

// TestPagingFromAMessageThatIsGone: a viewer can hold a position in a conversation
// that has since been read from a different store. Showing the end beats refusing.
func TestPagingFromAMessageThatIsGone(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := openFixture(t)
	chats, err := r.Chats(ctx)
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}

	var group model.Chat
	for _, c := range chats {
		if c.ID == sessionGroup {
			group = c
		}
	}

	page, _, err := r.Page(ctx, group, model.Cursor{SentAt: time.Now(), ID: 999999}, 3)
	if err != nil {
		t.Fatalf("Page() failed from a position that no longer exists: %v", err)
	}
	if len(page) == 0 {
		t.Error("a position that no longer exists returned nothing at all")
	}
}

func TestLayoutAndTimestamps(t *testing.T) {
	t.Parallel()

	t.Run("the layout is named", func(t *testing.T) {
		t.Parallel()
		if got := openFixture(t).Layout(); got != "core-data" {
			t.Errorf("Layout() = %q", got)
		}
	})

	t.Run("a time survives the trip into the store's epoch and back", func(t *testing.T) {
		t.Parallel()

		want := time.Date(2019, 6, 14, 9, 12, 0, 0, time.UTC)
		got := coreDataTime(sql.NullFloat64{Float64: coreDataSeconds(want), Valid: true})
		if !got.Equal(want) {
			t.Errorf("the time came back as %v, want %v", got, want)
		}
	})

	t.Run("a column nobody filled in is not a date in 2001", func(t *testing.T) {
		t.Parallel()

		for _, empty := range []sql.NullFloat64{{}, {Float64: 0, Valid: true}} {
			if got := coreDataTime(empty); !got.IsZero() {
				t.Errorf("an empty column became %v, want no date at all", got)
			}
		}
		if got := coreDataSeconds(time.Time{}); got != 0 {
			t.Errorf("no date became %v seconds", got)
		}
	})
}
