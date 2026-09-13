package export

import (
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

// TestHTMLPageIsSelfContained is the promise the format exists for: the file works
// with the network switched off, from a USB stick, in ten years. A single external
// reference breaks that and nothing on screen would say so.
func TestHTMLPageIsSelfContained(t *testing.T) {
	t.Parallel()

	opts := testOptions(t)
	message := incoming("a message")
	message.Attachment = &model.Attachment{
		MediaType: "image/jpeg",
		Preview:   model.Thumbnail{Data: []byte{0xff, 0xd8, 0xff, 0xe0, 1, 2, 3}},
	}
	message.Kind = model.KindImage

	result, err := WriteHTML(conversationOf(theChat, message), opts)
	if err != nil {
		t.Fatalf("WriteHTML() failed: %v", err)
	}
	page := readFile(t, result.Files[0])

	forbidden := []struct {
		name    string
		pattern string
	}{
		{name: "a stylesheet fetched from elsewhere", pattern: `<link rel="stylesheet"`},
		{name: "any linked resource at all", pattern: "<link "},
		{name: "a script fetched from elsewhere", pattern: "<script src="},
		{name: "an image fetched over the network", pattern: `src="http`},
		{name: "a web font", pattern: "@font-face"},
		{name: "a stylesheet import", pattern: "@import"},
		{name: "a frame", pattern: "<iframe"},
	}
	for _, f := range forbidden {
		t.Run(f.name, func(t *testing.T) {
			if strings.Contains(page, f.pattern) {
				t.Errorf("the page contains %q, so it would reach the network", f.pattern)
			}
		})
	}

	required := []struct {
		name    string
		pattern string
	}{
		{name: "the stylesheet is inside the file", pattern: "<style>"},
		{name: "the script is inside the file", pattern: "<script>"},
		{name: "the picture is inside the file", pattern: "src=\"data:image/jpeg;base64,"},
		{name: "it says the file itself is gone", pattern: "the file itself is not in this archive"},
		{name: "it disclaims affiliation", pattern: "Not affiliated with"},
	}
	for _, r := range required {
		t.Run(r.name, func(t *testing.T) {
			if !strings.Contains(page, r.pattern) {
				t.Errorf("the page is missing %q", r.pattern)
			}
		})
	}
}

// TestHTMLEscapesWhatPeopleSent is the security property. A message is text
// somebody else wrote, and this file is opened in a browser.
func TestHTMLEscapesWhatPeopleSent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		text   string
		absent []string
		want   string
	}{
		{
			name:   "a script tag",
			text:   `<script>alert(1)</script>`,
			absent: []string{"<script>alert"},
			want:   "&lt;script&gt;alert(1)&lt;/script&gt;",
		},
		{
			name:   "an image with a handler",
			text:   `<img src=x onerror="alert(1)">`,
			absent: []string{"<img src=x"},
			want:   "&lt;img src=x onerror=",
		},
		{
			name:   "an attempt to close the bubble",
			text:   `</div></article><script>x</script>`,
			absent: []string{"</article><script>"},
			want:   "&lt;/div&gt;&lt;/article&gt;",
		},
		{
			name:   "an ampersand, which is not an attack",
			text:   "tea & biscuits",
			absent: nil,
			want:   "tea &amp; biscuits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := testOptions(t)
			result, err := WriteHTML(conversationOf(theChat, incoming(tt.text)), opts)
			if err != nil {
				t.Fatalf("WriteHTML() failed: %v", err)
			}
			page := readFile(t, result.Files[0])

			if !strings.Contains(page, tt.want) {
				t.Errorf("the page does not contain the escaped text %q", tt.want)
			}
			for _, gone := range tt.absent {
				if strings.Contains(page, gone) {
					t.Errorf("the page contains %q, which the browser would act on", gone)
				}
			}
		})
	}
}

