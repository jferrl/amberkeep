package export

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

// A fixed instant and zone, so every assertion about a timestamp is exact.
var (
	madrid  = time.FixedZone("CET", 2*60*60)
	sentAt  = time.Date(2026, 9, 12, 18, 46, 9, 0, time.UTC)
	alice   = model.ParseJID("34600111222@s.whatsapp.net")
	bob     = model.ParseJID("34600333444@s.whatsapp.net")
	theChat = model.Chat{
		ID: 1, JID: alice, Kind: model.ChatDirect, Name: "Ana Lopez", Messages: 1,
	}
)

// testOptions returns options writing into a fresh directory.
func testOptions(t *testing.T) Options {
	t.Helper()

	names := model.NewDirectory()
	names.Add(model.Contact{JID: alice, Name: "Ana Lopez"})
	names.Add(model.Contact{JID: bob, Name: "Bob"})

	return Options{
		Directory: t.TempDir(),
		Names:     names,
		Location:  madrid,
		Me:        "You",
	}
}

// conversationOf wraps messages as a conversation the writers can pull through.
func conversationOf(chat model.Chat, messages ...model.Message) Conversation {
	chat.Messages = len(messages)
	return Conversation{
		Chat: chat,
		Messages: func(yield func(model.Message, error) bool) {
			for _, m := range messages {
				if !yield(m, nil) {
					return
				}
			}
		},
	}
}

// incoming builds a received message.
func incoming(text string) model.Message {
	return model.Message{ID: 1, Kind: model.KindText, SentAt: sentAt, Sender: alice, Text: text}
}

func TestWriteTextRendersEveryKindOfContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message model.Message
		want    []string // lines that must appear
		absent  []string
	}{
		{
			name:    "plain words",
			message: incoming("hello there"),
			want:    []string{"[12/09/2026, 20:46:09] Ana Lopez: hello there"},
		},
		{
			name:    "a message the archive's owner sent",
			message: model.Message{Kind: model.KindText, SentAt: sentAt, Text: "mine"}.NewOutgoing(),
			want:    []string{"[12/09/2026, 20:46:09] You: mine"},
		},
		{
			name: "a photograph whose file is gone but whose preview survived",
			message: func() model.Message {
				m := incoming("")
				m.Kind = model.KindImage
				m.Text = "at the beach"
				m.Attachment = &model.Attachment{
					FileName: "IMG-0001.jpg",
					Preview:  model.Thumbnail{Data: []byte{0xFF, 0xD8, 0xFF}},
				}
				return m
			}(),
			want: []string{"<image omitted: IMG-0001.jpg>", "[preview recovered]", "at the beach"},
		},
		{
			name: "a voice message says how long it was",
			message: func() model.Message {
				m := incoming("")
				m.Kind = model.KindVoice
				m.Attachment = &model.Attachment{Duration: 125 * time.Second}
				return m
			}(),
			want: []string{"<voice message omitted>", "(2m 5s)"},
		},
		{
			name: "a shared place names it and gives its position",
			message: func() model.Message {
				m := incoming("")
				m.Kind = model.KindLocation
				m.Place = &model.Place{
					Latitude: 41.3851, Longitude: 2.1734,
					Name: "Sagrada Familia", Address: "Carrer de Mallorca",
				}
				return m
			}(),
			want: []string{"<location: Sagrada Familia (41.3851, 2.1734)>", "address: Carrer de Mallorca"},
		},
		{
			name: "a poll states the question and lists the answers",
			message: func() model.Message {
				m := incoming("Where shall we eat?")
				m.Kind = model.KindPoll
				m.Poll = &model.Poll{
					Question: "Where shall we eat?",
					Options: []model.PollOption{
						{Name: "Pizza", Votes: 3}, {Name: "Sushi", Votes: 1},
					},
				}
				return m
			}(),
			want: []string{"<poll: Where shall we eat?>", "- Pizza (3 votes)", "- Sushi (1 vote)"},
		},
		{
			name: "an answered video call says how long it lasted",
			message: func() model.Message {
				m := incoming("")
				m.Kind = model.KindCall
				m.Call = &model.Call{Video: true, Duration: 125 * time.Second, Outcome: model.CallConnected}
				return m
			}(),
			want: []string{"<video call, 2m 5s>"},
		},
		{
			name: "a missed call says so",
			message: func() model.Message {
				m := incoming("")
				m.Kind = model.KindCall
				m.Call = &model.Call{Outcome: model.CallMissed}
				return m
			}(),
			want: []string{"<missed voice call>"},
		},
		{
			name: "a shared link keeps what the page said at the time",
			message: func() model.Message {
				m := incoming("look at this")
				m.Link = &model.LinkPreview{
					URL: "https://example.org/a", Title: "An article", Description: "About something",
				}
				return m
			}(),
			want: []string{"look at this", "link: An article — About something — https://example.org/a"},
		},
		{
			name: "a shared contact names them",
			message: func() model.Message {
				m := incoming("")
				m.Kind = model.KindContact
				m.Contacts = []model.ContactCard{{Name: "Dana Smith", VCard: "BEGIN:VCARD"}}
				return m
			}(),
			want: []string{"<contact card: Dana Smith>"},
		},
		{
			name: "a reply shows what it answered",
			message: func() model.Message {
				m := incoming("yes, agreed")
				m.Quote = &model.Quote{Sender: bob, Kind: model.KindText, Text: "shall we?"}
				return m
			}(),
			want: []string{"yes, agreed", "> Bob: shall we?"},
		},
		{
			name: "reactions are grouped by emoji",
			message: func() model.Message {
				m := incoming("funny")
				m.Reactions = []model.Reaction{
					{Sender: bob, Emoji: "😀"},
					{FromMe: true, Emoji: "😀"},
					{Sender: alice, Emoji: "❤️"},
				}
				return m
			}(),
			want: []string{"reactions: 😀 Bob, You; ❤️ Ana Lopez"},
		},
		{
			name: "an edited message is marked",
			message: func() model.Message {
				m := incoming("corrected")
				m.EditedAt = sentAt.Add(time.Minute)
				return m
			}(),
			want: []string{"corrected <This message was edited>"},
		},
		{
			name: "a deleted message says who removed it",
			message: func() model.Message {
				m := incoming("")
				m.Kind = model.KindDeleted
				m.Deleted = &model.Deletion{At: sentAt, By: bob}
				return m
			}(),
			want: []string{"This message was deleted by Bob"},
		},
		{
			name: "a widely forwarded message says so",
			message: func() model.Message {
				m := incoming("passed along")
				m.Forwarded = true
				m.ForwardScore = 7
				return m
			}(),
			want: []string{"forwarded many times"},
		},
		{
			name: "a disappearing message says how long it lasted",
			message: func() model.Message {
				m := incoming("temporary")
				m.Expires = 7 * 24 * time.Hour
				return m
			}(),
			want: []string{"disappears after 168h 0m 0s"},
		},
		{
			name:    "a multi-line message keeps its shape without extra timestamps",
			message: incoming("first line\nsecond line"),
			want:    []string{"[12/09/2026, 20:46:09] Ana Lopez: first line", "    second line"},
		},
		{
			name: "an unrecognised kind reports its number rather than vanishing",
			message: func() model.Message {
				m := incoming("")
				m.Kind = model.KindUnknown
				m.SourceType = 250
				return m
			}(),
			want: []string{"<unrecognised message, type 250>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := testOptions(t)
			result, err := WriteText(conversationOf(theChat, tt.message), opts)
			if err != nil {
				t.Fatalf("WriteText() failed: %v", err)
			}
			if result.Messages != 1 {
				t.Fatalf("wrote %d messages, want 1", result.Messages)
			}

			body := readFile(t, result.Files[0])
			for _, want := range tt.want {
				if !strings.Contains(body, want) {
					t.Errorf("the export does not contain %q\n---\n%s", want, body)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(body, absent) {
					t.Errorf("the export unexpectedly contains %q", absent)
				}
			}
		})
	}
}

