package model

import (
	"testing"
	"time"
)

func TestParseJID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        string
		wantUser   string
		wantServer Server
		wantGroup  bool
		wantHidden bool
		wantStatus bool
		wantPhone  string
	}{
		{
			name:       "a person addressed by phone number",
			raw:        "34600111222@s.whatsapp.net",
			wantUser:   "34600111222",
			wantServer: ServerUser,
			wantPhone:  "34600111222",
		},
		{
			name:       "a group",
			raw:        "123456789-1600000000@g.us",
			wantUser:   "123456789-1600000000",
			wantServer: ServerGroup,
			wantGroup:  true,
		},
		{
			name:       "a hidden identifier has no phone number of its own",
			raw:        "99887766554433@lid",
			wantUser:   "99887766554433",
			wantServer: ServerHidden,
			wantHidden: true,
		},
		{
			name:       "the status feed",
			raw:        "status@broadcast",
			wantUser:   "status",
			wantServer: ServerBroadcast,
			wantStatus: true,
		},
		{
			name:       "a device suffix names a linked device, not another person",
			raw:        "34600111222:12@s.whatsapp.net",
			wantUser:   "34600111222",
			wantServer: ServerUser,
			wantPhone:  "34600111222",
		},
		{
			name:     "something with no server at all is carried through unchanged",
			raw:      "not-an-address",
			wantUser: "not-an-address",
		},
		{
			name:       "an unfamiliar server does not stop an archive from opening",
			raw:        "abc@newthing",
			wantUser:   "abc",
			wantServer: "newthing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			j := ParseJID(tt.raw)
			if j.User != tt.wantUser {
				t.Errorf("User = %q, want %q", j.User, tt.wantUser)
			}
			if j.Server != tt.wantServer {
				t.Errorf("Server = %q, want %q", j.Server, tt.wantServer)
			}
			if j.IsGroup() != tt.wantGroup {
				t.Errorf("IsGroup() = %v, want %v", j.IsGroup(), tt.wantGroup)
			}
			if j.IsHidden() != tt.wantHidden {
				t.Errorf("IsHidden() = %v, want %v", j.IsHidden(), tt.wantHidden)
			}
			if j.IsStatus() != tt.wantStatus {
				t.Errorf("IsStatus() = %v, want %v", j.IsStatus(), tt.wantStatus)
			}
			phone, ok := j.Phone()
			if ok != (tt.wantPhone != "") || phone != tt.wantPhone {
				t.Errorf("Phone() = %q, %v; want %q", phone, ok, tt.wantPhone)
			}
			if j.String() != tt.raw {
				t.Errorf("String() = %q, want the original %q", j.String(), tt.raw)
			}
			if j.IsZero() {
				t.Error("IsZero() = true for a real address")
			}
		})
	}

	t.Run("the zero address reports itself as absent", func(t *testing.T) {
		t.Parallel()
		if !(JID{}).IsZero() {
			t.Error("IsZero() = false for the zero address")
		}
	})
}

func TestChatKindOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want ChatKind
	}{
		{"34600111222@s.whatsapp.net", ChatDirect},
		{"99887766554433@lid", ChatDirect},
		{"123456789-1600000000@g.us", ChatGroup},
		{"status@broadcast", ChatStatus},
		{"1600000000@broadcast", ChatBroadcast},
		{"1234@newsletter", ChatNewsletter},
		{"something@unknown-server", ChatUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()
			if got := ChatKindOf(ParseJID(tt.raw)); got != tt.want {
				t.Errorf("ChatKindOf() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChatTitleAndInclusion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		chat         Chat
		wantTitle    string
		wantIncluded bool
	}{
		{
			name:         "a named group",
			chat:         Chat{JID: ParseJID("1-2@g.us"), Kind: ChatGroup, Name: "Weekend plans", Messages: 5},
			wantTitle:    "Weekend plans",
			wantIncluded: true,
		},
		{
			name:         "a person with no name falls back to their number",
			chat:         Chat{JID: ParseJID("34600111222@s.whatsapp.net"), Kind: ChatDirect, Messages: 2},
			wantTitle:    "+34600111222",
			wantIncluded: true,
		},
		{
			name:         "a hidden identifier with no name falls back to the address",
			chat:         Chat{JID: ParseJID("99887766@lid"), Kind: ChatDirect, Messages: 1},
			wantTitle:    "99887766@lid",
			wantIncluded: true,
		},
		{
			name:         "an empty conversation is left out",
			chat:         Chat{JID: ParseJID("34600111222@s.whatsapp.net"), Kind: ChatDirect},
			wantTitle:    "+34600111222",
			wantIncluded: false,
		},
		{
			name:         "the status feed is left out even when it has content",
			chat:         Chat{JID: ParseJID("status@broadcast"), Kind: ChatStatus, Messages: 40},
			wantTitle:    "status@broadcast",
			wantIncluded: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.chat.Title(); got != tt.wantTitle {
				t.Errorf("Title() = %q, want %q", got, tt.wantTitle)
			}
			if got := tt.chat.Includable(); got != tt.wantIncluded {
				t.Errorf("Includable() = %v, want %v", got, tt.wantIncluded)
			}
		})
	}
}

func TestContactDisplayName(t *testing.T) {
	t.Parallel()

	jid := ParseJID("34600111222@s.whatsapp.net")

	tests := []struct {
		name       string
		contact    Contact
		want       string
		identified bool
	}{
		{
			name:       "a name from the address book wins",
			contact:    Contact{JID: jid, Name: "Alice", PushName: "ali", Phone: "34600111222"},
			want:       "Alice",
			identified: true,
		},
		{
			name:       "a self-chosen name is marked as such",
			contact:    Contact{JID: jid, PushName: "ali", Phone: "34600111222"},
			want:       "~ali",
			identified: true,
		},
		{
			name:    "a number is better than an address",
			contact: Contact{JID: jid, Phone: "34600111222"},
			want:    "+34600111222",
		},
		{
			name:    "an address is the last resort",
			contact: Contact{JID: ParseJID("99887766@lid")},
			want:    "99887766@lid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.contact.DisplayName(); got != tt.want {
				t.Errorf("DisplayName() = %q, want %q", got, tt.want)
			}
			if got := tt.contact.IsIdentified(); got != tt.identified {
				t.Errorf("IsIdentified() = %v, want %v", got, tt.identified)
			}
		})
	}
}

