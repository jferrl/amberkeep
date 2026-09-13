package model

import "time"

// Kind is what a message is. It is deliberately smaller than the set of numeric
// codes WhatsApp uses: several codes mean "a photo" and callers should not care
// which. A code we do not recognise becomes KindUnknown and keeps its original
// number in Message.SourceType so nothing is silently lost.
type Kind uint8

// The kinds an archive can contain.
const (
	KindUnknown Kind = iota
	KindText
	KindImage
	KindVideo
	KindAudio // an audio file that was sent as a file
	KindVoice // a recorded voice message
	KindDocument
	KindSticker
	KindGIF
	KindContact  // one or more shared contact cards
	KindLocation // a place, whether pinned once or shared live
	KindPoll
	KindEvent
	KindDeleted  // deleted for everyone, by the sender or by an administrator
	KindSystem   // a notice from WhatsApp: someone joined, the subject changed
	KindCall     // an entry in the call history
	KindViewOnce // media that could be opened once
	KindPayment
	// KindAlbum is the container row WhatsApp writes when several pictures were
	// sent at once. The pictures themselves are separate messages.
	KindAlbum
	// KindInvite is an invitation to join a group.
	KindInvite
	// KindInteractive is a business message built from buttons, lists or cards.
	// It usually carries readable text, which is the part worth keeping.
	KindInteractive
	// KindIgnored is a row WhatsApp itself does not display. It is recognised
	// rather than unknown, so it can be left out without being reported as a gap.
	KindIgnored
)

