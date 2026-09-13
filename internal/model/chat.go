package model

import "time"

// ChatKind is what sort of conversation this is.
type ChatKind uint8

// The kinds of conversation an archive can contain.
const (
	ChatUnknown ChatKind = iota
	ChatDirect           // a conversation with one other person
	ChatGroup
	ChatBroadcast  // a broadcast list the owner created
	ChatStatus     // the status feed, excluded from archives by default
	ChatNewsletter // a channel the owner follows
)

// String names the kind for diagnostics.
func (k ChatKind) String() string {
	switch k {
	case ChatDirect:
		return "direct"
	case ChatGroup:
		return "group"
	case ChatBroadcast:
		return "broadcast"
	case ChatStatus:
		return "status"
	case ChatNewsletter:
		return "newsletter"
	case ChatUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// ChatKindOf classifies an address. It is the single place that mapping lives, so
// readers for different platforms cannot disagree about what a group is.
func ChatKindOf(j JID) ChatKind {
	switch {
	case j.IsStatus():
		return ChatStatus
	case j.Server == ServerGroup:
		return ChatGroup
	case j.Server == ServerUser, j.Server == ServerHidden:
		return ChatDirect
	case j.Server == ServerBroadcast:
		return ChatBroadcast
	case j.Server == ServerNewsletter:
		return ChatNewsletter
	default:
		return ChatUnknown
	}
}

// Participant is somebody who belongs to a group.
type Participant struct {
	JID   JID
	Name  string // resolved as far as the sources allowed
	Admin bool
}

// Chat is one conversation, without its messages. Messages are streamed separately
// because a single conversation can hold hundreds of thousands of them.
type Chat struct {
	ID   int64 // identity within the source database
	JID  JID
	Kind ChatKind

	// Name is the group subject, or the best name found for the other person.
	// It may be empty when nothing better than an address was available.
	Name string

	Participants []Participant
	Description  string

	CreatedAt time.Time
	LastAt    time.Time
	Archived  bool

	// Messages is how many messages the source holds for this conversation. It is
	// counted while reading rather than derived later, so a caller can show progress
	// before streaming anything.
	Messages int
}

// Title is what to call this conversation. It prefers a real name, falls back to a
// phone number, and only then to the raw address, so a chat is never nameless.
func (c Chat) Title() string {
	if c.Name != "" {
		return c.Name
	}
	if phone, ok := c.JID.Phone(); ok {
		return "+" + phone
	}
	return c.JID.String()
}

// IsEmpty reports whether the conversation holds no messages. Those exist in real
// databases and are hidden from archives unless asked for.
func (c Chat) IsEmpty() bool { return c.Messages == 0 }

// Includable reports whether this conversation belongs in an archive by default.
// The status feed and empty conversations are excluded; a caller can still ask for
// them explicitly.
func (c Chat) Includable() bool {
	return c.Kind != ChatStatus && !c.IsEmpty()
}
