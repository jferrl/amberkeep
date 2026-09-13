package android

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

// readAll collects one conversation, keyed by message identifier, so an assertion
// can name the message it is about.
func readAll(t *testing.T, r *Reader, chatID int64) map[int64]model.Message {
	t.Helper()

	chats, err := r.Chats(context.Background())
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}
	for _, c := range chats {
		if c.ID != chatID {
			continue
		}
		out := make(map[int64]model.Message, c.Messages)
		for m, err := range r.Messages(context.Background(), c) {
			if err != nil {
				t.Fatalf("Messages() failed: %v", err)
			}
			out[m.ID] = m
		}
		return out
	}
	t.Fatalf("conversation %d is missing", chatID)
	return nil
}

// TestRecoversContentBeyondText is the heart of the reader's value. Each case is a
// kind of content WhatsApp stores in its own table, which an archive that stops at
// the message table turns into a blank line.
func TestRecoversContentBeyondText(t *testing.T) {
	t.Parallel()

	msgs := readAll(t, openFixture(t), chatAlice)

	tests := []struct {
		name  string
		id    int64
		check func(t *testing.T, m model.Message)
	}{
		{
			name: "a shared place keeps its name, address and position",
			id:   20,
			check: func(t *testing.T, m model.Message) {
				if m.Place == nil {
					t.Fatal("Place is missing")
				}
				if m.Place.Name != "Sagrada Familia" {
					t.Errorf("Name = %q", m.Place.Name)
				}
				if m.Place.Address != "Carrer de Mallorca" {
					t.Errorf("Address = %q", m.Place.Address)
				}
				if !m.Place.HasCoordinates() {
					t.Error("HasCoordinates() = false")
				}
				if m.Place.Live {
					t.Error("Live = true for a place shared once")
				}
				if m.Place.Label() != "Sagrada Familia" {
					t.Errorf("Label() = %q", m.Place.Label())
				}
			},
		},
		{
			name: "a poll keeps its question, answers and vote counts",
			id:   21,
			check: func(t *testing.T, m model.Message) {
				if m.Poll == nil {
					t.Fatal("Poll is missing")
				}
				if m.Poll.Question != "Where shall we eat?" {
					t.Errorf("Question = %q", m.Poll.Question)
				}
				if len(m.Poll.Options) != 2 {
					t.Fatalf("Options = %d, want 2", len(m.Poll.Options))
				}
				if m.Poll.Options[0].Name != "Pizza" || m.Poll.Options[0].Votes != 3 {
					t.Errorf("first answer = %+v", m.Poll.Options[0])
				}
				if m.Poll.TotalVotes() != 4 {
					t.Errorf("TotalVotes() = %d, want 4", m.Poll.TotalVotes())
				}
			},
		},
		{
			name: "an answered video call keeps its length",
			id:   22,
			check: func(t *testing.T, m model.Message) {
				if m.Call == nil {
					t.Fatal("Call is missing")
				}
				if !m.Call.Video {
					t.Error("Video = false")
				}
				if m.Call.Duration != 125*time.Second {
					t.Errorf("Duration = %v, want 2m5s", m.Call.Duration)
				}
				if !m.Call.Answered() {
					t.Error("Answered() = false for a call that lasted two minutes")
				}
			},
		},
		{
			name: "a missed call is recorded as missed",
			id:   23,
			check: func(t *testing.T, m model.Message) {
				if m.Call == nil {
					t.Fatal("Call is missing")
				}
				if m.Call.Outcome != model.CallMissed {
					t.Errorf("Outcome = %v, want missed", m.Call.Outcome)
				}
				if m.Call.Answered() {
					t.Error("Answered() = true for a missed call")
				}
			},
		},
		{
			name: "a shared link keeps the page title and description",
			id:   24,
			check: func(t *testing.T, m model.Message) {
				if m.Link == nil {
					t.Fatal("Link is missing")
				}
				if m.Link.URL != "https://example.org/article" {
					t.Errorf("URL = %q", m.Link.URL)
				}
				if m.Link.Title != "An article" {
					t.Errorf("Title = %q", m.Link.Title)
				}
				if m.Link.Description == "" {
					t.Error("Description is empty")
				}
				if m.Link.IsEmpty() {
					t.Error("IsEmpty() = true for a filled preview")
				}
			},
		},
		{
			name: "a shared contact keeps the card exactly as it was sent",
			id:   25,
			check: func(t *testing.T, m model.Message) {
				if len(m.Contacts) != 1 {
					t.Fatalf("Contacts = %d, want 1", len(m.Contacts))
				}
				card := m.Contacts[0]
				if card.Name != "Dana Smith" {
					t.Errorf("Name = %q", card.Name)
				}
				if card.VCard == "" {
					t.Error("the card itself was not kept")
				}
				if len(card.JIDs) != 1 {
					t.Errorf("JIDs = %d, want the address it matched", len(card.JIDs))
				}
			},
		},
		{
			name: "a disappearing message says how long it was set to last",
			id:   26,
			check: func(t *testing.T, m model.Message) {
				if !m.IsDisappearing() {
					t.Fatal("IsDisappearing() = false")
				}
				if m.Expires != 7*24*time.Hour {
					t.Errorf("Expires = %v, want a week", m.Expires)
				}
			},
		},
		{
			name: "a group invitation keeps the group's name",
			id:   27,
			check: func(t *testing.T, m model.Message) {
				if m.Invite == nil {
					t.Fatal("Invite is missing")
				}
				if m.Invite.GroupName != "Book club" {
					t.Errorf("GroupName = %q", m.Invite.GroupName)
				}
				if m.Kind != model.KindInvite {
					t.Errorf("Kind = %v, want invite", m.Kind)
				}
			},
		},
		{
			name: "an album says how many things were sent together",
			id:   28,
			check: func(t *testing.T, m model.Message) {
				if m.AlbumSize != 4 {
					t.Errorf("AlbumSize = %d, want 4", m.AlbumSize)
				}
				if m.Kind != model.KindAlbum {
					t.Errorf("Kind = %v, want album", m.Kind)
				}
				// The container itself is not shown; its pictures are separate messages.
				if m.Displayable() {
					t.Error("Displayable() = true for an album container with no text")
				}
			},
		},
		{
			name: "a picture whose file is gone still has a recognisable preview",
			id:   29,
			check: func(t *testing.T, m model.Message) {
				if m.Attachment == nil {
					t.Fatal("Attachment is missing")
				}
				if !m.Attachment.HasPreview() {
					t.Fatal("HasPreview() = false; the surviving image was lost")
				}
				if len(m.Attachment.Preview.Data) < 4 {
					t.Errorf("the preview is only %d bytes", len(m.Attachment.Preview.Data))
				}
				if !m.HasRecoveredContent() {
					t.Error("HasRecoveredContent() = false despite a surviving preview")
				}
			},
		},
		{
			name: "a widely forwarded message says how far it travelled",
			id:   30,
			check: func(t *testing.T, m model.Message) {
				if !m.Forwarded {
					t.Error("Forwarded = false")
				}
				if m.ForwardScore != 7 {
					t.Errorf("ForwardScore = %d, want 7", m.ForwardScore)
				}
				if !m.WasForwardedMany() {
					t.Error("WasForwardedMany() = false for a score of seven")
				}
			},
		},
		{
			name: "a deleted message records who removed it and when",
			id:   31,
			check: func(t *testing.T, m model.Message) {
				if m.Deleted == nil {
					t.Fatal("Deleted is missing")
				}
				if !m.Deleted.ByAdmin() {
					t.Error("ByAdmin() = false, but an administrator removed it")
				}
				if m.Deleted.At.IsZero() {
					t.Error("the time of deletion was not recorded")
				}
				if !m.WasDeleted() || m.Kind != model.KindDeleted {
					t.Errorf("Kind = %v, want deleted", m.Kind)
				}
			},
		},
		{
			name: "a row the app never shows is recognised rather than unknown",
			id:   32,
			check: func(t *testing.T, m model.Message) {
				if m.Kind != model.KindIgnored {
					t.Errorf("Kind = %v, want ignored", m.Kind)
				}
				if m.Displayable() {
					t.Error("Displayable() = true for a row WhatsApp itself hides")
				}
			},
		},
		{
			name: "a reply to a photo shows what was replied to",
			id:   33,
			check: func(t *testing.T, m model.Message) {
				if m.Quote == nil {
					t.Fatal("Quote is missing")
				}
				if m.Quote.Attachment == nil {
					t.Fatal("the quoted attachment was not recovered")
				}
				if !m.Quote.Attachment.HasPreview() {
					t.Error("the quoted photo has no surviving preview")
				}
				if m.Quote.Text != "the photo replied to" {
					t.Errorf("quoted text = %q, want the caption", m.Quote.Text)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, ok := msgs[tt.id]
			if !ok {
				t.Fatalf("message %d is missing", tt.id)
			}
			tt.check(t, m)
		})
	}
}

// TestRecoversSystemNotices covers the largest single gap an archive can have.
// In a real database, 28,361 of 28,909 notices carry no text of their own: what
// happened is a numeric code, and the particulars are spread across a dozen tables.
func TestRecoversSystemNotices(t *testing.T) {
	t.Parallel()

	msgs := readAll(t, openFixture(t), chatGroup)

	tests := []struct {
		name   string
		id     int64
		action int
		want   string
	}{
		{
			name:   "a security code change names the other person",
			id:     40,
			action: actionSecurityCode,
			want:   "Your security code with +34600111222 changed",
		},
		{
			name:   "people added to a group are named",
			id:     41,
			action: actionAddedToGroupNew,
			want:   "+34600111222 added ~Carol Q and ~Carol Q",
		},
		{
			name:   "a subject change shows what it was and what it became",
			id:     42,
			action: actionSubjectChanged,
			want:   `+34600111222 changed the subject from "Old subject" to "Weekend plans"`,
		},
		{
			name: "a notice about the archive's owner reads the right way round",
			// Without this, the owner appears to have added themselves.
			id:     43,
			action: actionAddedToGroupNew,
			want:   "+34600111222 added you",
		},
		{
			name:   "a linked device change counts the devices",
			id:     44,
			action: actionDeviceChanged,
			want:   "+34600111222 added 2 devices and removed 1 device",
		},
		{
			name:   "the encryption banner is recognised",
			id:     45,
			action: actionEncryptionBanner,
			want:   "Messages and calls are end-to-end encrypted",
		},
		{
			name: "a code no source identifies is described, never invented",
			// Guessing here would mislabel real messages, so the archive says what it
			// recovered and admits the rest.
			id:     46,
			action: 165,
			want:   "Unrecognised notice from +34600111222, code 165",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, ok := msgs[tt.id]
			if !ok {
				t.Fatalf("notice %d is missing", tt.id)
			}
			if m.Notice == nil {
				t.Fatal("Notice is missing")
			}
			if m.Notice.Action != tt.action {
				t.Errorf("Action = %d, want %d", m.Notice.Action, tt.action)
			}
			if m.Kind != model.KindSystem {
				t.Errorf("Kind = %v, want system", m.Kind)
			}
			if m.SystemText != tt.want {
				t.Errorf("SystemText =\n  %q\nwant\n  %q", m.SystemText, tt.want)
			}
		})
	}

	t.Run("every notice is phrased somehow", func(t *testing.T) {
		for _, m := range msgs {
			if m.Notice != nil && m.SystemText == "" {
				t.Errorf("notice %d has no wording at all", m.ID)
			}
		}
	})
}