// String names the kind for diagnostics. User-visible wording is chosen by the
// presentation layer, which knows the reader's language; this is not that.
func (k Kind) String() string {
	switch k {
	case KindText:
		return "text"
	case KindImage:
		return "image"
	case KindVideo:
		return "video"
	case KindAudio:
		return "audio"
	case KindVoice:
		return "voice"
	case KindDocument:
		return "document"
	case KindSticker:
		return "sticker"
	case KindGIF:
		return "gif"
	case KindContact:
		return "contact"
	case KindLocation:
		return "location"
	case KindPoll:
		return "poll"
	case KindEvent:
		return "event"
	case KindDeleted:
		return "deleted"
	case KindSystem:
		return "system"
	case KindCall:
		return "call"
	case KindViewOnce:
		return "view-once"
	case KindPayment:
		return "payment"
	case KindAlbum:
		return "album"
	case KindInvite:
		return "invite"
	case KindInteractive:
		return "interactive"
	case KindIgnored:
		return "ignored"
	case KindUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// HasAttachment reports whether the kind refers to a file that lived outside the
// message database. Archives carry the description of those files, not the files.
func (k Kind) HasAttachment() bool {
	switch k {
	case KindImage, KindVideo, KindAudio, KindVoice, KindDocument, KindSticker, KindGIF, KindViewOnce:
		return true
	case KindUnknown, KindText, KindContact, KindLocation, KindPoll, KindEvent,
		KindDeleted, KindSystem, KindCall, KindPayment, KindAlbum, KindInvite,
		KindInteractive, KindIgnored:
		return false
	default:
		return false
	}
}

// IsNotice reports whether the kind describes something WhatsApp did rather than
// something a person wrote: a security-code change, a business notice, someone
// joining a group, a call that was placed. Exports leave these out by default
// because they add noise to a conversation without adding anybody's words.
func (k Kind) IsNotice() bool {
	switch k {
	case KindSystem, KindCall:
		return true
	case KindUnknown, KindText, KindImage, KindVideo, KindAudio, KindVoice,
		KindDocument, KindSticker, KindGIF, KindContact, KindLocation, KindPoll,
		KindEvent, KindDeleted, KindViewOnce, KindPayment, KindAlbum, KindInvite,
		KindInteractive, KindIgnored:
		return false
	default:
		return false
	}
}

// Attachment describes a file a message referred to. The file itself is not part
// of an archive: photos and videos already live in the phone's gallery, and copying
// gigabytes to say what is already there helps nobody.
type Attachment struct {
	MediaType string // the reported media type, such as "image/jpeg"
	FileName  string // the original name, for documents
	Size      int64  // in bytes, as recorded
	Duration  time.Duration
	Width     int
	Height    int
	// Caption is the text sent alongside the file.
	Caption string

	// Preview is a small copy of the picture or video kept inside the message
	// database. When the original file is gone, and for an old archive it usually
	// is, this is the only surviving image of what was sent.
	Preview Thumbnail
}

// HasPreview reports whether a recognisable image of the attachment survives even
// though the file itself may not.
func (a Attachment) HasPreview() bool { return !a.Preview.IsEmpty() }

// Quote is the message a reply pointed at, as far as it can be recovered. WhatsApp
// stores a copy of the quoted content rather than a reference, so a quote survives
// even when the original message is gone.
type Quote struct {
	Sender JID
	FromMe bool
	Kind   Kind
	Text   string

	// Attachment describes the file the quoted message carried, including its
	// embedded preview, so a reply to a photo still shows what was replied to.
	Attachment *Attachment
	// Place is the location the quoted message pointed at.
	Place *Place
}

// Reaction is an emoji someone attached to a message.
type Reaction struct {
	Sender JID
	FromMe bool
	Emoji  string
	At     time.Time
}

// Message is one entry in a conversation.
//
// A message answers questions about itself rather than exposing codes for callers
// to interpret: ask IsFromMe, HasText or Displayable instead of comparing SourceType.
type Message struct {
	ID     int64 // identity within the source database
	ChatID int64

	// Key is WhatsApp's own identifier for the message, stable across devices.
	// It is what makes merging two archives possible without duplicates.
	Key string

	Kind   Kind
	SentAt time.Time

	// Sender is who wrote it. It is the zero value for messages the archive's owner
	// sent; ask IsFromMe rather than comparing.
	Sender   JID
	fromMe   bool
	PushName string // the name the sender had set in WhatsApp at the time

	// Text is the body, or the caption of an attachment.
	Text string

	Attachment *Attachment
	Quote      *Quote
	Reactions  []Reaction
	Mentions   []JID

	// Everything below is content WhatsApp keeps in its own table. Each is nil
	// unless the message actually carried it. Reading them is what turns a shared
	// place, a poll or a contact card from a blank line back into content.
	Place    *Place
	Poll     *Poll
	Call     *Call
	Link     *LinkPreview
	Contacts []ContactCard
	Invite   *GroupInvite

	// Deleted records that the message was withdrawn for everyone, and by whom.
	// The words are gone; the fact that something was said is not.
	Deleted *Deletion

	// AlbumSize is how many items were sent together as one album, counted from
	// the message that opens it.
	AlbumSize int

	// Expires marks a disappearing message and says how long it was set to last.
	Expires time.Duration

	Starred   bool
	Forwarded bool
	// ForwardScore is how many times a message had been forwarded before it
	// arrived. WhatsApp shows anything above four as "forwarded many times".
	ForwardScore int
	EditedAt     time.Time

	// Notice carries the facts behind a system message: who did what to whom.
	// It is nil for anything a person actually wrote.
	Notice *Notice

	// SystemText is the rendered description of a system notice, when one could be
	// reconstructed from the source.
	SystemText string

	// SourceType is the original numeric type from the source database. It exists
	// for diagnostics and for reporting unrecognised kinds, not for branching.
	SourceType int
}

// NewOutgoing marks a message as sent by the archive's owner. Construction goes
// through the readers, so this is the only way the flag is set.
func (m Message) NewOutgoing() Message {
	m.fromMe = true
	m.Sender = JID{}
	return m
}

// IsFromMe reports whether the archive's owner sent this message.
func (m Message) IsFromMe() bool { return m.fromMe }

// HasText reports whether the message carries readable words of its own, as opposed
// to being an attachment, a deleted message or a notice.
func (m Message) HasText() bool { return m.Text != "" }

// WasEdited reports whether the message was edited after it was sent. The text an
// archive holds is always the final version; WhatsApp does not keep the earlier ones.
func (m Message) WasEdited() bool { return !m.EditedAt.IsZero() }

// IsReply reports whether the message answered another one.
func (m Message) IsReply() bool { return m.Quote != nil }

// WasDeleted reports whether the message was withdrawn for everyone.
func (m Message) WasDeleted() bool { return m.Deleted != nil || m.Kind == KindDeleted }

// IsDisappearing reports whether the message was set to vanish on its own.
func (m Message) IsDisappearing() bool { return m.Expires > 0 }

// WasForwardedMany reports whether the message had been passed along enough times
// for WhatsApp to label it as widely forwarded.
func (m Message) WasForwardedMany() bool { return m.ForwardScore > 4 }

// HasRecoveredContent reports whether anything beyond plain words survived for this
// message: a place, a poll, a call, a link preview, a contact card, an invitation,
// or a picture preview. It is what an archive counts to tell the user how much was
// recovered rather than merely listed.
func (m Message) HasRecoveredContent() bool {
	return m.Place != nil || m.Poll != nil || m.Call != nil || m.Link != nil ||
		m.Invite != nil || len(m.Contacts) > 0 ||
		(m.Attachment != nil && m.Attachment.HasPreview())
}

// Displayable reports whether the message should appear in a conversation at all.
//
// Two kinds of row are not content: those WhatsApp itself never shows, and the
// container it writes when several pictures were sent together, whose pictures are
// separate messages that would otherwise be counted twice.
func (m Message) Displayable() bool {
	switch m.Kind {
	case KindIgnored:
		return false
	case KindAlbum:
		return m.HasText()
	default:
		return m.HasText() || m.Attachment != nil || m.Kind != KindUnknown
	}
}