func TestDirectory(t *testing.T) {
	t.Parallel()

	hidden := ParseJID("99887766554433@lid")
	phone := ParseJID("34600333444@s.whatsapp.net")
	stranger := ParseJID("34699999999@s.whatsapp.net")

	t.Run("a name on a hidden identifier reaches the phone address", func(t *testing.T) {
		t.Parallel()

		d := NewDirectory()
		d.Add(Contact{JID: hidden, PushName: "Carol Q"})
		d.Add(Contact{JID: phone})
		d.Alias(hidden, phone)

		if got := d.NameOf(phone); got != "~Carol Q" {
			t.Errorf("NameOf(phone) = %q, want %q", got, "~Carol Q")
		}
		if got := d.NameOf(hidden); got != "~Carol Q" {
			t.Errorf("NameOf(hidden) = %q, want %q", got, "~Carol Q")
		}
	})

	t.Run("a name on the phone address reaches the hidden identifier", func(t *testing.T) {
		t.Parallel()

		d := NewDirectory()
		d.Add(Contact{JID: phone, Name: "Carol at work"})
		d.Alias(hidden, phone)

		if got := d.NameOf(hidden); got != "Carol at work" {
			t.Errorf("NameOf(hidden) = %q, want %q", got, "Carol at work")
		}
	})

	t.Run("the address book outranks a self-chosen name", func(t *testing.T) {
		t.Parallel()

		d := NewDirectory()
		d.Add(Contact{JID: phone, PushName: "carolq"})
		d.Add(Contact{JID: phone, Name: "Carol"})

		if got := d.NameOf(phone); got != "Carol" {
			t.Errorf("NameOf() = %q, want %q", got, "Carol")
		}
	})

	t.Run("adding nothing never erases what is known", func(t *testing.T) {
		t.Parallel()

		d := NewDirectory()
		d.Add(Contact{JID: phone, Name: "Carol", PushName: "carolq"})
		d.Add(Contact{JID: phone})

		c := d.Lookup(phone)
		if c.Name != "Carol" || c.PushName != "carolq" {
			t.Errorf("Lookup() lost information: %+v", c)
		}
	})

	t.Run("an unknown address still names itself usefully", func(t *testing.T) {
		t.Parallel()

		d := NewDirectory()
		if got := d.NameOf(stranger); got != "+34699999999" {
			t.Errorf("NameOf() = %q, want %q", got, "+34699999999")
		}
	})

	t.Run("two records of one number are matched even without a link", func(t *testing.T) {
		t.Parallel()

		d := NewDirectory()
		d.Add(Contact{JID: phone, Name: "Carol"})
		// The same number arriving as a bare address, as an address book import does.
		if got := d.NameOf(ParseJID("34600333444@s.whatsapp.net")); got != "Carol" {
			t.Errorf("NameOf() = %q, want %q", got, "Carol")
		}
	})

	t.Run("the owner is returned for the absent address", func(t *testing.T) {
		t.Parallel()

		d := NewDirectory()
		d.SetOwner(Contact{Name: "Owner"})
		if got := d.NameOf(JID{}); got != "Owner" {
			t.Errorf("NameOf(zero) = %q, want %q", got, "Owner")
		}
	})

	t.Run("counts report how complete the address book is", func(t *testing.T) {
		t.Parallel()

		d := NewDirectory()
		d.Add(Contact{JID: phone, Name: "Carol"})
		d.Add(Contact{JID: stranger})
		if d.Len() != 2 {
			t.Errorf("Len() = %d, want 2", d.Len())
		}
		if d.Identified() != 1 {
			t.Errorf("Identified() = %d, want 1", d.Identified())
		}
	})

	t.Run("an incomplete link is ignored rather than corrupting the index", func(t *testing.T) {
		t.Parallel()

		d := NewDirectory()
		d.Alias(JID{}, phone)
		d.Alias(hidden, JID{})
		if got := d.NameOf(hidden); got != hidden.String() {
			t.Errorf("NameOf() = %q, want the raw address", got)
		}
	})
}