// TestTextExportIsParseable guards the property that makes plain text worth
// choosing: a line beginning with a timestamp starts a message, and nothing else
// does, so the file can be read back by the tools people already have.
func TestTextExportIsParseable(t *testing.T) {
	t.Parallel()

	messages := []model.Message{
		incoming("first\nsecond\nthird"),
		func() model.Message {
			m := incoming("with a reply")
			m.ID = 2
			m.Quote = &model.Quote{Sender: bob, Text: "the question"}
			m.Reactions = []model.Reaction{{Sender: bob, Emoji: "👍"}}
			return m
		}(),
	}

	opts := testOptions(t)
	result, err := WriteText(conversationOf(theChat, messages...), opts)
	if err != nil {
		t.Fatalf("WriteText() failed: %v", err)
	}

	body := readFile(t, result.Files[0])
	_, conversation, found := strings.Cut(body, strings.Repeat("-", 60)+"\n\n")
	if !found {
		t.Fatal("the header separator is missing")
	}

	var starts int
	for line := range strings.SplitSeq(strings.TrimRight(conversation, "\n"), "\n") {
		if strings.HasPrefix(line, "[") {
			starts++
			continue
		}
		if !strings.HasPrefix(line, textIndent) {
			t.Errorf("a line neither starts a message nor is indented: %q", line)
		}
	}
	if starts != len(messages) {
		t.Errorf("found %d message starts, want %d", starts, len(messages))
	}
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	message := incoming("hello")
	message.Attachment = &model.Attachment{
		FileName: "IMG.jpg", Preview: model.Thumbnail{Data: []byte{0xFF, 0xD8, 0xFF}},
	}
	message.Kind = model.KindImage
	message.Reactions = []model.Reaction{{Sender: bob, Emoji: "👍"}}
	message.Poll = &model.Poll{Question: "Q", Options: []model.PollOption{{Name: "A", Votes: 2}}}

	opts := testOptions(t)
	opts.NoticeIdentified = func(int) bool { return true }
	result, err := WriteJSON(conversationOf(theChat, message), opts)
	if err != nil {
		t.Fatalf("WriteJSON() failed: %v", err)
	}

	var file struct {
		Version  int      `json:"version"`
		Exported string   `json:"exported"`
		Chat     chatJSON `json:"chat"`
		Messages []struct {
			messageJSON
			Attachment struct {
				FileName string `json:"file_name"`
				Preview  string `json:"preview_base64"`
			} `json:"attachment"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(readFile(t, result.Files[0])), &file); err != nil {
		t.Fatalf("the export is not valid structured data: %v", err)
	}

	if file.Version != jsonVersion {
		t.Errorf("version = %d, want %d", file.Version, jsonVersion)
	}
	if file.Chat.Name != "Ana Lopez" {
		t.Errorf("chat name = %q", file.Chat.Name)
	}
	if len(file.Messages) != 1 {
		t.Fatalf("wrote %d messages, want 1", len(file.Messages))
	}

	m := file.Messages[0]
	t.Run("the sender is named as well as addressed", func(t *testing.T) {
		if m.SenderName != "Ana Lopez" || m.Sender != alice.String() {
			t.Errorf("sender = %q / %q", m.Sender, m.SenderName)
		}
	})
	t.Run("a readable rendering is included", func(t *testing.T) {
		if m.Rendered == "" {
			t.Error("the rendered form is missing")
		}
	})
	t.Run("the surviving picture travels with the archive", func(t *testing.T) {
		if m.Attachment.Preview == "" {
			t.Error("the recovered preview was not written")
		}
		if m.Attachment.FileName != "IMG.jpg" {
			t.Errorf("file name = %q", m.Attachment.FileName)
		}
	})
	t.Run("reactions and polls survive the round trip", func(t *testing.T) {
		if len(m.Reactions) != 1 || m.Reactions[0].Emoji != "👍" {
			t.Errorf("reactions = %+v", m.Reactions)
		}
		if m.Poll == nil || len(m.Poll.Options) != 1 || m.Poll.Options[0].Votes != 2 {
			t.Errorf("poll = %+v", m.Poll)
		}
	})
	t.Run("times are written in universal time", func(t *testing.T) {
		if !m.SentAt.Equal(sentAt) {
			t.Errorf("sent at %v, want %v", m.SentAt, sentAt)
		}
	})
}

func TestNoticesAreLeftOutByDefault(t *testing.T) {
	t.Parallel()

	notice := model.Message{
		ID: 1, Kind: model.KindSystem, SentAt: sentAt,
		SystemText: "Ana Lopez added you",
		Notice:     &model.Notice{Action: 12},
	}
	ordinary := incoming("a real message")
	ordinary.ID = 2

	t.Run("left out unless asked for", func(t *testing.T) {
		t.Parallel()

		opts := testOptions(t)
		result, err := WriteText(conversationOf(theChat, notice, ordinary), opts)
		if err != nil {
			t.Fatalf("WriteText() failed: %v", err)
		}
		if result.Messages != 1 || result.Skipped != 1 {
			t.Errorf("wrote %d and skipped %d, want 1 and 1", result.Messages, result.Skipped)
		}
		if strings.Contains(readFile(t, result.Files[0]), "added you") {
			t.Error("a notice was written without being asked for")
		}
	})

	t.Run("included on request, and without a sender", func(t *testing.T) {
		t.Parallel()

		opts := testOptions(t)
		opts.IncludeNotices = true
		result, err := WriteText(conversationOf(theChat, notice, ordinary), opts)
		if err != nil {
			t.Fatalf("WriteText() failed: %v", err)
		}
		if result.Messages != 2 {
			t.Errorf("wrote %d messages, want 2", result.Messages)
		}
		body := readFile(t, result.Files[0])
		if !strings.Contains(body, "[12/09/2026, 20:46:09] Ana Lopez added you") {
			t.Errorf("the notice was not written as expected\n---\n%s", body)
		}
	})
}

func TestWritingIsSafe(t *testing.T) {
	t.Parallel()

	t.Run("an existing file is not destroyed", func(t *testing.T) {
		t.Parallel()

		opts := testOptions(t)
		conv := conversationOf(theChat, incoming("first"))
		if _, err := WriteText(conv, opts); err != nil {
			t.Fatalf("the first export failed: %v", err)
		}
		_, err := WriteText(conv, opts)
		if !errors.Is(err, ErrExists) {
			t.Fatalf("the second export error = %v, want %v", err, ErrExists)
		}
	})

	t.Run("overwriting is possible when asked for", func(t *testing.T) {
		t.Parallel()

		opts := testOptions(t)
		conv := conversationOf(theChat, incoming("first"))
		if _, err := WriteText(conv, opts); err != nil {
			t.Fatalf("the first export failed: %v", err)
		}
		opts.Overwrite = true
		if _, err := WriteText(conversationOf(theChat, incoming("second")), opts); err != nil {
			t.Fatalf("the second export failed: %v", err)
		}
		body := readFile(t, filepath.Join(opts.Directory, FileName(theChat, ".txt")))
		if !strings.Contains(body, "second") {
			t.Error("the file was not replaced")
		}
	})

	t.Run("a failure part way through leaves nothing behind", func(t *testing.T) {
		t.Parallel()

		opts := testOptions(t)
		failing := Conversation{
			Chat: theChat,
			Messages: func(yield func(model.Message, error) bool) {
				yield(incoming("one"), nil)
				yield(model.Message{}, errors.New("the database went away"))
			},
		}
		if _, err := WriteText(failing, opts); err == nil {
			t.Fatal("WriteText() did not report the failure")
		}

		entries, err := os.ReadDir(opts.Directory)
		if err != nil {
			t.Fatalf("reading the export directory: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("a failed export left %d files behind: %v", len(entries), entries)
		}
	})

	t.Run("an archive is not readable by everybody", func(t *testing.T) {
		t.Parallel()

		opts := testOptions(t)
		result, err := WriteText(conversationOf(theChat, incoming("private")), opts)
		if err != nil {
			t.Fatalf("WriteText() failed: %v", err)
		}
		info, err := os.Stat(result.Files[0])
		if err != nil {
			t.Fatalf("inspecting the export: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("permissions = %04o, want 0600", perm)
		}
	})
}

func TestFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		chat model.Chat
		want string
	}{
		{
			name: "an ordinary name",
			chat: model.Chat{JID: alice, Name: "Ana Lopez"},
			want: "Ana Lopez (34600111222).txt",
		},
		{
			name: "characters no filesystem accepts",
			chat: model.Chat{JID: alice, Name: `Work: plans / "ideas"`},
			want: "Work- plans - -ideas- (34600111222).txt",
		},
		{
			name: "a name that is only punctuation falls back",
			chat: model.Chat{JID: alice, Name: "..."},
			want: "conversation (34600111222).txt",
		},
		{
			name: "a name Windows reserves",
			chat: model.Chat{JID: model.ParseJID("1@s.whatsapp.net"), Name: "CON"},
			want: "CON_ (1).txt",
		},
		{
			name: "a name that already contains the number is not repeated",
			chat: model.Chat{JID: alice, Name: "+34600111222"},
			want: "+34600111222.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := FileName(tt.chat, ".txt"); got != tt.want {
				t.Errorf("FileName() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("a very long name is shortened", func(t *testing.T) {
		t.Parallel()

		chat := model.Chat{JID: alice, Name: strings.Repeat("long ", 60)}
		got := FileName(chat, ".txt")
		if len(got) > 130 {
			t.Errorf("the name is %d bytes, which is too long for some filesystems", len(got))
		}
	})
}

// readFile returns a written export.
func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the export: %v", err)
	}
	return string(data)
}

// TestJSONCarriesEverythingRecovered checks the commitment that makes the
// structured format worth building on: nothing the reader recovered is lost on
// the way out.
func TestJSONCarriesEverythingRecovered(t *testing.T) {
	t.Parallel()

	full := incoming("the words")
	full.Key = "3EB0ABCD"
	full.Place = &model.Place{Latitude: 41.3851, Longitude: 2.1734, Name: "Home", Address: "A street"}
	full.Call = &model.Call{Video: true, Duration: 90 * time.Second, Outcome: model.CallConnected}
	full.Link = &model.LinkPreview{URL: "https://example.org", Title: "T", Description: "D"}
	full.Contacts = []model.ContactCard{{Name: "Dana", VCard: "BEGIN:VCARD", JIDs: []model.JID{bob}}}
	full.Invite = &model.GroupInvite{GroupName: "Book club", Group: bob, ExpiresAt: sentAt}
	full.Deleted = &model.Deletion{At: sentAt, By: bob}
	full.Notice = &model.Notice{Action: 12, Actor: alice, Targets: []model.JID{bob}, Old: "a", New: "b"}
	full.Quote = &model.Quote{Sender: bob, Kind: model.KindImage, Text: "q",
		Attachment: &model.Attachment{FileName: "Q.jpg", Preview: model.Thumbnail{Data: []byte{1, 2}}}}
	full.Mentions = []model.JID{bob}
	full.Expires = time.Hour
	full.Starred = true
	full.EditedAt = sentAt.Add(time.Minute)
	full.SourceType = 66

	opts := testOptions(t)
	opts.IncludeNotices = true
	opts.NoticeIdentified = func(action int) bool { return action == 12 }
	result, err := WriteJSON(conversationOf(theChat, full), opts)
	if err != nil {
		t.Fatalf("WriteJSON() failed: %v", err)
	}

	body := readFile(t, result.Files[0])
	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("the export is not valid structured data: %v", err)
	}

	// Each of these is a piece of content that would otherwise be silently lost.
	for _, want := range []string{
		`"key": "3EB0ABCD"`, `"place"`, `"Home"`, `"call"`, `"outcome": "connected"`,
		`"link"`, `"contact_cards"`, `"BEGIN:VCARD"`, `"group_invite"`, `"Book club"`,
		`"deleted"`, `"by_admin": true`, `"notice"`, `"identified": true`,
		`"reply_to"`, `"preview_base64"`, `"mentions"`, `"expires_after": "1h 0m 0s"`,
		`"starred": true`, `"edited_at"`, `"source_type": 66`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the export is missing %s", want)
		}
	}
}

func TestJSONReportsUnidentifiedNotices(t *testing.T) {
	t.Parallel()

	notice := model.Message{
		ID: 1, Kind: model.KindSystem, SentAt: sentAt,
		SystemText: "Unrecognised notice, code 165",
		Notice:     &model.Notice{Action: 165},
	}

	opts := testOptions(t)
	opts.IncludeNotices = true
	opts.NoticeIdentified = func(int) bool { return false }
	result, err := WriteJSON(conversationOf(theChat, notice), opts)
	if err != nil {
		t.Fatalf("WriteJSON() failed: %v", err)
	}

	body := readFile(t, result.Files[0])
	if !strings.Contains(body, `"identified": false`) {
		t.Error("an unidentified notice was not admitted as such")
	}
	if !strings.Contains(body, "code 165") {
		t.Error("the action code was not reported")
	}
}

func TestGroupHeaderListsMembers(t *testing.T) {
	t.Parallel()

	group := model.Chat{
		ID: 2, JID: model.ParseJID("1-2@g.us"), Kind: model.ChatGroup,
		Name:   "Weekend plans",
		LastAt: sentAt,
		Participants: []model.Participant{
			{JID: alice, Name: "Ana Lopez", Admin: true},
			{JID: bob, Name: "Bob"},
		},
	}

	opts := testOptions(t)
	result, err := WriteText(conversationOf(group, incoming("hello")), opts)
	if err != nil {
		t.Fatalf("WriteText() failed: %v", err)
	}

	body := readFile(t, result.Files[0])
	for _, want := range []string{"Weekend plans", "Group conversation", "Members: Ana Lopez, Bob", "Last message:"} {
		if !strings.Contains(body, want) {
			t.Errorf("the header is missing %q\n---\n%s", want, body)
		}
	}
}

func TestDefaultsAreFilledIn(t *testing.T) {
	t.Parallel()

	opts := Options{Directory: t.TempDir()}.withDefaults()
	if opts.Location == nil {
		t.Error("no time zone was chosen")
	}
	if opts.Me == "" {
		t.Error("the archive owner has no label")
	}
	if opts.NoticeIdentified == nil {
		t.Fatal("the notice identifier is missing")
	}
	if opts.NoticeIdentified(12) {
		t.Error("without being told, the exporter claimed to understand a notice code")
	}
}