// TestLinkify checks that addresses people shared become clickable without any
// other markup surviving, and that nothing but http and https ever becomes a link.
func TestLinkify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		in     string
		want   string
		absent string
	}{
		{
			name: "a plain address",
			in:   "see https://example.org now",
			want: `see <a href="https://example.org" rel="noreferrer noopener">https://example.org</a> now`,
		},
		{
			name: "a sentence's full stop is not part of the address",
			in:   "go to https://example.org.",
			want: `<a href="https://example.org" rel="noreferrer noopener">https://example.org</a>.`,
		},
		{
			name: "an address in brackets",
			in:   "(https://example.org)",
			want: `(<a href="https://example.org" rel="noreferrer noopener">https://example.org</a>)`,
		},
		{
			name:   "a script address is left as text",
			in:     "javascript:alert(1)",
			want:   "javascript:alert(1)",
			absent: "<a href",
		},
		{
			name:   "an address that tries to close its own attribute",
			in:     `https://example.org/"onmouseover="alert(1)`,
			absent: `"onmouseover="alert`,
			want:   "&#34;onmouseover=",
		},
		{
			name: "text around an address is still escaped",
			in:   "<b>https://example.org</b>",
			want: "&lt;b&gt;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := string(linkify(tt.in))
			if !strings.Contains(got, tt.want) {
				t.Errorf("linkify(%q) = %q, want it to contain %q", tt.in, got, tt.want)
			}
			if tt.absent != "" && strings.Contains(got, tt.absent) {
				t.Errorf("linkify(%q) = %q, want it not to contain %q", tt.in, got, tt.absent)
			}
		})
	}
}

// TestPreviewsKeepTheirFormat guards against an archive showing a broken picture
// where a recovered one should be. WhatsApp writes JPEG almost always, and almost
// is not always.
func TestPreviewsKeepTheirFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "jpeg", data: []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0, 0, 0}, want: "image/jpeg"},
		{name: "png", data: []byte{0x89, 'P', 'N', 'G', 13, 10, 26, 10}, want: "image/png"},
		{name: "gif", data: []byte("GIF89a....."), want: "image/gif"},
		{name: "webp", data: []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), want: "image/webp"},
		{name: "something unrecognised is called jpeg", data: []byte{1, 2, 3, 4}, want: "image/jpeg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uri := string(dataURI(model.Thumbnail{Data: tt.data}))
			if !strings.HasPrefix(uri, "data:"+tt.want+";base64,") {
				t.Errorf("dataURI() = %.40q, want it to start with data:%s;base64,", uri, tt.want)
			}
		})
	}

	t.Run("nothing recovered produces nothing", func(t *testing.T) {
		t.Parallel()
		if got := dataURI(model.Thumbnail{}); got != "" {
			t.Errorf("dataURI() on an empty preview = %q, want empty", got)
		}
	})
}

// TestHTMLShowsRecoveredContent is the reason the format exists: everything the
// reader pulled out of the database has somewhere to appear on the page.
func TestHTMLShowsRecoveredContent(t *testing.T) {
	t.Parallel()

	full := incoming("the caption")
	full.Kind = model.KindImage
	full.Attachment = &model.Attachment{
		FileName: "beach.jpg",
		Duration: 0,
		Preview:  model.Thumbnail{Data: []byte{0xff, 0xd8, 0xff, 0xe0, 9}},
	}
	full.Quote = &model.Quote{Sender: bob, Kind: model.KindText, Text: "what did you do"}
	full.Reactions = []model.Reaction{{Sender: bob, Emoji: "👍"}}
	full.Link = &model.LinkPreview{URL: "https://example.org", Title: "A page", Description: "About it"}
	full.Forwarded = true
	full.Starred = true
	full.EditedAt = sentAt

	poll := incoming("")
	poll.ID = 2
	poll.Kind = model.KindPoll
	poll.Poll = &model.Poll{
		Question: "Where shall we eat",
		Options:  []model.PollOption{{Name: "Pizza", Votes: 3}, {Name: "Sushi", Votes: 1}},
	}

	call := incoming("")
	call.ID = 3
	call.Kind = model.KindCall
	call.Call = &model.Call{Video: true, Duration: 90 * time.Second, Outcome: model.CallConnected}

	opts := testOptions(t)
	result, err := WriteHTML(conversationOf(theChat, full, poll, call), opts)
	if err != nil {
		t.Fatalf("WriteHTML() failed: %v", err)
	}
	page := readFile(t, result.Files[0])

	wants := []struct {
		name    string
		pattern string
	}{
		{name: "the recovered picture", pattern: `class="preview"`},
		{name: "the caption beside it", pattern: "the caption"},
		{name: "the file that is not here", pattern: "beach.jpg"},
		{name: "what the reply answered", pattern: "what did you do"},
		{name: "who was replied to", pattern: "Bob"},
		{name: "the reaction", pattern: "👍 Bob"},
		{name: "the link's title", pattern: "A page"},
		{name: "the link's address", pattern: "https://example.org"},
		{name: "that it was forwarded", pattern: "forwarded"},
		{name: "that it was starred", pattern: "starred"},
		{name: "that it was edited", pattern: "edited"},
		{name: "the poll's question", pattern: "Where shall we eat"},
		{name: "a poll answer and its votes", pattern: "3 votes"},
		{name: "a single vote in the singular", pattern: "1 vote<"},
		{name: "the call", pattern: "video call"},
		{name: "the day it happened", pattern: "Saturday, 12 September 2026"},
	}
	for _, w := range wants {
		t.Run(w.name, func(t *testing.T) {
			if !strings.Contains(page, w.pattern) {
				t.Errorf("the page is missing %q", w.pattern)
			}
		})
	}

	if result.Messages != 3 {
		t.Errorf("result.Messages = %d, want 3", result.Messages)
	}
}