func TestMessagePredicates(t *testing.T) {
	t.Parallel()

	sent := time.Unix(1_700_000_000, 0).UTC()

	tests := []struct {
		name         string
		message      Message
		wantFromMe   bool
		wantHasText  bool
		wantEdited   bool
		wantReply    bool
		wantNotice   bool
		wantAttached bool
	}{
		{
			name:        "a plain incoming message",
			message:     Message{Kind: KindText, Text: "hello", SentAt: sent},
			wantHasText: true,
		},
		{
			name:       "an outgoing message",
			message:    Message{Kind: KindText, Text: "hi"}.NewOutgoing(),
			wantFromMe: true, wantHasText: true,
		},
		{
			name:        "an edited message",
			message:     Message{Kind: KindText, Text: "fixed", SentAt: sent, EditedAt: sent.Add(time.Minute)},
			wantHasText: true, wantEdited: true,
		},
		{
			name:        "a reply",
			message:     Message{Kind: KindText, Text: "yes", Quote: &Quote{Text: "?"}},
			wantHasText: true, wantReply: true,
		},
		{
			name:       "a notice",
			message:    Message{Kind: KindSystem, SystemText: "someone joined"},
			wantNotice: true,
		},
		{
			name:         "a photo",
			message:      Message{Kind: KindImage, Attachment: &Attachment{MediaType: "image/jpeg"}},
			wantAttached: true,
		},
		{
			name:       "a call",
			message:    Message{Kind: KindCall},
			wantNotice: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := tt.message
			if m.IsFromMe() != tt.wantFromMe {
				t.Errorf("IsFromMe() = %v, want %v", m.IsFromMe(), tt.wantFromMe)
			}
			if m.HasText() != tt.wantHasText {
				t.Errorf("HasText() = %v, want %v", m.HasText(), tt.wantHasText)
			}
			if m.WasEdited() != tt.wantEdited {
				t.Errorf("WasEdited() = %v, want %v", m.WasEdited(), tt.wantEdited)
			}
			if m.IsReply() != tt.wantReply {
				t.Errorf("IsReply() = %v, want %v", m.IsReply(), tt.wantReply)
			}
			if m.Kind.IsNotice() != tt.wantNotice {
				t.Errorf("Kind.IsNotice() = %v, want %v", m.Kind.IsNotice(), tt.wantNotice)
			}
			if m.Kind.HasAttachment() != tt.wantAttached {
				t.Errorf("Kind.HasAttachment() = %v, want %v", m.Kind.HasAttachment(), tt.wantAttached)
			}
		})
	}

	t.Run("marking a message as outgoing removes its sender", func(t *testing.T) {
		t.Parallel()

		m := Message{Sender: ParseJID("34600111222@s.whatsapp.net")}.NewOutgoing()
		if !m.Sender.IsZero() {
			t.Errorf("Sender = %v, want none", m.Sender)
		}
	})
}

// TestKindNamesAreStable pins the names used in diagnostics and in the schema
// report, which people paste into support conversations.
func TestKindNamesAreStable(t *testing.T) {
	t.Parallel()

	tests := map[Kind]string{
		KindText: "text", KindImage: "image", KindVideo: "video", KindAudio: "audio",
		KindVoice: "voice", KindDocument: "document", KindSticker: "sticker",
		KindGIF: "gif", KindContact: "contact", KindLocation: "location",
		KindPoll: "poll", KindEvent: "event", KindDeleted: "deleted",
		KindSystem: "system", KindCall: "call", KindViewOnce: "view-once",
		KindPayment: "payment", KindUnknown: "unknown",
	}
	for kind, want := range tests {
		if got := kind.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", kind, got, want)
		}
	}

	chats := map[ChatKind]string{
		ChatDirect: "direct", ChatGroup: "group", ChatBroadcast: "broadcast",
		ChatStatus: "status", ChatNewsletter: "newsletter", ChatUnknown: "unknown",
	}
	for kind, want := range chats {
		if got := kind.String(); got != want {
			t.Errorf("ChatKind(%d).String() = %q, want %q", kind, got, want)
		}
	}
}
