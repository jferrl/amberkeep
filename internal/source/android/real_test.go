package android

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

// TestAgainstARealDatabase reads a genuine decrypted message database when one is
// available. Synthetic fixtures prove the reader does what it was told to do; only
// a real database proves it was told the right thing, because real archives contain
// a decade of schema changes, type codes nobody documented, and conversations large
// enough for paging to matter.
//
// Real databases never enter the repository, so the path comes from the environment
// and the test skips without it:
//
//	AMBERKEEP_REAL_MSGSTORE=/path/to/msgstore.db go test ./internal/source/android/
//
// Nothing here reads message text. It asserts structure and counts, and prints a
// summary of what the reader made of the file.
func TestAgainstARealDatabase(t *testing.T) {
	path := os.Getenv("AMBERKEEP_REAL_MSGSTORE")
	if path == "" {
		t.Skip("no real database configured; set AMBERKEEP_REAL_MSGSTORE to run this")
	}

	ctx := context.Background()
	started := time.Now()

	r, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() failed on a real database: %v", err)
	}
	defer r.Close()

	chats, err := r.Chats(ctx)
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}
	if len(chats) == 0 {
		t.Fatal("a real database yielded no conversations")
	}

	var (
		counted    int
		unknown    = make(map[int]int)
		kinds      = make(map[model.Kind]int)
		byChatKind = make(map[model.ChatKind]int)
		included   int
		named      int
		withQuote  int
		withMedia  int
		withEmoji  int
		edited     int

		previews    int
		previewKB   int64
		places      int
		polls       int
		pollOptions int
		calls       int
		links       int
		cards       int
		invites     int
		deletions   int
		byAdmin     int
		forwards    int
		albums      int
		expiring    int
		notices     int
		phrased     int
		detailed    int
		noticeCodes = make(map[int]int)
		earliest    time.Time
		latest      time.Time
	)

	for _, c := range chats {
		byChatKind[c.Kind]++
		if !c.Includable() {
			continue
		}
		included++
		if c.Name != "" {
			named++
		}

		var (
			last  model.Message
			first = true
			seen  = make(map[int64]struct{})
		)
		for m, err := range r.Messages(ctx, c) {
			if err != nil {
				t.Fatalf("Messages() failed on conversation %d: %v", c.ID, err)
			}
			if _, repeated := seen[m.ID]; repeated {
				t.Fatalf("message %d was read twice in conversation %d", m.ID, c.ID)
			}
			seen[m.ID] = struct{}{}

			if !first && m.SentAt.Before(last.SentAt) {
				t.Fatalf("conversation %d is out of order at message %d", c.ID, m.ID)
			}
			if m.IsFromMe() && !m.Sender.IsZero() {
				t.Fatalf("message %d is both outgoing and attributed to someone", m.ID)
			}

			counted++
			kinds[m.Kind]++
			if m.Kind == model.KindUnknown {
				unknown[m.SourceType]++
			}
			if m.Quote != nil {
				withQuote++
			}
			if m.Attachment != nil {
				withMedia++
			}
			if len(m.Reactions) > 0 {
				withEmoji++
			}
			if m.WasEdited() {
				edited++
			}
			if m.Attachment != nil && m.Attachment.HasPreview() {
				previews++
				previewKB += int64(len(m.Attachment.Preview.Data))
			}
			if m.Place != nil {
				places++
			}
			if m.Poll != nil {
				polls++
				pollOptions += len(m.Poll.Options)
			}
			if m.Call != nil {
				calls++
			}
			if m.Link != nil {
				links++
			}
			if len(m.Contacts) > 0 {
				cards += len(m.Contacts)
			}
			if m.Invite != nil {
				invites++
			}
			if m.Deleted != nil {
				deletions++
				if m.Deleted.ByAdmin() {
					byAdmin++
				}
			}
			if m.Forwarded {
				forwards++
			}
			if m.AlbumSize > 0 {
				albums++
			}
			if m.IsDisappearing() {
				expiring++
			}
			if m.Notice != nil {
				notices++
				noticeCodes[m.Notice.Action]++
				if m.Notice.HasDetail() {
					detailed++
				}
				if m.SystemText != "" {
					phrased++
				}
			}
			if !m.SentAt.IsZero() {
				if earliest.IsZero() || m.SentAt.Before(earliest) {
					earliest = m.SentAt
				}
				if m.SentAt.After(latest) {
					latest = m.SentAt
				}
			}
			last, first = m, false
		}

		// The conversation list and the messages themselves must agree, or an export
		// would silently lose or repeat history.
		if len(seen) != c.Messages {
			t.Errorf("conversation %d lists %d messages but yielded %d", c.ID, c.Messages, len(seen))
		}
	}

	dir := r.Directory()
	t.Logf("layout: %s", r.Layout())
	t.Logf("conversations: %d total, %d worth exporting, %d with a name", len(chats), included, named)
	t.Logf("conversation kinds: %v", byChatKind)
	t.Logf("messages: %d read in %s", counted, time.Since(started).Round(time.Millisecond))
	t.Logf("people: %d known, %d with a human name", dir.Len(), dir.Identified())
	t.Logf("details: %d replies, %d attachments, %d with reactions, %d edited", withQuote, withMedia, withEmoji, edited)
	t.Logf("span: %s to %s", earliest.Format(time.DateOnly), latest.Format(time.DateOnly))
	t.Logf("kinds: %v", kinds)
	t.Logf("RECOVERED BEYOND PLAIN TEXT:")
	t.Logf("  picture previews: %d (%d MB of images that survived without their files)", previews, previewKB/(1<<20))
	t.Logf("  shared places: %d, polls: %d with %d answers, calls: %d", places, polls, pollOptions, calls)
	t.Logf("  link previews: %d, contact cards: %d, group invitations: %d", links, cards, invites)
	t.Logf("  deletions recorded: %d (%d by an administrator), forwarded: %d", deletions, byAdmin, forwards)
	t.Logf("  albums: %d, disappearing: %d", albums, expiring)
	t.Logf("  notices: %d, %d with recovered detail, %d phrased", notices, detailed, phrased)

	// A notice code nobody has identified is reported rather than quietly rendered
	// as a number, so the mapping can be extended from real archives.
	var unphrased int
	for code, n := range noticeCodes {
		if code != 0 {
			_ = code
		}
		_ = n
	}
	_ = unphrased
	t.Logf("  notice codes seen: %v", noticeCodes)

	if len(unknown) > 0 {
		// Not a failure: an unrecognised code is carried through as unknown rather
		// than dropped. It is reported so a new WhatsApp release is noticed early.
		t.Logf("unrecognised type codes, worth adding to the mapping: %v", unknown)
	}
	if counted == 0 {
		t.Fatal("no messages were read from a real database")
	}
}
