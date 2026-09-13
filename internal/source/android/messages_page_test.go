package android

import (
	"context"
	"testing"

	"github.com/jferrl/amberkeep/internal/model"
)

// TestPageReadsBackwardsFromTheEnd covers what looking at a conversation needs and
// streaming cannot give: the most recent messages first, then the ones before them,
// without reading the whole conversation to find them.
func TestPageReadsBackwardsFromTheEnd(t *testing.T) {
	t.Parallel()

	r := openFixture(t)
	ctx := context.Background()

	var chat model.Chat
	chats, err := r.Chats(ctx)
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}
	for _, c := range chats {
		if c.ID == chatAlice {
			chat = c
		}
	}

	// The whole conversation, in order, as the streaming reader sees it.
	var all []model.Message
	for m, err := range r.Messages(ctx, chat) {
		if err != nil {
			t.Fatalf("Messages() failed: %v", err)
		}
		all = append(all, m)
	}
	if len(all) < 4 {
		t.Fatalf("the fixture holds %d messages, too few to page through", len(all))
	}

	t.Run("the first page is the end of the conversation", func(t *testing.T) {
		page, _, err := r.Page(ctx, chat, model.Cursor{}, 2)
		if err != nil {
			t.Fatalf("Page() failed: %v", err)
		}
		if len(page) != 2 {
			t.Fatalf("Page() returned %d messages, want 2", len(page))
		}
		want := all[len(all)-2:]
		for i := range page {
			if page[i].ID != want[i].ID {
				t.Errorf("page[%d] is message %d, want %d", i, page[i].ID, want[i].ID)
			}
		}
	})

	t.Run("paging back reaches every message exactly once, in order", func(t *testing.T) {
		var (
			seen   []model.Message
			cursor model.Cursor
		)
		for range len(all) + 2 { // a bound, so a broken cursor cannot loop forever
			page, next, err := r.Page(ctx, chat, cursor, 2)
			if err != nil {
				t.Fatalf("Page() failed: %v", err)
			}
			if len(page) == 0 {
				break
			}
			seen = append(page, seen...)
			if next.IsZero() {
				break
			}
			cursor = next
		}

		if len(seen) != len(all) {
			t.Fatalf("paging saw %d messages, want %d", len(seen), len(all))
		}
		for i := range seen {
			if seen[i].ID != all[i].ID {
				t.Errorf("message %d of the paged conversation is %d, want %d", i, seen[i].ID, all[i].ID)
			}
		}
	})

	t.Run("a page carries the details, not only the text", func(t *testing.T) {
		// Paging must enrich exactly as streaming does, or a viewer would show
		// blank lines where the streamed export shows recovered content.
		page, _, err := r.Page(ctx, chat, model.Cursor{}, len(all))
		if err != nil {
			t.Fatalf("Page() failed: %v", err)
		}

		var streamed, paged int
		for _, m := range all {
			if m.HasRecoveredContent() {
				streamed++
			}
		}
		for _, m := range page {
			if m.HasRecoveredContent() {
				paged++
			}
		}
		if streamed == 0 {
			t.Skip("the fixture has nothing recovered beyond text to compare")
		}
		if paged != streamed {
			t.Errorf("a page recovered content for %d messages, streaming for %d", paged, streamed)
		}
	})

	t.Run("the cursor is empty once the beginning is reached", func(t *testing.T) {
		_, next, err := r.Page(ctx, chat, model.Cursor{}, len(all)+10)
		if err != nil {
			t.Fatalf("Page() failed: %v", err)
		}
		if !next.IsZero() {
			t.Error("a page holding the whole conversation still offers another one")
		}
	})

	t.Run("a limit of nothing still returns something", func(t *testing.T) {
		page, _, err := r.Page(ctx, chat, model.Cursor{}, 0)
		if err != nil {
			t.Fatalf("Page() failed: %v", err)
		}
		if len(page) == 0 {
			t.Error("Page() with no limit returned nothing")
		}
	})
}