func TestCallOutcome(t *testing.T) {
	t.Parallel()

	valid := func(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }
	absent := sql.NullInt64{}

	tests := []struct {
		name     string
		result   sql.NullInt64
		duration sql.NullInt64
		want     model.CallOutcome
	}{
		{
			// The result codes shift between WhatsApp versions, so a call that lasted
			// was answered whatever its code claims.
			name: "any call that lasted was answered", result: valid(2), duration: valid(30),
			want: model.CallConnected,
		},
		{name: "recorded as connected", result: valid(5), duration: valid(0), want: model.CallConnected},
		{name: "recorded as missed", result: valid(2), duration: valid(0), want: model.CallMissed},
		{name: "recorded as declined", result: valid(3), duration: absent, want: model.CallDeclined},
		{name: "another declined code", result: valid(4), duration: absent, want: model.CallDeclined},
		{name: "no result at all", result: absent, duration: absent, want: model.CallOutcomeUnknown},
		{name: "a code nobody documents", result: valid(42), duration: valid(0), want: model.CallOutcomeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := callOutcome(tt.result, tt.duration); got != tt.want {
				t.Errorf("callOutcome() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNameFromVCard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		card string
		want string
	}{
		{
			name: "a card with a display name",
			card: "BEGIN:VCARD\nVERSION:3.0\nFN:Dana Smith\nEND:VCARD",
			want: "Dana Smith",
		},
		{
			name: "surrounding whitespace is trimmed",
			card: "BEGIN:VCARD\r\nFN:  Dana Smith  \r\nEND:VCARD",
			want: "Dana Smith",
		},
		{name: "a card with no display name", card: "BEGIN:VCARD\nN:Smith;Dana\nEND:VCARD", want: ""},
		{name: "not a card at all", card: "nonsense", want: ""},
		{name: "nothing", card: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := nameFromVCard(tt.card); got != tt.want {
				t.Errorf("nameFromVCard() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRemainingNoticeDetails covers the notice tables that only a few archives
// contain, so a rare event is still recovered rather than reported as bare.
func TestRemainingNoticeDetails(t *testing.T) {
	t.Parallel()

	msgs := readAll(t, openFixture(t), chatGroup)

	tests := []struct {
		name  string
		id    int64
		check func(t *testing.T, m model.Message)
	}{
		{
			name: "a phone number change names both numbers",
			id:   47,
			check: func(t *testing.T, m model.Message) {
				if m.Notice.Old == "" || m.Notice.New == "" {
					t.Fatalf("the old and new numbers were not recovered: %+v", m.Notice)
				}
				if !strings.Contains(m.SystemText, "changed their phone number") {
					t.Errorf("SystemText = %q", m.SystemText)
				}
			},
		},
		{
			name: "a business notice names the business",
			id:   48,
			check: func(t *testing.T, m model.Message) {
				if m.Notice.Business != "A shop" {
					t.Errorf("Business = %q", m.Notice.Business)
				}
				if !strings.Contains(m.SystemText, "A shop") {
					t.Errorf("SystemText = %q", m.SystemText)
				}
			},
		},
		{
			name: "a blocked contact is recorded as blocked",
			id:   49,
			check: func(t *testing.T, m model.Message) {
				if !m.Notice.Blocked {
					t.Error("Blocked = false")
				}
				if m.SystemText != "You blocked this contact" {
					t.Errorf("SystemText = %q", m.SystemText)
				}
			},
		},
		{
			name: "a community change names the group",
			id:   50,
			check: func(t *testing.T, m model.Message) {
				if m.Notice.Subject != "Neighbours" {
					t.Errorf("Subject = %q", m.Notice.Subject)
				}
			},
		},
		{
			name: "an unidentified code still reports the detail it carried",
			id:   51,
			check: func(t *testing.T, m model.Message) {
				if m.Notice.Old != "oldname" || m.Notice.New != "newname" {
					t.Errorf("the username change was not recovered: %+v", m.Notice)
				}
				// The code is unknown, so the wording describes rather than asserts,
				// but the values it carried are still reported.
				if !strings.Contains(m.SystemText, "oldname") {
					t.Errorf("SystemText = %q, want it to mention what was recovered", m.SystemText)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, ok := msgs[tt.id]
			if !ok {
				t.Fatalf("notice %d is missing", tt.id)
			}
			if m.Notice == nil {
				t.Fatal("Notice is missing")
			}
			tt.check(t, m)
		})
	}

	t.Run("a group message with no recorded sender is still read", func(t *testing.T) {
		m, ok := msgs[52]
		if !ok {
			t.Fatal("the message is missing")
		}
		if m.Text != "sender not recorded" {
			t.Errorf("Text = %q", m.Text)
		}
		if !m.Sender.IsZero() {
			t.Errorf("Sender = %v, want none for a group message with no sender column", m.Sender)
		}
	})
}
