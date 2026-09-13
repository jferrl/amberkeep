package ios

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

// TestAgainstARealStore reads a real iPhone store when one is pointed at, because
// no fixture finds what real data finds.
//
// It is skipped by default and the path comes from the environment: a real store is
// every message somebody ever sent and must never be committed, here or anywhere.
//
//	AMBERKEEP_REAL_CHATSTORAGE=/path/to/ChatStorage.sqlite go test ./internal/source/ios/
func TestAgainstARealStore(t *testing.T) {
	path := os.Getenv("AMBERKEEP_REAL_CHATSTORAGE")
	if path == "" {
		t.Skip("set AMBERKEEP_REAL_CHATSTORAGE to check against a real iPhone store")
	}

	ctx := context.Background()
	started := time.Now()

	r, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer func() { _ = r.Close() }()

	chats, err := r.Chats(ctx)
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}

	var (
		messages, recovered, replies, named int
		byKind                              = map[model.ChatKind]int{}
		unknown                             = map[int]int{}
		earliest, latest                    time.Time
	)
	for _, chat := range chats {
		byKind[chat.Kind]++
		if chat.Kind == model.ChatDirect && r.Directory().Lookup(chat.JID).IsIdentified() {
			named++
		}
		if !chat.Includable() {
			continue
		}
		for m, err := range r.Messages(ctx, chat) {
			if err != nil {
				t.Fatalf("Messages() failed in %q: %v", chat.Kind, err)
			}
			messages++
			if m.HasRecoveredContent() {
				recovered++
			}
			if m.IsReply() {
				replies++
			}
			if m.Kind == model.KindUnknown {
				unknown[m.SourceType]++
			}
			if !m.SentAt.IsZero() {
				if earliest.IsZero() || m.SentAt.Before(earliest) {
					earliest = m.SentAt
				}
				if m.SentAt.After(latest) {
					latest = m.SentAt
				}
			}
		}
	}

	t.Logf("layout %s", r.Layout())
	t.Logf("%d conversations: %d direct, %d groups, %d status, %d broadcast",
		len(chats), byKind[model.ChatDirect], byKind[model.ChatGroup],
		byKind[model.ChatStatus], byKind[model.ChatBroadcast])
	t.Logf("%d direct conversations carry a name", named)
	t.Logf("%d messages read in %s", messages, time.Since(started).Round(time.Millisecond))
	t.Logf("%d carry content beyond words, %d are replies", recovered, replies)
	t.Logf("people known: %d, with a name: %d", r.Directory().Len(), r.Directory().Identified())
	if !earliest.IsZero() {
		t.Logf("span %s to %s", earliest.Format(time.DateOnly), latest.Format(time.DateOnly))
	}
	for code, n := range unknown {
		t.Logf("unrecognised type %d: %d messages", code, n)
	}

	if messages == 0 {
		t.Error("the store yielded no messages at all")
	}

	t.Run("paging backwards reads the same conversation", func(t *testing.T) {
		var biggest model.Chat
		for _, chat := range chats {
			if chat.Includable() && chat.Messages > biggest.Messages {
				biggest = chat
			}
		}
		if biggest.Messages == 0 {
			t.Skip("nothing to page through")
		}

		var streamed []int64
		for m, err := range r.Messages(ctx, biggest) {
			if err != nil {
				t.Fatalf("Messages() failed: %v", err)
			}
			streamed = append(streamed, m.ID)
		}

		var (
			paged  []int64
			cursor model.Cursor
			at     = time.Now()
		)
		for range len(streamed)/500 + 2 {
			page, next, err := r.Page(ctx, biggest, cursor, 500)
			if err != nil {
				t.Fatalf("Page() failed: %v", err)
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
		t.Logf("paged %d messages of the largest conversation in %s",
			len(paged), time.Since(at).Round(time.Millisecond))

		if len(paged) != len(streamed) {
			t.Errorf("paging saw %d messages, streaming saw %d", len(paged), len(streamed))
		}
		for i := range paged {
			if i < len(streamed) && paged[i] != streamed[i] {
				t.Fatalf("message %d differs: paged %d, streamed %d", i, paged[i], streamed[i])
			}
		}
	})
}
