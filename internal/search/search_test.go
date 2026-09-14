package search

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

var (
	ana   = model.ParseJID("34600111222@s.whatsapp.net")
	luis  = model.ParseJID("34600333444@s.whatsapp.net")
	group = model.ParseJID("120363001@g.us")
	base  = time.Date(2019, 6, 14, 9, 0, 0, 0, time.UTC)
)

// archive is a source built from messages held in memory, so the tests describe
// what is indexed rather than how a database stores it.
type archive struct {
	chats    []model.Chat
	messages map[int64][]model.Message
}

func (a archive) Chats(context.Context) ([]model.Chat, error) { return a.chats, nil }

func (a archive) Messages(_ context.Context, chat model.Chat) iter.Seq2[model.Message, error] {
	return func(yield func(model.Message, error) bool) {
		for _, m := range a.messages[chat.ID] {
			if !yield(m, nil) {
				return
			}
		}
	}
}

// said builds a received text message.
func said(id int64, minutes int, text string) model.Message {
	return model.Message{
		ID: id, Kind: model.KindText, Sender: ana,
		SentAt: base.Add(time.Duration(minutes) * time.Minute), Text: text,
	}
}

// buildIndex indexes the given messages in one direct conversation.
func buildIndex(t *testing.T, messages ...model.Message) *Index {
	t.Helper()

	chat := model.Chat{ID: 1, JID: ana, Kind: model.ChatDirect, Name: "Ana Lopez", Messages: len(messages)}
	return buildArchive(t, archive{
		chats:    []model.Chat{chat},
		messages: map[int64][]model.Message{1: messages},
	}, Options{})
}

func buildArchive(t *testing.T, src archive, opts Options) *Index {
	t.Helper()

	names := model.NewDirectory()
	names.Add(model.Contact{JID: ana, Name: "Ana Lopez"})
	names.Add(model.Contact{JID: luis, Name: "Luis"})
	opts.Names = names

	path := filepath.Join(t.TempDir(), "index.db")
	index, err := Build(context.Background(), src, path, "msgstore.db", opts)
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	t.Cleanup(func() { _ = index.Close() })
	return index
}

// found returns the messages a search matched, as their snippets.
func found(t *testing.T, index *Index, term string) []string {
	t.Helper()

	hits, err := index.Search(context.Background(), term, Query{})
	if err != nil {
		t.Fatalf("Search(%q) failed: %v", term, err)
	}
	out := make([]string, len(hits))
	for i, hit := range hits {
		out[i] = hit.Snippet
	}
	return out
}

// TestSearchFoldsAccents is the difference between a search that works in Spanish
// and one that silently returns nothing.
func TestSearchFoldsAccents(t *testing.T) {
	t.Parallel()

	index := buildIndex(t,
		said(1, 0, "José viene mañana"),
		said(2, 1, "cuántos años tiene"),
		said(3, 2, "nothing to see"),
	)

	tests := []struct {
		name string
		term string
		want int
	}{
		{name: "plain finds accented", term: "jose", want: 1},
		{name: "accented finds accented", term: "José", want: 1},
		{name: "plain finds tilde", term: "manana", want: 1},
		{name: "accented finds plain", term: "añoS", want: 1},
		{name: "case does not matter", term: "JOSE", want: 1},
		{name: "a word nobody said", term: "tortuga", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(found(t, index, tt.term)); got != tt.want {
				t.Errorf("searching for %q found %d messages, want %d", tt.term, got, tt.want)
			}
		})
	}
}

// TestSearchNeverFailsOnWhatSomebodyTyped is the promise behind the search box:
// it is a box, not a query language, so nothing typed into it is a syntax error.
func TestSearchNeverFailsOnWhatSomebodyTyped(t *testing.T) {
	t.Parallel()

	index := buildIndex(t,
		said(1, 0, "it's a (good) morning * AND NOT OR"),
		said(2, 1, "second message"),
	)

	terms := []string{
		"it's", "(good)", "*", "AND", "NOT", "OR", "a AND", "NOT b",
		`"unclosed phrase`, `""`, "-", "- -", "((((", "a:b", "^start", "100%",
		"👍", "a  b", "  spaced  ", "morning*", `"good morning"`,
	}

	for _, term := range terms {
		t.Run(term, func(t *testing.T) {
			_, err := index.Search(context.Background(), term, Query{})
			if err != nil && !errors.Is(err, ErrEmptyQuery) {
				t.Errorf("searching for %q failed: %v", term, err)
			}
		})
	}
}

