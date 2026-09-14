package export

import (
	"strings"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

// testRenderer builds a renderer with names for the two people the tests use.
func testRenderer(t *testing.T) renderer {
	t.Helper()
	return newRenderer(testOptions(t).Names, testOptions(t).withDefaults())
}

// TestBodyNeverReturnsNothing is the promise that matters most in an export: a
// blank line in somebody's history looks like data loss, so every message says at
// least what kind of thing it was.
func TestBodyNeverReturnsNothing(t *testing.T) {
	t.Parallel()

	r := testRenderer(t)
	kinds := []model.Kind{
		model.KindUnknown, model.KindText, model.KindImage, model.KindVideo,
		model.KindAudio, model.KindVoice, model.KindDocument, model.KindSticker,
		model.KindGIF, model.KindContact, model.KindLocation, model.KindPoll,
		model.KindEvent, model.KindDeleted, model.KindSystem, model.KindCall,
		model.KindViewOnce, model.KindPayment, model.KindAlbum, model.KindInvite,
		model.KindInteractive, model.KindIgnored,
	}

	for _, kind := range kinds {
		t.Run(kind.String(), func(t *testing.T) {
			t.Parallel()

			// Deliberately bare: no text, no attachment, nothing recovered.
			body := r.body(model.Message{Kind: kind, SentAt: sentAt})
			if strings.TrimSpace(body) == "" {
				t.Errorf("a %s message rendered as nothing at all", kind)
			}
		})
	}
}

func TestRenderDetails(t *testing.T) {
	t.Parallel()

	r := testRenderer(t)

	tests := []struct {
		name    string
		message model.Message
		want    []string
	}{
		{
			name:    "a message with nothing attached has no detail lines",
			message: model.Message{Kind: model.KindText, Text: "plain"},
		},
		{
			name: "a reply to a photograph names the file",
			message: model.Message{
				Kind: model.KindText, Text: "nice",
				Quote: &model.Quote{
					Sender: bob, Kind: model.KindImage,
					Attachment: &model.Attachment{FileName: "IMG.jpg"},
				},
			},
			want: []string{"> Bob: <IMG.jpg>"},
		},
		{
			name: "a reply to a photograph with no name says what kind it was",
			message: model.Message{
				Kind: model.KindText, Text: "nice",
				Quote: &model.Quote{Sender: bob, Kind: model.KindImage},
			},
			want: []string{"> Bob: <image>"},
		},
		{
			name: "a reply to something the owner sent names them",
			message: model.Message{
				Kind: model.KindText, Text: "yes",
				Quote: &model.Quote{FromMe: true, Kind: model.KindText, Text: "shall we?"},
			},
			want: []string{"> You: shall we?"},
		},
		{
			name: "a starred message says so",
			message: model.Message{
				Kind: model.KindText, Text: "important", Starred: true,
			},
			want: []string{"starred"},
		},
		{
			name: "a forwarded message that did not travel far",
			message: model.Message{
				Kind: model.KindText, Text: "passed on", Forwarded: true, ForwardScore: 1,
			},
			want: []string{"forwarded"},
		},
		{
			name: "a very long quotation is shortened",
			message: model.Message{
				Kind: model.KindText, Text: "short reply",
				Quote: &model.Quote{Sender: bob, Text: strings.Repeat("word ", 100)},
			},
			want: []string{"…"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			details := strings.Join(r.details(tt.message), "\n")
			if len(tt.want) == 0 && details != "" {
				t.Errorf("expected no detail lines, got %q", details)
			}
			for _, want := range tt.want {
				if !strings.Contains(details, want) {
					t.Errorf("details %q do not contain %q", details, want)
				}
			}
		})
	}
}

func TestRenderEdgeCases(t *testing.T) {
	t.Parallel()

	r := testRenderer(t)

	t.Run("a message with no recorded time still says so", func(t *testing.T) {
		t.Parallel()
		if got := r.timestamp(time.Time{}); got != "[unknown date]" {
			t.Errorf("timestamp() = %q", got)
		}
	})

	t.Run("a sender nobody can name is not left blank", func(t *testing.T) {
		t.Parallel()
		if got := r.sender(model.Message{Kind: model.KindText}); got != "Unknown" {
			t.Errorf("sender() = %q", got)
		}
	})

	t.Run("the owner deleting their own message reads differently", func(t *testing.T) {
		t.Parallel()

		mine := model.Message{Kind: model.KindDeleted}.NewOutgoing()
		if got := r.body(mine); got != "You deleted this message" {
			t.Errorf("body() = %q", got)
		}
		theirs := model.Message{Kind: model.KindDeleted, Sender: alice}
		if got := r.body(theirs); got != "This message was deleted" {
			t.Errorf("body() = %q", got)
		}
	})

	t.Run("an invitation with no group name still describes itself", func(t *testing.T) {
		t.Parallel()

		m := model.Message{Kind: model.KindInvite, Invite: &model.GroupInvite{}}
		if got := r.body(m); got != "<invitation to join a group>" {
			t.Errorf("body() = %q", got)
		}
	})

	t.Run("an album says how many things it held", func(t *testing.T) {
		t.Parallel()

		m := model.Message{Kind: model.KindAlbum, AlbumSize: 4}
		if got := r.body(m); !strings.Contains(got, "4 items") {
			t.Errorf("body() = %q", got)
		}
		withCaption := model.Message{Kind: model.KindAlbum, AlbumSize: 4, Text: "our holiday"}
		if got := r.body(withCaption); !strings.Contains(got, "our holiday") {
			t.Errorf("body() = %q", got)
		}
	})

	t.Run("contact cards with no names are still counted", func(t *testing.T) {
		t.Parallel()

		m := model.Message{Kind: model.KindContact, Contacts: []model.ContactCard{{}, {}}}
		if got := r.body(m); !strings.Contains(got, "2 contact cards") {
			t.Errorf("body() = %q", got)
		}
	})

	t.Run("a declined call is told apart from a missed one", func(t *testing.T) {
		t.Parallel()

		declined := model.Message{Kind: model.KindCall, Call: &model.Call{Outcome: model.CallDeclined}}
		if got := r.body(declined); !strings.Contains(got, "declined") {
			t.Errorf("body() = %q", got)
		}
		unknown := model.Message{Kind: model.KindCall, Call: &model.Call{Outcome: model.CallOutcomeUnknown}}
		if got := r.body(unknown); !strings.Contains(got, "not answered") {
			t.Errorf("body() = %q", got)
		}
		group := model.Message{Kind: model.KindCall, Call: &model.Call{Group: true, Outcome: model.CallConnected}}
		if got := r.body(group); !strings.Contains(got, "group voice call") {
			t.Errorf("body() = %q", got)
		}
	})

	t.Run("a live location is told apart from a pinned one", func(t *testing.T) {
		t.Parallel()

		m := model.Message{Kind: model.KindLocation, Place: &model.Place{Live: true, Name: "Home"}}
		if got := r.body(m); !strings.Contains(got, "live location") {
			t.Errorf("body() = %q", got)
		}
	})

	t.Run("a system notice with no wording still says something", func(t *testing.T) {
		t.Parallel()

		m := model.Message{Kind: model.KindSystem, Notice: &model.Notice{Action: 99}}
		if got := r.body(m); got != "System notice" {
			t.Errorf("body() = %q", got)
		}
	})
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		d    time.Duration
		want string
	}{
		{d: 0, want: "0s"},
		{d: -time.Second, want: "0s"},
		{d: 45 * time.Second, want: "45s"},
		{d: 125 * time.Second, want: "2m 5s"},
		{d: 2*time.Hour + 3*time.Minute + 4*time.Second, want: "2h 3m 4s"},
		{d: 7 * 24 * time.Hour, want: "168h 0m 0s"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := formatDuration(tt.d); got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

// TestRenderingWithoutNames checks that an export still works before any address
// book has been loaded, which is what happens when somebody exports immediately.
func TestRenderingWithoutNames(t *testing.T) {
	t.Parallel()

	r := newRenderer(nil, Options{}.withDefaults())

	m := model.Message{Kind: model.KindText, Text: "hello", Sender: alice}
	if got := r.sender(m); got != alice.String() {
		t.Errorf("sender() = %q, want the raw address", got)
	}
	if got := r.name(bob, false); got != bob.String() {
		t.Errorf("name() = %q, want the raw address", got)
	}
	if got := r.name(model.JID{}, true); got != "You" {
		t.Errorf("name() for the owner = %q", got)
	}
}

// TestLineIsTheOneWording covers the entry point three packages now share. A
// photograph described one way in an export and another way on a phone is the sort
// of difference that makes somebody doubt both.
func TestLineIsTheOneWording(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give model.Message
		want string
	}{
		{name: "plain words", give: model.Message{Kind: model.KindText, Text: "hello there"}, want: "hello there"},
		{
			name: "a file that is no longer here",
			give: model.Message{Kind: model.KindImage, Attachment: &model.Attachment{}},
			want: "<image omitted>",
		},
		{
			name: "a file with something written beside it",
			give: model.Message{Kind: model.KindImage, Text: "look at this", Attachment: &model.Attachment{}},
			want: "<image omitted> look at this",
		},
		{name: "a message taken back", give: model.Message{Kind: model.KindDeleted}, want: "This message was deleted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Line(tt.give, Options{}); got != tt.want {
				t.Errorf("Line() = %q, want %q", got, tt.want)
			}
		})
	}
}
