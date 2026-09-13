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
		earliest   time.Time
		latest     time.Time
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

	if len(unknown) > 0 {
		// Not a failure: an unrecognised code is carried through as unknown rather
		// than dropped. It is reported so a new WhatsApp release is noticed early.
		t.Logf("unrecognised type codes, worth adding to the mapping: %v", unknown)
	}
	if counted == 0 {
		t.Fatal("no messages were read from a real database")
	}
}
