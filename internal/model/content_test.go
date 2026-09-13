package model

import (
	"strings"
	"testing"
	"time"
)

func TestPlace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		place     Place
		wantLabel string
		wantCoord bool
	}{
		{
			name:      "a named place",
			place:     Place{Latitude: 41.3851, Longitude: 2.1734, Name: "Sagrada Familia", Address: "Carrer de Mallorca"},
			wantLabel: "Sagrada Familia",
			wantCoord: true,
		},
		{
			name:      "an address with no name",
			place:     Place{Latitude: 41.3851, Longitude: 2.1734, Address: "Carrer de Mallorca"},
			wantLabel: "Carrer de Mallorca",
			wantCoord: true,
		},
		{
			name:      "a dropped pin falls back to its position",
			place:     Place{Latitude: 41.3851, Longitude: 2.1734},
			wantLabel: "41.3851, 2.1734",
			wantCoord: true,
		},
		{
			name:      "a position of exactly zero is treated as absent",
			place:     Place{Name: "Somewhere"},
			wantLabel: "Somewhere",
			wantCoord: false,
		},
		{
			name:      "trailing zeroes are trimmed from a position",
			place:     Place{Latitude: 41.5, Longitude: -2},
			wantLabel: "41.5, -2",
			wantCoord: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.place.Label(); got != tt.wantLabel {
				t.Errorf("Label() = %q, want %q", got, tt.wantLabel)
			}
			if got := tt.place.HasCoordinates(); got != tt.wantCoord {
				t.Errorf("HasCoordinates() = %v, want %v", got, tt.wantCoord)
			}
		})
	}
}

