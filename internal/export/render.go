package export

import (
	"fmt"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

// Turning a message into words.
//
// This is where the reader's work becomes visible. A message that carried a poll,
// a place, a call or a photograph is not plain text, and an export that prints
// only the text column reduces all of it to blank lines.
//
// The wording is English and plain, and it follows what WhatsApp's own export
// produces where that is sensible, so the result is familiar and can be read by
// the tools people already use. It deliberately does not copy the invisible
// direction marks WhatsApp scatters through its exports, which break every parser
// that meets them.

// renderer turns messages into readable lines in one time zone and one language.
type renderer struct {
	dir *model.Directory
	loc *time.Location
	me  string
}

func newRenderer(dir *model.Directory, opts Options) renderer {
	return renderer{dir: dir, loc: opts.Location, me: opts.Me}
}

// timestamp formats when a message was sent, in the reader's own time zone.
//
// The form is unambiguous on purpose: a four-digit year and a day before the
// month, so an archive read in another country still says what it means.
func (r renderer) timestamp(t time.Time) string {
	if t.IsZero() {
		return "[unknown date]"
	}
	return t.In(r.loc).Format("[02/01/2006, 15:04:05]")
}

// sender names whoever wrote a message.
func (r renderer) sender(m model.Message) string {
	if m.IsFromMe() {
		return r.me
	}
	if m.Sender.IsZero() {
		return "Unknown"
	}
	if r.dir != nil {
		return r.dir.NameOf(m.Sender)
	}
	return m.Sender.String()
}

// name resolves anybody else, for quotes and reactions.
func (r renderer) name(j model.JID, fromMe bool) string {
	switch {
	case fromMe:
		return r.me
	case j.IsZero():
		return "Unknown"
	case r.dir != nil:
		return r.dir.NameOf(j)
	default:
		return j.String()
	}
}

// body is the single line a message becomes. It never returns an empty string:
// a message with nothing readable still says what kind of thing it was, because
// a blank line in an archive looks like data loss.
func (r renderer) body(m model.Message) string {
	switch {
	case m.WasDeleted():
		return r.deleted(m)
	case m.Kind == model.KindSystem:
		return firstNonEmpty(m.SystemText, m.Text, "System notice")
	case m.Call != nil:
		return r.call(m)
	case m.Place != nil:
		return r.place(m)
	case m.Poll != nil:
		return r.poll(m)
	case m.Invite != nil:
		return r.invite(m)
	case len(m.Contacts) > 0:
		return r.contacts(m)
	case m.Kind == model.KindAlbum:
		return r.album(m)
	case m.Kind.HasAttachment() || m.Attachment != nil:
		return r.attachment(m)
	case m.HasText():
		return m.Text
	case m.Kind == model.KindUnknown:
		// An unrecognised kind is reported with its number rather than hidden, so a
		// new WhatsApp release shows up as a question instead of a silent gap.
		return fmt.Sprintf("<unrecognised message, type %d>", m.SourceType)
	default:
		return "<" + m.Kind.String() + ">"
	}
}

// deleted says that a message was withdrawn, and by whom when that is known.
func (r renderer) deleted(m model.Message) string {
	if m.Deleted != nil && m.Deleted.ByAdmin() {
		return fmt.Sprintf("This message was deleted by %s", r.name(m.Deleted.By, false))
	}
	if m.IsFromMe() {
		return "You deleted this message"
	}
	return "This message was deleted"
}

// attachment describes a file that is not part of the archive, and says when a
// picture of it survived anyway.
func (r renderer) attachment(m model.Message) string {
	noun := attachmentNoun(m.Kind)
	var parts []string

	if m.Attachment != nil && m.Attachment.FileName != "" {
		parts = append(parts, fmt.Sprintf("<%s omitted: %s>", noun, m.Attachment.FileName))
	} else {
		parts = append(parts, fmt.Sprintf("<%s omitted>", noun))
	}
	if m.Attachment != nil && m.Attachment.Duration > 0 {
		parts = append(parts, "("+formatDuration(m.Attachment.Duration)+")")
	}
	if m.Attachment != nil && m.Attachment.HasPreview() {
		// Worth saying: the file is gone but a recognisable image of it is not.
		parts = append(parts, "[preview recovered]")
	}
	if m.HasText() {
		parts = append(parts, m.Text)
	}
	return strings.Join(parts, " ")
}

// attachmentNoun names the kind of file in the way WhatsApp's own export does.
func attachmentNoun(k model.Kind) string {
	switch k {
	case model.KindImage:
		return "image"
	case model.KindVideo:
		return "video"
	case model.KindVoice:
		return "voice message"
	case model.KindAudio:
		return "audio"
	case model.KindDocument:
		return "document"
	case model.KindSticker:
		return "sticker"
	case model.KindGIF:
		return "GIF"
	case model.KindViewOnce:
		return "view once media"
	case model.KindContact, model.KindUnknown, model.KindText, model.KindLocation,
		model.KindPoll, model.KindEvent, model.KindDeleted, model.KindSystem,
		model.KindCall, model.KindPayment, model.KindAlbum, model.KindInvite,
		model.KindInteractive, model.KindIgnored:
		return "media"
	default:
		return "media"
	}
}

// call describes an entry in the call history.
func (r renderer) call(m model.Message) string {
	kind := "voice call"
	if m.Call.Video {
		kind = "video call"
	}
	if m.Call.Group {
		kind = "group " + kind
	}
	switch {
	case m.Call.Answered():
		if m.Call.Duration > 0 {
			return fmt.Sprintf("<%s, %s>", kind, formatDuration(m.Call.Duration))
		}
		return fmt.Sprintf("<%s>", kind)
	case m.Call.Outcome == model.CallMissed:
		return fmt.Sprintf("<missed %s>", kind)
	case m.Call.Outcome == model.CallDeclined:
		return fmt.Sprintf("<declined %s>", kind)
	default:
		return fmt.Sprintf("<%s, not answered>", kind)
	}
}

// place describes somewhere a message pointed at.
func (r renderer) place(m model.Message) string {
	label := m.Place.Label()
	what := "location"
	if m.Place.Live {
		what = "live location"
	}
	if m.Place.HasCoordinates() && m.Place.Name != "" {
		return fmt.Sprintf("<%s: %s (%s)>", what, label, m.Place.Coordinates())
	}
	return fmt.Sprintf("<%s: %s>", what, label)
}

// poll states the question. The answers follow as detail lines.
func (r renderer) poll(m model.Message) string {
	question := firstNonEmpty(m.Poll.Question, m.Text, "(no question recorded)")
	return fmt.Sprintf("<poll: %s>", question)
}

// invite describes an invitation to a group.
func (r renderer) invite(m model.Message) string {
	if m.Invite.GroupName != "" {
		return fmt.Sprintf("<invitation to join %q>", m.Invite.GroupName)
	}
	return "<invitation to join a group>"
}

// contacts describes shared contact cards.
func (r renderer) contacts(m model.Message) string {
	names := make([]string, 0, len(m.Contacts))
	for _, c := range m.Contacts {
		if c.Name != "" {
			names = append(names, c.Name)
		}
	}
	if len(names) == 0 {
		return fmt.Sprintf("<%d contact cards omitted>", len(m.Contacts))
	}
	return fmt.Sprintf("<contact card: %s>", strings.Join(names, ", "))
}

// album describes several pictures sent at once.
func (r renderer) album(m model.Message) string {
	if m.HasText() {
		return fmt.Sprintf("<album of %d items> %s", m.AlbumSize, m.Text)
	}
	return fmt.Sprintf("<album of %d items>", m.AlbumSize)
}

// details are the extra lines written beneath a message: what it replied to, how
// people reacted, what a poll offered, what a link led to.
//
// They are separate from the body so that a text export stays parseable: every
// detail line is indented, and only the first line of a message carries a
// timestamp, exactly as a multi-line message does.
func (r renderer) details(m model.Message) []string {
	var out []string

	if m.Quote != nil {
		out = append(out, "> "+r.quote(*m.Quote))
	}
	if m.Poll != nil {
		for _, option := range m.Poll.Options {
			out = append(out, fmt.Sprintf("- %s (%s)", option.Name, plural(option.Votes, "vote", "votes")))
		}
	}
	if m.Link != nil && !m.Link.IsEmpty() {
		out = append(out, "link: "+r.link(*m.Link))
	}
	if m.Place != nil && m.Place.Address != "" && m.Place.Address != m.Place.Name {
		out = append(out, "address: "+m.Place.Address)
	}
	if len(m.Reactions) > 0 {
		out = append(out, "reactions: "+r.reactions(m.Reactions))
	}
	if m.WasForwardedMany() {
		out = append(out, "forwarded many times")
	} else if m.Forwarded {
		out = append(out, "forwarded")
	}
	if m.IsDisappearing() {
		out = append(out, "disappears after "+formatDuration(m.Expires))
	}
	if m.Starred {
		out = append(out, "starred")
	}
	return out
}

// quote renders what a reply was answering.
func (r renderer) quote(q model.Quote) string {
	who := r.name(q.Sender, q.FromMe)
	switch {
	case q.Text != "":
		return who + ": " + oneLine(q.Text, 160)
	case q.Attachment != nil && q.Attachment.FileName != "":
		return fmt.Sprintf("%s: <%s>", who, q.Attachment.FileName)
	default:
		return fmt.Sprintf("%s: <%s>", who, attachmentNoun(q.Kind))
	}
}

// link renders what a shared address led to, which may be the only surviving
// record of it: the page itself may have changed or gone.
func (r renderer) link(l model.LinkPreview) string {
	var parts []string
	if l.Title != "" {
		parts = append(parts, l.Title)
	}
	if l.Description != "" {
		parts = append(parts, oneLine(l.Description, 200))
	}
	if l.URL != "" {
		parts = append(parts, l.URL)
	}
	return strings.Join(parts, " — ")
}

// reactions renders who reacted with what, grouping identical emoji.
func (r renderer) reactions(reactions []model.Reaction) string {
	type group struct {
		emoji string
		who   []string
	}
	var (
		order  []string
		groups = make(map[string]*group)
	)
	for _, reaction := range reactions {
		g, seen := groups[reaction.Emoji]
		if !seen {
			g = &group{emoji: reaction.Emoji}
			groups[reaction.Emoji] = g
			order = append(order, reaction.Emoji)
		}
		g.who = append(g.who, r.name(reaction.Sender, reaction.FromMe))
	}

	parts := make([]string, 0, len(order))
	for _, emoji := range order {
		g := groups[emoji]
		parts = append(parts, fmt.Sprintf("%s %s", g.emoji, strings.Join(g.who, ", ")))
	}
	return strings.Join(parts, "; ")
}

// edited is the marker WhatsApp appends to a message that was changed.
const edited = "<This message was edited>"

// formatDuration renders a length of time the way people say it.
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	d = d.Round(time.Second)
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	switch {
	case hours > 0:
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	case minutes > 0:
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

// plural renders a count with the right form of its noun.
func plural(n int, singular, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// oneLine flattens text onto a single line and shortens it, for the places where
// a quotation would otherwise swallow the page.
func oneLine(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) <= limit {
		return s
	}
	return string([]rune(s)[:limit]) + "…"
}

// firstNonEmpty returns the first value with anything in it.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
