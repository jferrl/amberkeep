package model

import (
	"strconv"
	"time"
)

// The types here describe what a message contained beyond plain words. WhatsApp
// keeps each of them in its own table, and an archive that ignores those tables
// turns a shared place, a poll or a contact card into a blank line. Recovering them
// is the difference between an archive and a transcript.

// Place is somewhere a message pointed at, whether pinned once or shared live.
type Place struct {
	Latitude  float64
	Longitude float64
	// Name and Address are present when the sender picked a known place rather
	// than dropping a pin.
	Name    string
	Address string
	URL     string
	// Live marks a location that was shared continuously for a while rather than
	// sent once.
	Live bool
	// SharedFor is how long a live location was shared.
	SharedFor time.Duration
}

// HasCoordinates reports whether the place can actually be put on a map. Some rows
// survive with a name but no usable position.
func (p Place) HasCoordinates() bool {
	return p.Latitude != 0 || p.Longitude != 0
}

// Label is the best short description of the place: its name, then its address,
// then its coordinates. It never returns an empty string.
func (p Place) Label() string {
	switch {
	case p.Name != "":
		return p.Name
	case p.Address != "":
		return p.Address
	default:
		return formatCoordinates(p.Latitude, p.Longitude)
	}
}

// PollOption is one answer people could choose.
type PollOption struct {
	Name string
	// Votes is how many people chose it, as WhatsApp counted at the time.
	Votes int
}

// Poll is a question a message asked.
//
// WhatsApp stores the question as the message's own text and the answers in a
// separate table, so a poll read without that table looks like a bare sentence.
type Poll struct {
	Question string
	Options  []PollOption
	// Selectable is how many answers each person could choose.
	Selectable int
	// Closed marks a poll that was ended by whoever created it.
	Closed bool
}

// TotalVotes is how many votes were cast across every answer.
func (p Poll) TotalVotes() int {
	var n int
	for _, o := range p.Options {
		n += o.Votes
	}
	return n
}

// CallOutcome is how a call ended.
type CallOutcome uint8

// The outcomes an archive distinguishes. Anything else is recorded as unknown
// rather than guessed at.
const (
	CallOutcomeUnknown CallOutcome = iota
	CallConnected
	CallMissed
	CallDeclined
	CallFailed
)

// String names the outcome for diagnostics.
func (o CallOutcome) String() string {
	switch o {
	case CallConnected:
		return "connected"
	case CallMissed:
		return "missed"
	case CallDeclined:
		return "declined"
	case CallFailed:
		return "failed"
	case CallOutcomeUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// Call is an entry in the call history.
type Call struct {
	Video    bool
	Group    bool
	Outcome  CallOutcome
	Duration time.Duration
}

// Answered reports whether the call actually connected. A call with a duration
// connected regardless of how its outcome was recorded, which matters because the
// outcome codes vary between WhatsApp versions.
func (c Call) Answered() bool {
	return c.Outcome == CallConnected || c.Duration > 0
}

// LinkPreview is what WhatsApp showed beneath a shared link: the page's title and
// description as they were when the message was sent.
//
// This is genuinely irrecoverable elsewhere. The page may have changed or vanished
// since, so the preview is often the only surviving record of what was shared.
type LinkPreview struct {
	URL         string
	Title       string
	Description string
}

// IsEmpty reports whether the preview holds nothing worth showing.
func (l LinkPreview) IsEmpty() bool {
	return l.URL == "" && l.Title == "" && l.Description == ""
}

// ContactCard is somebody's details, shared into a conversation.
//
// The raw vCard is kept verbatim rather than parsed into fields, because it is the
// thing the sender actually sent and any parsing loses something. Name is pulled
// out for display.
type ContactCard struct {
	Name string
	// VCard is the card exactly as it was sent, in vCard format.
	VCard string
	// JIDs are the WhatsApp addresses the card resolved to, when it resolved to any.
	JIDs []JID
}

// Deletion records that a message was deleted for everyone, and by whom.
//
// The words are gone, but the fact is not, and in a group it matters whether the
// sender withdrew their own message or an administrator removed it.
type Deletion struct {
	At time.Time
	// By is the administrator who removed it, when an administrator did. It is the
	// zero value when the sender deleted their own message.
	By JID
}

// ByAdmin reports whether somebody other than the sender removed the message.
func (d Deletion) ByAdmin() bool { return !d.By.IsZero() }

// GroupInvite is an invitation to a group, sent into a conversation.
type GroupInvite struct {
	GroupName string
	Group     JID
	ExpiresAt time.Time
}

// Thumbnail is a small copy of a picture or video, stored inside the message
// database itself.
//
// This is the most valuable thing an archive can recover when the media files are
// gone: the original photo may be long deleted, but a recognisable preview of it
// survives here.
type Thumbnail struct {
	// Data is the encoded image, almost always JPEG.
	Data []byte
}

// IsEmpty reports whether there is no preview to show.
func (t Thumbnail) IsEmpty() bool { return len(t.Data) == 0 }

// formatCoordinates renders a position with enough precision to find a building
// and no more.
func formatCoordinates(lat, lon float64) string {
	return trimFloat(lat) + ", " + trimFloat(lon)
}

// trimFloat formats a coordinate to five decimal places, about a metre, without
// trailing zeroes.
func trimFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', 5, 64)
	for s != "" && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if s != "" && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}

// Notice is what WhatsApp itself recorded about a conversation: somebody joined,
// the subject changed, a phone number moved, a security code was renewed.
//
// A real archive is roughly two per cent notices, and a reader that skips them
// loses the shape of a group's history: who was added, by whom, and when.
//
// The facts are kept separate from the wording. Which sentence to show, and in
// which language, is a question for the presentation layer; what happened is a
// question for the source, and this is the answer.
type Notice struct {
	// Action is the source's own code for what happened. It is kept so that a
	// notice this build cannot phrase is still reported honestly rather than
	// silently dropped.
	Action int

	// Actor is who did it, when the source recorded anybody.
	Actor JID
	// Targets are who it was done to: the people added, removed or invited.
	Targets []JID

	// Old and New carry whatever changed: a group subject, a phone number, a
	// username. Which of them is filled depends on Action.
	Old string
	New string

	// Subject is a group's name, when the notice concerned one.
	Subject string
	// Business is the verified name of a business account, for the notices
	// WhatsApp shows about them.
	Business string

	// DevicesAdded and DevicesRemoved count the linked devices a security notice
	// reported changing.
	DevicesAdded   int
	DevicesRemoved int

	// Joined marks a group notice about the archive's owner rather than somebody else.
	Joined bool
	// Blocked marks a notice about blocking or unblocking a contact.
	Blocked bool
}

// HasDetail reports whether anything beyond the bare action code was recovered.
// A notice without detail can still be counted and dated, but it cannot be phrased
// with any specifics.
func (n Notice) HasDetail() bool {
	return !n.Actor.IsZero() || len(n.Targets) > 0 || n.Old != "" || n.New != "" ||
		n.Subject != "" || n.Business != "" || n.DevicesAdded > 0 || n.DevicesRemoved > 0
}