func TestPollTotalVotes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		poll Poll
		want int
	}{
		{name: "no answers", poll: Poll{}, want: 0},
		{
			name: "several answers",
			poll: Poll{Options: []PollOption{{Name: "Pizza", Votes: 3}, {Name: "Sushi", Votes: 1}}},
			want: 4,
		},
		{
			name: "answers nobody chose",
			poll: Poll{Options: []PollOption{{Name: "Pizza"}, {Name: "Sushi"}}},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.poll.TotalVotes(); got != tt.want {
				t.Errorf("TotalVotes() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCallAnswered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call Call
		want bool
	}{
		{name: "recorded as connected", call: Call{Outcome: CallConnected}, want: true},
		{
			// The outcome codes shift between WhatsApp versions, so a call that
			// lasted was answered whatever the code says.
			name: "a call that lasted, whatever its code",
			call: Call{Outcome: CallOutcomeUnknown, Duration: 30 * time.Second},
			want: true,
		},
		{name: "missed", call: Call{Outcome: CallMissed}, want: false},
		{name: "declined", call: Call{Outcome: CallDeclined}, want: false},
		{name: "nothing recorded", call: Call{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.call.Answered(); got != tt.want {
				t.Errorf("Answered() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("outcomes are named for diagnostics", func(t *testing.T) {
		t.Parallel()
		names := map[CallOutcome]string{
			CallConnected: "connected", CallMissed: "missed", CallDeclined: "declined",
			CallFailed: "failed", CallOutcomeUnknown: "unknown", CallOutcome(99): "unknown",
		}
		for outcome, want := range names {
			if got := outcome.String(); got != want {
				t.Errorf("CallOutcome(%d).String() = %q, want %q", outcome, got, want)
			}
		}
	})
}

func TestLinkPreviewIsEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		preview LinkPreview
		want    bool
	}{
		{name: "nothing at all", preview: LinkPreview{}, want: true},
		{name: "just an address", preview: LinkPreview{URL: "https://example.org"}, want: false},
		{name: "a title without an address", preview: LinkPreview{Title: "An article"}, want: false},
		{name: "only a description", preview: LinkPreview{Description: "about something"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.preview.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeletionAndThumbnail(t *testing.T) {
	t.Parallel()

	t.Run("a deletion says whether an administrator did it", func(t *testing.T) {
		t.Parallel()
		byAdmin := Deletion{By: ParseJID("34600111222@s.whatsapp.net")}
		if !byAdmin.ByAdmin() {
			t.Error("ByAdmin() = false when an administrator removed the message")
		}
		if (Deletion{}).ByAdmin() {
			t.Error("ByAdmin() = true when the sender removed their own message")
		}
	})

	t.Run("a preview reports whether an image survived", func(t *testing.T) {
		t.Parallel()
		if !(Thumbnail{}).IsEmpty() {
			t.Error("IsEmpty() = false for no preview")
		}
		if (Thumbnail{Data: []byte{0xFF, 0xD8}}).IsEmpty() {
			t.Error("IsEmpty() = true despite a preview")
		}
	})
}

func TestNoticeHasDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		notice Notice
		want   bool
	}{
		{name: "only an action code", notice: Notice{Action: 18}, want: false},
		{name: "somebody acted", notice: Notice{Actor: ParseJID("1@s.whatsapp.net")}, want: true},
		{name: "somebody was affected", notice: Notice{Targets: []JID{ParseJID("1@s.whatsapp.net")}}, want: true},
		{name: "a value changed", notice: Notice{Old: "before", New: "after"}, want: true},
		{name: "a group was named", notice: Notice{Subject: "Weekend plans"}, want: true},
		{name: "a business was named", notice: Notice{Business: "A shop"}, want: true},
		{name: "devices changed", notice: Notice{DevicesAdded: 1}, want: true},
		{name: "devices were removed", notice: Notice{DevicesRemoved: 2}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.notice.HasDetail(); got != tt.want {
				t.Errorf("HasDetail() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAttachmentAndMessageRecovery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		message      Message
		wantPreview  bool
		wantRecovery bool
		wantShown    bool
	}{
		{
			name:      "plain words",
			message:   Message{Kind: KindText, Text: "hello"},
			wantShown: true,
		},
		{
			name:         "a photo whose file is gone but whose preview survives",
			message:      Message{Kind: KindImage, Attachment: &Attachment{Preview: Thumbnail{Data: []byte{1}}}},
			wantPreview:  true,
			wantRecovery: true,
			wantShown:    true,
		},
		{
			name:         "a shared place",
			message:      Message{Kind: KindLocation, Place: &Place{Name: "Home"}},
			wantRecovery: true,
			wantShown:    true,
		},
		{
			name:         "a poll",
			message:      Message{Kind: KindPoll, Poll: &Poll{Question: "?"}},
			wantRecovery: true,
			wantShown:    true,
		},
		{
			name:         "a call",
			message:      Message{Kind: KindCall, Call: &Call{}},
			wantRecovery: true,
			wantShown:    true,
		},
		{
			name:         "a shared link",
			message:      Message{Kind: KindText, Link: &LinkPreview{URL: "https://example.org"}},
			wantRecovery: true,
			wantShown:    true,
		},
		{
			name:         "a shared contact",
			message:      Message{Kind: KindContact, Contacts: []ContactCard{{Name: "Dana"}}},
			wantRecovery: true,
			wantShown:    true,
		},
		{
			name:         "an invitation",
			message:      Message{Kind: KindInvite, Invite: &GroupInvite{GroupName: "Book club"}},
			wantRecovery: true,
			wantShown:    true,
		},
		{
			name:      "a row the app itself hides",
			message:   Message{Kind: KindIgnored},
			wantShown: false,
		},
		{
			// Hiding this would make the loss invisible; reporting it invites the gap
			// to be closed.
			name:      "a kind this build does not recognise is still shown",
			message:   Message{Kind: KindUnknown, SourceType: 250},
			wantShown: true,
		},
		{
			name:      "an album container with nothing of its own",
			message:   Message{Kind: KindAlbum},
			wantShown: false,
		},
		{
			name:      "an album container that carried a caption",
			message:   Message{Kind: KindAlbum, Text: "our holiday"},
			wantShown: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := tt.message
			if m.Attachment != nil && m.Attachment.HasPreview() != tt.wantPreview {
				t.Errorf("HasPreview() = %v, want %v", m.Attachment.HasPreview(), tt.wantPreview)
			}
			if got := m.HasRecoveredContent(); got != tt.wantRecovery {
				t.Errorf("HasRecoveredContent() = %v, want %v", got, tt.wantRecovery)
			}
			if got := m.Displayable(); got != tt.wantShown {
				t.Errorf("Displayable() = %v, want %v", got, tt.wantShown)
			}
		})
	}
}

// TestNewKindNamesAreStable pins the names for the kinds added when the reader
// learned to recover more. They appear in reports people paste into support
// conversations, so they must not drift.
func TestNewKindNamesAreStable(t *testing.T) {
	t.Parallel()

	tests := map[Kind]string{
		KindAlbum: "album", KindInvite: "invite",
		KindInteractive: "interactive", KindIgnored: "ignored",
	}
	for kind, want := range tests {
		if got := kind.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", kind, got, want)
		}
		if kind.IsNotice() {
			t.Errorf("Kind(%s).IsNotice() = true, want false", want)
		}
		if kind.HasAttachment() {
			t.Errorf("Kind(%s).HasAttachment() = true, want false", want)
		}
	}
}

// TestContactCardKeepsWhatWasSent guards the decision to store a shared contact
// exactly as it arrived rather than parsing it into fields, because any parsing
// loses something the sender actually sent.
func TestContactCardKeepsWhatWasSent(t *testing.T) {
	t.Parallel()

	raw := "BEGIN:VCARD\nVERSION:3.0\nFN:Dana Smith\nTEL;TYPE=CELL:+34600999888\nEND:VCARD"
	card := ContactCard{Name: "Dana Smith", VCard: raw}

	if card.VCard != raw {
		t.Error("the card was altered in storage")
	}
	if !strings.Contains(card.VCard, "TEL") {
		t.Error("the telephone number was lost")
	}
}

// TestSearchTextGathersEveryWord is the contract the index depends on. A message
// that carried a photograph, a poll or a shared place is findable by the words
// that survived with it, and testing this here is what stops a new kind of content
// being added without becoming searchable.
func TestSearchTextGathersEveryWord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message Message
		want    []string
		absent  []string
	}{
		{
			name:    "what somebody typed",
			message: Message{Kind: KindText, Text: "hola mundo"},
			want:    []string{"hola mundo"},
		},
		{
			name: "the name of a file whose picture is gone",
			message: Message{
				Kind:       KindImage,
				Attachment: &Attachment{FileName: "IMG-20190614-WA0007.jpg", Caption: "at the beach"},
			},
			want: []string{"IMG-20190614-WA0007.jpg", "at the beach"},
		},
		{
			name: "a poll's question and its answers",
			message: Message{
				Kind: KindPoll,
				Poll: &Poll{
					Question: "Where shall we eat",
					Options:  []PollOption{{Name: "Casa Blanca"}, {Name: "Sushi"}},
				},
			},
			want: []string{"Where shall we eat", "Casa Blanca", "Sushi"},
		},
		{
			name: "what a page said when it was shared",
			message: Message{
				Kind: KindText,
				Link: &LinkPreview{URL: "https://example.org/x", Title: "El Vermut", Description: "A bar"},
			},
			want: []string{"El Vermut", "A bar", "https://example.org/x"},
		},
		{
			name: "where a place was",
			message: Message{
				Kind:  KindLocation,
				Place: &Place{Name: "Parc Güell", Address: "Carrer Olot 5"},
			},
			want: []string{"Parc Güell", "Carrer Olot 5"},
		},
		{
			name:    "whose card was shared",
			message: Message{Kind: KindContact, Contacts: []ContactCard{{Name: "Marta Ruiz"}}},
			want:    []string{"Marta Ruiz"},
		},
		{
			name: "the message a reply answered",
			message: Message{
				Kind:  KindText,
				Text:  "yes",
				Quote: &Quote{Text: "shall we go on Sunday", Attachment: &Attachment{FileName: "plan.pdf"}},
			},
			want: []string{"yes", "shall we go on Sunday", "plan.pdf"},
		},
		{
			name:    "an invitation to a group",
			message: Message{Kind: KindInvite, Invite: &GroupInvite{GroupName: "Vermut del sábado"}},
			want:    []string{"Vermut del sábado"},
		},
		{
			name: "what a notice recorded",
			message: Message{
				Kind:       KindSystem,
				SystemText: "Ana changed the subject",
				Notice:     &Notice{Old: "Comida", New: "Cena", Subject: "Familia", Business: "Bar Pepe"},
			},
			want: []string{"Ana changed the subject", "Comida", "Cena", "Familia", "Bar Pepe"},
		},
		{
			name:    "the name the sender had at the time",
			message: Message{Kind: KindText, Text: "hola", PushName: "Anita"},
			want:    []string{"hola", "Anita"},
		},
		{
			name:    "a call has no words of its own",
			message: Message{Kind: KindCall, Call: &Call{}},
			want:    nil,
		},
		{
			name:    "an empty message yields nothing to index",
			message: Message{Kind: KindText},
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.message.SearchText()
			if len(tt.want) == 0 {
				if got != "" {
					t.Errorf("SearchText() = %q, want nothing to index", got)
				}
				return
			}
			for _, word := range tt.want {
				if !strings.Contains(got, word) {
					t.Errorf("SearchText() = %q, missing %q", got, word)
				}
			}
			for _, word := range tt.absent {
				if strings.Contains(got, word) {
					t.Errorf("SearchText() = %q, should not contain %q", got, word)
				}
			}
		})
	}

	t.Run("the words are separated so two of them cannot run together", func(t *testing.T) {
		t.Parallel()

		m := Message{Kind: KindText, Text: "hola", PushName: "Anita"}
		if got := m.SearchText(); got != "hola\nAnita" {
			t.Errorf("SearchText() = %q, want the words on separate lines", got)
		}
	})
}

// TestMessageStatePredicates covers the questions a message answers about itself,
// which callers ask instead of comparing fields.
func TestMessageStatePredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message Message
		deleted bool
		expires bool
		spread  bool
	}{
		{name: "an ordinary message", message: Message{Kind: KindText, Text: "hola"}},
		{name: "deleted by its kind", message: Message{Kind: KindDeleted}, deleted: true},
		{name: "deleted by record", message: Message{Kind: KindText, Deleted: &Deletion{}}, deleted: true},
		{name: "disappearing", message: Message{Kind: KindText, Expires: time.Hour}, expires: true},
		{name: "forwarded a few times", message: Message{Kind: KindText, ForwardScore: 3}},
		{name: "forwarded many times", message: Message{Kind: KindText, ForwardScore: 5}, spread: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.message.WasDeleted(); got != tt.deleted {
				t.Errorf("WasDeleted() = %v, want %v", got, tt.deleted)
			}
			if got := tt.message.IsDisappearing(); got != tt.expires {
				t.Errorf("IsDisappearing() = %v, want %v", got, tt.expires)
			}
			if got := tt.message.WasForwardedMany(); got != tt.spread {
				t.Errorf("WasForwardedMany() = %v, want %v", got, tt.spread)
			}
		})
	}
}