// TestHTMLSeparatesDays checks that the date appears once between days rather than
// on every message, and that a run of messages on one day gets one separator.
func TestHTMLSeparatesDays(t *testing.T) {
	t.Parallel()

	first := incoming("morning")
	second := incoming("still morning")
	second.ID = 2
	second.SentAt = sentAt.Add(2 * time.Hour)
	third := incoming("next day")
	third.ID = 3
	third.SentAt = sentAt.Add(26 * time.Hour)

	opts := testOptions(t)
	result, err := WriteHTML(conversationOf(theChat, first, second, third), opts)
	if err != nil {
		t.Fatalf("WriteHTML() failed: %v", err)
	}
	page := readFile(t, result.Files[0])

	if got := strings.Count(page, `<div class="day">`); got != 2 {
		t.Errorf("the page has %d day separators, want 2 for three messages over two days", got)
	}
}

// TestHTMLNamesItsFileTheSameWayTheIndexDoes guards the link between the two: an
// index that points at a name the writer did not use is an archive of dead links.
func TestHTMLNamesItsFileTheSameWayTheIndexDoes(t *testing.T) {
	t.Parallel()

	chats := []model.Chat{
		theChat,
		{ID: 2, JID: model.ParseJID("120363@g.us"), Kind: model.ChatGroup, Name: "Familia / Casa"},
		{ID: 3, JID: bob, Kind: model.ChatDirect},
	}

	for _, chat := range chats {
		t.Run(chat.Title(), func(t *testing.T) {
			t.Parallel()

			opts := testOptions(t)
			result, err := WriteHTML(conversationOf(chat, incoming("hello")), opts)
			if err != nil {
				t.Fatalf("WriteHTML() failed: %v", err)
			}
			if got, want := filepath.Base(result.Files[0]), FileName(chat, ".html"); got != want {
				t.Errorf("the page was written as %q, but the index would link to %q", got, want)
			}
		})
	}
}

// hrefPattern pulls the links out of the index so a test can follow them.
var hrefPattern = regexp.MustCompile(`href="([^"]+)"`)

func TestWriteIndex(t *testing.T) {
	t.Parallel()

	older := model.Chat{
		ID: 1, JID: alice, Kind: model.ChatDirect, Name: "Ana Lopez",
		Messages: 10, LastAt: sentAt.Add(-48 * time.Hour),
	}
	newer := model.Chat{
		ID: 2, JID: model.ParseJID("120363@g.us"), Kind: model.ChatGroup,
		Name: "Familia & <friends>", Messages: 4, LastAt: sentAt,
		Participants: []model.Participant{{JID: alice}, {JID: bob}},
	}

	opts := testOptions(t)
	entries := make([]Entry, 0, 2)
	for _, chat := range []model.Chat{older, newer} {
		if _, err := WriteHTML(conversationOf(chat, incoming("hello")), opts); err != nil {
			t.Fatalf("writing a conversation page failed: %v", err)
		}
		entries = append(entries, Entry{
			Chat: chat, File: FileName(chat, ".html"), Messages: chat.Messages,
		})
	}

	result, err := WriteIndex(entries, opts)
	if err != nil {
		t.Fatalf("WriteIndex() failed: %v", err)
	}

	if got := filepath.Base(result.Files[0]); got != IndexName {
		t.Errorf("the index was written as %q, want %q", got, IndexName)
	}
	page := readFile(t, result.Files[0])

	t.Run("the most recent conversation is first", func(t *testing.T) {
		familia := strings.Index(page, "Familia")
		ana := strings.Index(page, "Ana Lopez")
		if familia == -1 || ana == -1 {
			t.Fatalf("a conversation is missing from the index")
		}
		if familia > ana {
			t.Error("the conversation with the most recent message is not listed first")
		}
	})

	t.Run("a name with markup in it is escaped", func(t *testing.T) {
		if strings.Contains(page, "<friends>") {
			t.Error("a group name was written into the page as markup")
		}
		if !strings.Contains(page, "Familia &amp; &lt;friends&gt;") {
			t.Error("the escaped group name is missing")
		}
	})

	t.Run("every link leads to a file that is there", func(t *testing.T) {
		// The names are escaped for a URL by the time they reach the page, and a
		// conversation called "Ana Lopez" is a file called "Ana Lopez (…).html".
		// Comparing the strings would only prove the escaping matches itself, so
		// this follows each link the way a browser would.
		hrefs := hrefPattern.FindAllStringSubmatch(page, -1)
		if len(hrefs) != 2 {
			t.Fatalf("the index has %d links, want one per conversation", len(hrefs))
		}
		for _, href := range hrefs {
			target, err := url.PathUnescape(html.UnescapeString(href[1]))
			if err != nil {
				t.Fatalf("the index wrote a link that is not a usable address: %q", href[1])
			}
			if _, err := os.Stat(filepath.Join(opts.Directory, target)); err != nil {
				t.Errorf("the index links to %q, which is not in the archive", target)
			}
		}
	})

	t.Run("it says what the archive holds", func(t *testing.T) {
		for _, want := range []string{"2 conversations", "14 messages", "group", "2 members"} {
			if !strings.Contains(page, want) {
				t.Errorf("the index does not mention %q", want)
			}
		}
	})

	t.Run("it is self-contained too", func(t *testing.T) {
		for _, forbidden := range []string{"<link ", "<script src=", `src="http`} {
			if strings.Contains(page, forbidden) {
				t.Errorf("the index contains %q, so it would reach the network", forbidden)
			}
		}
	})
}