// TestExpression checks the query builder directly, because most of its work is
// invisible in results.
func TestExpression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		in    string
		want  string
		fails bool
	}{
		{name: "one word", in: "hola", want: `"hola"`},
		{name: "two words are both required", in: "hola mundo", want: `"hola" AND "mundo"`},
		{name: "a phrase stays whole", in: `"buenos dias"`, want: `"buenos dias"`},
		{name: "a trailing star searches the start of a word", in: "maña*", want: `"maña"*`},
		{name: "a leading minus excludes", in: "hola -adios", want: `"hola" NOT "adios"`},
		{name: "a stray quote splits a word rather than vanishing", in: `it"s`, want: `"it" AND "s"`},
		{name: "the syntax words are searched for, not obeyed", in: "a AND b", want: `"a" AND "AND" AND "b"`},
		{name: "punctuation alone is dropped", in: "hola ... !!", want: `"hola"`},
		{name: "an unclosed phrase is closed at the end", in: `"buenos dias`, want: `"buenos dias"`},
		{name: "nothing at all", in: "", fails: true},
		{name: "only punctuation", in: "!!! ???", fails: true},
		{name: "only an exclusion", in: "-hola", fails: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := expression(tt.in)
			if tt.fails {
				if !errors.Is(err, ErrEmptyQuery) {
					t.Fatalf("expression(%q) = %q, %v; want it refused", tt.in, got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expression(%q) failed: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("expression(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestSearchFindsWhatWasRecovered is the reason the index holds more than the text
// column: a photograph, a poll or a shared place is findable by the words that
// survived with it.
func TestSearchFindsWhatWasRecovered(t *testing.T) {
	t.Parallel()

	photo := said(1, 0, "")
	photo.Kind = model.KindImage
	photo.Attachment = &model.Attachment{FileName: "IMG-20190614-WA0007.jpg"}

	poll := said(2, 1, "")
	poll.Kind = model.KindPoll
	poll.Poll = &model.Poll{
		Question: "Where shall we eat",
		Options:  []model.PollOption{{Name: "Casa Blanca"}, {Name: "Sushi"}},
	}

	link := said(3, 2, "have a look")
	link.Link = &model.LinkPreview{
		URL: "https://example.org/vermut", Title: "El Vermut", Description: "A bar in Gracia",
	}

	place := said(4, 3, "")
	place.Kind = model.KindLocation
	place.Place = &model.Place{Name: "Parc Güell", Address: "Carrer Olot 5"}

	card := said(5, 4, "")
	card.Kind = model.KindContact
	card.Contacts = []model.ContactCard{{Name: "Marta Ruiz"}}

	quote := said(6, 5, "yes")
	quote.Quote = &model.Quote{Sender: luis, Text: "shall we go on Sunday"}

	index := buildIndex(t, photo, poll, link, place, card, quote)

	tests := []struct {
		name string
		term string
	}{
		{name: "the name of a file whose picture is gone", term: "WA0007"},
		{name: "a poll's question", term: "shall we eat"},
		{name: "a poll's answer", term: "Casa Blanca"},
		{name: "the title a page had at the time", term: "vermut"},
		{name: "the description of a link", term: "Gracia"},
		{name: "the name of a place", term: "Güell"},
		{name: "the address of a place", term: "Olot"},
		{name: "whose contact card was shared", term: "Marta"},
		{name: "the message a reply answered", term: "Sunday"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(found(t, index, tt.term)); got == 0 {
				t.Errorf("searching for %q found nothing, so that recovery is invisible", tt.term)
			}
		})
	}
}

// TestSnippetMarksTheMatch checks the part people actually read.
func TestSnippetMarksTheMatch(t *testing.T) {
	t.Parallel()

	index := buildIndex(t, said(1, 0, "we should go to the beach on Sunday"))

	hits, err := index.Search(context.Background(), "beach", Query{Before: "[", After: "]"})
	if err != nil {
		t.Fatalf("Search() failed: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("Search() found %d messages, want 1", len(hits))
	}
	if !strings.Contains(hits[0].Snippet, "[beach]") {
		t.Errorf("the snippet does not mark the match: %q", hits[0].Snippet)
	}

	// The marks a program uses rather than a person reads. The null character looks
	// like the natural choice for this and is silently dropped by SQLite, which
	// loses the opening of every match and leaves the closing in place, so the pair
	// that is actually used is checked here.
	t.Run("the marks meant for a program survive", func(t *testing.T) {
		marked, err := index.Search(context.Background(), "beach",
			Query{Before: MarkOpen, After: MarkClose})
		if err != nil {
			t.Fatalf("Search() failed: %v", err)
		}
		if len(marked) != 1 {
			t.Fatalf("Search() found %d messages, want 1", len(marked))
		}
		if want := MarkOpen + "beach" + MarkClose; !strings.Contains(marked[0].Snippet, want) {
			t.Errorf("the snippet is %q, want it to contain %q", marked[0].Snippet, want)
		}
	})
	if hits[0].Chat != "Ana Lopez" {
		t.Errorf("the hit names the conversation %q, want %q", hits[0].Chat, "Ana Lopez")
	}
	if hits[0].Sender != "Ana Lopez" {
		t.Errorf("the hit names the sender %q, want %q", hits[0].Sender, "Ana Lopez")
	}
	if !hits[0].SentAt.Equal(base) {
		t.Errorf("the hit is dated %v, want %v", hits[0].SentAt, base)
	}
}

// TestSearchNarrows covers the filters, which is how a search over a million
// messages becomes an answer.
func TestSearchNarrows(t *testing.T) {
	t.Parallel()

	direct := model.Chat{ID: 1, JID: ana, Kind: model.ChatDirect, Name: "Ana Lopez", Messages: 2}
	chatGroup := model.Chat{ID: 2, JID: group, Kind: model.ChatGroup, Name: "Vermut", Messages: 1}

	index := buildArchive(t, archive{
		chats: []model.Chat{direct, chatGroup},
		messages: map[int64][]model.Message{
			1: {said(1, 0, "vermut on Saturday"), said(2, 60*24*30, "vermut again")},
			2: {said(3, 10, "vermut in the group")},
		},
	}, Options{})

	tests := []struct {
		name  string
		query Query
		want  int
	}{
		{name: "everything", query: Query{}, want: 3},
		{name: "one conversation", query: Query{Chat: ana.String()}, want: 2},
		{name: "the group only", query: Query{Chat: group.String()}, want: 1},
		{name: "after a date", query: Query{Since: base.Add(24 * time.Hour)}, want: 1},
		{name: "before a date", query: Query{Until: base.Add(24 * time.Hour)}, want: 2},
		{name: "a window that holds nothing", query: Query{
			Since: base.Add(48 * time.Hour), Until: base.Add(72 * time.Hour),
		}, want: 0},
		{name: "just the first result", query: Query{Limit: 1}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits, err := index.Search(context.Background(), "vermut", tt.query)
			if err != nil {
				t.Fatalf("Search() failed: %v", err)
			}
			if len(hits) != tt.want {
				t.Errorf("Search() found %d messages, want %d", len(hits), tt.want)
			}
		})
	}

	t.Run("the count ignores the limit", func(t *testing.T) {
		n, err := index.Count(context.Background(), "vermut", Query{Limit: 1})
		if err != nil {
			t.Fatalf("Count() failed: %v", err)
		}
		if n != 3 {
			t.Errorf("Count() = %d, want 3", n)
		}
	})
}

// TestNoticesAreLeftOutByDefault keeps a search for a name from returning two
// hundred security-code notices.
func TestNoticesAreLeftOutByDefault(t *testing.T) {
	t.Parallel()

	notice := said(1, 0, "")
	notice.Kind = model.KindSystem
	notice.SystemText = "Your security code with Ana Lopez changed"

	t.Run("left out", func(t *testing.T) {
		t.Parallel()
		if got := len(found(t, buildIndex(t, notice, said(2, 1, "hello")), "security")); got != 0 {
			t.Errorf("a notice was indexed without being asked for")
		}
	})

	t.Run("kept when asked for", func(t *testing.T) {
		t.Parallel()

		chat := model.Chat{ID: 1, JID: ana, Kind: model.ChatDirect, Name: "Ana Lopez", Messages: 2}
		index := buildArchive(t, archive{
			chats:    []model.Chat{chat},
			messages: map[int64][]model.Message{1: {notice, said(2, 1, "hello")}},
		}, Options{IncludeNotices: true})

		if got := len(found(t, index, "security")); got != 1 {
			t.Errorf("a notice was left out of an index that asked for them")
		}
	})
}

// TestBuildLeavesNothingBehindWhenItFails checks that an interrupted build does
// not replace a good index with a half-written one.
func TestBuildLeavesNothingBehindWhenItFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "index.db")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	chat := model.Chat{ID: 1, JID: ana, Kind: model.ChatDirect, Name: "Ana Lopez", Messages: 1}
	src := archive{chats: []model.Chat{chat}, messages: map[int64][]model.Message{1: {said(1, 0, "hello")}}}

	if _, err := Build(ctx, src, path, "msgstore.db", Options{}); err == nil {
		t.Fatal("Build() succeeded after the context was cancelled")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the directory: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("a failed build left %v behind", names)
	}
}

// TestOpenRefusesWhatIsNotAnIndex guards against a confusing failure much later.
func TestOpenRefusesWhatIsNotAnIndex(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "not-an-index.db")
	if err := os.WriteFile(path, []byte("this is not a database"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	_, err := Open(context.Background(), path)
	if err == nil {
		t.Fatal("Open() accepted a file that is not an index")
	}
	if !errors.Is(err, ErrUnreadable) {
		t.Errorf("Open() error = %v, want an unreadable-index error", err)
	}
}

func TestStats(t *testing.T) {
	t.Parallel()

	index := buildIndex(t, said(1, 0, "hello"), said(2, 1, "there"))

	stats, err := index.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() failed: %v", err)
	}
	if stats.Messages != 2 {
		t.Errorf("stats.Messages = %d, want 2", stats.Messages)
	}
	if stats.Conversations != 1 {
		t.Errorf("stats.Conversations = %d, want 1", stats.Conversations)
	}
	if stats.Source != "msgstore.db" {
		t.Errorf("stats.Source = %q, want the archive it was built from", stats.Source)
	}
	if stats.BuiltAt.IsZero() {
		t.Error("stats.BuiltAt is unset, so nobody can tell how old the index is")
	}
	if stats.Notices {
		t.Error("stats.Notices says notices were indexed, and they were not")
	}
}

// TestIndexIsPrivate: an index is a copy of every word in somebody's history, so
// it is no more readable than the archive it came from.
func TestIndexIsPrivate(t *testing.T) {
	t.Parallel()

	// Windows has no Unix permission bits: a file created 0600 is reported 0666,
	// because os.Chmod there only toggles the read-only flag. The rule still matters
	// there and is simply not expressible this way — keeping an index to its owner on
	// Windows means an ACL, which this program does not yet set. Said out loud rather
	// than asserted loosely, so the gap is a known one.
	if runtime.GOOS == "windows" {
		t.Skip("permission bits mean nothing on Windows")
	}

	index := buildIndex(t, said(1, 0, "hello"))

	info, err := os.Stat(index.Path())
	if err != nil {
		t.Fatalf("looking at the index: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("the index is mode %o, want 600", perm)
	}
}

// TestStaleIndexIsNoticed guards the quiet failure: an index that has fallen
// behind still answers, and the answer silently misses everything said since.
func TestStaleIndexIsNoticed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	archivePath := filepath.Join(dir, "msgstore.db")
	if err := os.WriteFile(archivePath, []byte("pretend this is an archive"), 0o600); err != nil {
		t.Fatalf("writing the fixture archive: %v", err)
	}

	names := model.NewDirectory()
	chat := model.Chat{ID: 1, JID: ana, Kind: model.ChatDirect, Name: "Ana Lopez", Messages: 1}
	src := archive{chats: []model.Chat{chat}, messages: map[int64][]model.Message{1: {said(1, 0, "hello")}}}

	index, err := Build(context.Background(), src, filepath.Join(dir, "index.db"), archivePath,
		Options{Names: names})
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	defer func() { _ = index.Close() }()

	stats, err := index.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() failed: %v", err)
	}

	t.Run("an untouched archive still matches", func(t *testing.T) {
		if !stats.MatchesSource(archivePath) {
			t.Error("the index reports itself stale against the archive it was built from")
		}
	})

	t.Run("an archive that grew no longer matches", func(t *testing.T) {
		if err := os.WriteFile(archivePath, []byte("pretend this is a longer archive"), 0o600); err != nil {
			t.Fatalf("changing the fixture archive: %v", err)
		}
		if stats.MatchesSource(archivePath) {
			t.Error("the index did not notice that the archive changed")
		}
	})

	t.Run("an archive that is gone does not match", func(t *testing.T) {
		if err := os.Remove(archivePath); err != nil {
			t.Fatalf("removing the fixture archive: %v", err)
		}
		if stats.MatchesSource(archivePath) {
			t.Error("the index matched an archive that is not there")
		}
	})
}

// TestSearchReadsTheIndexWithoutWritingToIt: a search must never be able to damage
// the thing it is reading, and an index opened for writing can be corrupted by a
// crash mid-query.
func TestSearchReadsTheIndexWithoutWritingToIt(t *testing.T) {
	t.Parallel()

	index := buildIndex(t, said(1, 0, "hello"))

	before, err := os.Stat(index.Path())
	if err != nil {
		t.Fatalf("looking at the index: %v", err)
	}
	if _, err := index.Search(context.Background(), "hello", Query{}); err != nil {
		t.Fatalf("Search() failed: %v", err)
	}
	after, err := os.Stat(index.Path())
	if err != nil {
		t.Fatalf("looking at the index: %v", err)
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		t.Error("searching changed the index file")
	}
}

// TestErrorsCarryTheirGuidance: a failure is matched by what it means rather than
// by its wording, so wrapping one never loses the advice attached to it.
func TestErrorsCarryTheirGuidance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		sentinel *Error
	}{
		{name: "unreadable", sentinel: ErrUnreadable},
		{name: "no full text", sentinel: ErrNoFullText},
		{name: "empty query", sentinel: ErrEmptyQuery},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cause := errors.New("the underlying reason")
			wrapped := fmt.Errorf("while searching: %w", tt.sentinel.withCause(cause))

			if !errors.Is(wrapped, tt.sentinel) {
				t.Error("a wrapped failure no longer matches the sentinel it came from")
			}
			if !errors.Is(wrapped, cause) {
				t.Error("the underlying reason was lost")
			}
			if !strings.Contains(wrapped.Error(), "the underlying reason") {
				t.Errorf("the message does not say why: %q", wrapped.Error())
			}
			if tt.sentinel.Error() == "" {
				t.Error("the sentinel has no message of its own")
			}
		})
	}

	t.Run("a different failure does not match", func(t *testing.T) {
		t.Parallel()
		if errors.Is(ErrEmptyQuery, ErrNoFullText) {
			t.Error("two unrelated failures compare equal")
		}
		if errors.Is(errors.New("something else"), ErrUnreadable) {
			t.Error("an unrelated error matched a sentinel")
		}
	})
}
