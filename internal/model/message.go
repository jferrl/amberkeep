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
		KindDeleted, KindSystem, KindCall, KindPayment:
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
		KindEvent, KindDeleted, KindViewOnce, KindPayment:
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
}

// Quote is the message a reply pointed at, as far as it can be recovered. WhatsApp
// stores a copy of the quoted content rather than a reference, so a quote survives
// even when the original message is gone.
type Quote struct {
	Sender JID
	FromMe bool
	Kind   Kind
	Text   string
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

	Starred   bool
	Forwarded bool
	EditedAt  time.Time

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

// Displayable reports whether the message should appear in a conversation at all.
// Some rows in a source database are bookkeeping rather than content.
func (m Message) Displayable() bool {
	return m.HasText() || m.Attachment != nil || m.Kind != KindUnknown
}