// TestIndexOfNothing checks the empty case, which happens when every conversation
// was filtered out and would otherwise write a page claiming to be an archive.
func TestIndexOfNothing(t *testing.T) {
	t.Parallel()

	opts := testOptions(t)
	result, err := WriteIndex(nil, opts)
	if err != nil {
		t.Fatalf("WriteIndex() on nothing failed: %v", err)
	}
	page := readFile(t, result.Files[0])
	if !strings.Contains(page, "0 conversations") {
		t.Error("an empty archive does not say that it is empty")
	}
}

// TestHTMLRefusesToOverwrite repeats for this format the rule the others follow:
// an export never destroys one that is already there.
func TestHTMLRefusesToOverwrite(t *testing.T) {
	t.Parallel()

	opts := testOptions(t)
	conv := conversationOf(theChat, incoming("hello"))

	if _, err := WriteHTML(conv, opts); err != nil {
		t.Fatalf("the first export failed: %v", err)
	}
	if _, err := WriteHTML(conv, opts); err == nil {
		t.Fatal("the second export overwrote the first")
	}

	opts.Overwrite = true
	if _, err := WriteHTML(conv, opts); err != nil {
		t.Errorf("--force did not permit replacing the file: %v", err)
	}

	entries, err := os.ReadDir(opts.Directory)
	if err != nil {
		t.Fatalf("reading the export directory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("the directory holds %d files, want 1 with no leftovers", len(entries))
	}
}

// TestHTMLLeavesNoticesOutByDefault mirrors the rule the other formats follow, so
// the three formats cannot disagree about what an archive contains.
func TestHTMLLeavesNoticesOutByDefault(t *testing.T) {
	t.Parallel()

	notice := incoming("")
	notice.ID = 2
	notice.Kind = model.KindSystem
	notice.SystemText = "Your security code with Ana Lopez changed"

	t.Run("left out", func(t *testing.T) {
		t.Parallel()

		opts := testOptions(t)
		result, err := WriteHTML(conversationOf(theChat, incoming("hello"), notice), opts)
		if err != nil {
			t.Fatalf("WriteHTML() failed: %v", err)
		}
		if strings.Contains(readFile(t, result.Files[0]), "security code") {
			t.Error("a notice appeared in an export that did not ask for them")
		}
		if result.Skipped != 1 {
			t.Errorf("result.Skipped = %d, want 1", result.Skipped)
		}
	})

	t.Run("kept when asked for", func(t *testing.T) {
		t.Parallel()

		opts := testOptions(t)
		opts.IncludeNotices = true
		result, err := WriteHTML(conversationOf(theChat, incoming("hello"), notice), opts)
		if err != nil {
			t.Fatalf("WriteHTML() failed: %v", err)
		}
		page := readFile(t, result.Files[0])
		if !strings.Contains(page, "security code") {
			t.Error("a notice was left out of an export that asked for them")
		}
		if !strings.Contains(page, "msg notice") {
			t.Error("a notice is not marked as one, so it would look like somebody said it")
		}
	})
}
