package export

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

// Structured data, for anything that wants to read an archive by machine.
//
// Two commitments make this format worth building on. Nothing is lost: every
// piece of content the reader recovered appears here, including the small copies
// of pictures whose files are gone. And the shape is stable: fields may be added,
// but a field that exists keeps its name and meaning, because somebody's tooling
// will depend on it.
//
// The file is written a message at a time rather than assembled in memory, so a
// conversation of any size costs the same.

// jsonVersion is the shape of the file. It is written into every export so a
// reader can tell what it is looking at, and it changes only if a field's meaning
// ever changes, which it should not.
const jsonVersion = 1

// WriteJSON writes one conversation as structured data.
func WriteJSON(conv Conversation, opts Options) (Result, error) {
	opts = opts.withDefaults()
	path := filepath.Join(opts.Directory, FileName(conv.Chat, ".json"))
	r := newRenderer(opts.Names, opts)

	var result Result
	written, err := atomicWrite(path, opts.Overwrite, func(w io.Writer) error {
		out := bufio.NewWriterSize(w, writeBuffer)

		// Each value is encoded into a buffer first so the trailing newline that
		// Encode always appends can be dropped, which a list cannot carry. Encoding
		// straight to the output would leave the file invalid.
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		// Text is written as it is, so a message containing an angle bracket or an
		// ampersand is not mangled into escapes nobody asked for.
		encoder.SetEscapeHTML(false)
		// An archive is meant to be opened years from now, possibly by somebody with
		// no tooling at all, so it is laid out to be read.
		encoder.SetIndent("  ", "  ")

		write := func(v any) error {
			buf.Reset()
			if err := encoder.Encode(v); err != nil {
				return err
			}
			_, err := out.Write(bytes.TrimRight(buf.Bytes(), "\n"))
			return err
		}

		fmt.Fprintf(out, "{\n  \"version\": %d,\n  \"exported\": %q,\n  \"chat\": ",
			jsonVersion, time.Now().UTC().Format(time.RFC3339))
		if err := write(jsonChat(conv.Chat, r)); err != nil {
			return fmt.Errorf("writing the conversation: %w", err)
		}

		out.WriteString(",\n  \"messages\": [")
		first := true
		for m, err := range conv.Messages {
			if err != nil {
				return fmt.Errorf("reading %s: %w", conv.Chat.Title(), err)
			}
			if skip(m, opts) {
				result.Skipped++
				continue
			}
			if !first {
				out.WriteString(",")
			}
			first = false
			out.WriteString("\n  ")
			if err := write(jsonMessage(m, r, opts)); err != nil {
				return fmt.Errorf("writing a message: %w", err)
			}
			result.Messages++
		}

		if !first {
			out.WriteString("\n  ")
		}
		out.WriteString("]\n}\n")
		return out.Flush()
	})
	if err != nil {
		return Result{}, err
	}

	result.Conversations = 1
	result.Files = []string{path}
	result.Bytes = written
	result.Carried, result.CarriedBytes = r.files.carried()
	return result, nil
}

// The shapes below are the file format. Every field is omitted when empty, so an
// ordinary text message stays small, and a reader can tell "absent" from "empty"
// without guessing.

type chatJSON struct {
	ID           int64        `json:"id"`
	Address      string       `json:"address"`
	Kind         string       `json:"kind"`
	Name         string       `json:"name"`
	Participants []personJSON `json:"participants,omitempty"`
	Description  string       `json:"description,omitempty"`
	CreatedAt    *time.Time   `json:"created_at,omitempty"`
	LastAt       *time.Time   `json:"last_message_at,omitempty"`
	Archived     bool         `json:"archived,omitempty"`
	Messages     int          `json:"message_count"`
}

type personJSON struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
	Admin   bool   `json:"admin,omitempty"`
}

type messageJSON struct {
	ID         int64     `json:"id"`
	Key        string    `json:"key,omitempty"`
	SentAt     time.Time `json:"sent_at"`
	FromMe     bool      `json:"from_me"`
	Sender     string    `json:"sender,omitempty"`
	SenderName string    `json:"sender_name,omitempty"`
	Kind       string    `json:"kind"`
	Text       string    `json:"text,omitempty"`
	// Rendered is the message as the text export would show it, so a consumer that
	// only wants something readable does not have to reimplement the wording.
	Rendered string `json:"rendered"`

	Attachment *attachmentJSON `json:"attachment,omitempty"`
	Quote      *quoteJSON      `json:"reply_to,omitempty"`
	Reactions  []reactionJSON  `json:"reactions,omitempty"`
	Mentions   []string        `json:"mentions,omitempty"`
	Place      *placeJSON      `json:"place,omitempty"`
	Poll       *pollJSON       `json:"poll,omitempty"`
	Call       *callJSON       `json:"call,omitempty"`
	Link       *linkJSON       `json:"link,omitempty"`
	Contacts   []cardJSON      `json:"contact_cards,omitempty"`
	Invite     *inviteJSON     `json:"group_invite,omitempty"`
	Notice     *noticeJSON     `json:"notice,omitempty"`
	Deleted    *deletionJSON   `json:"deleted,omitempty"`

	AlbumSize    int        `json:"album_size,omitempty"`
	ExpiresAfter string     `json:"expires_after,omitempty"`
	Starred      bool       `json:"starred,omitempty"`
	Forwarded    bool       `json:"forwarded,omitempty"`
	ForwardScore int        `json:"forward_score,omitempty"`
	EditedAt     *time.Time `json:"edited_at,omitempty"`

	// SourceType is the number the source database used. It is kept so an
	// unrecognised message can be investigated rather than merely noticed.
	SourceType int `json:"source_type"`
}

type attachmentJSON struct {
	MediaType string `json:"media_type,omitempty"`
	FileName  string `json:"file_name,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Duration  string `json:"duration,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Caption   string `json:"caption,omitempty"`
	// Preview is the small copy of the picture that survived inside the message
	// database, encoded so it can travel inside a text file. For an old archive it
	// is often the only image of what was sent.
	Preview string `json:"preview_base64,omitempty"`
	// File is where the file itself is, when the archive was given the folder
	// holding it: the path the phone recorded. Absent for the great majority of
	// attachments, which is what an archive read without its files looks like.
	File string `json:"file,omitempty"`
}

type quoteJSON struct {
	Sender     string          `json:"sender,omitempty"`
	SenderName string          `json:"sender_name,omitempty"`
	FromMe     bool            `json:"from_me,omitempty"`
	Kind       string          `json:"kind"`
	Text       string          `json:"text,omitempty"`
	Attachment *attachmentJSON `json:"attachment,omitempty"`
}

type reactionJSON struct {
	Emoji      string     `json:"emoji"`
	Sender     string     `json:"sender,omitempty"`
	SenderName string     `json:"sender_name,omitempty"`
	FromMe     bool       `json:"from_me,omitempty"`
	At         *time.Time `json:"at,omitempty"`
}

type placeJSON struct {
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
	Name      string  `json:"name,omitempty"`
	Address   string  `json:"address,omitempty"`
	URL       string  `json:"url,omitempty"`
	Live      bool    `json:"live,omitempty"`
}

type pollJSON struct {
	Question string           `json:"question"`
	Options  []pollOptionJSON `json:"options"`
	Closed   bool             `json:"closed,omitempty"`
}

type pollOptionJSON struct {
	Name  string `json:"name"`
	Votes int    `json:"votes"`
}

type callJSON struct {
	Video    bool   `json:"video"`
	Group    bool   `json:"group,omitempty"`
	Outcome  string `json:"outcome"`
	Duration string `json:"duration,omitempty"`
}

type linkJSON struct {
	URL         string `json:"url,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type cardJSON struct {
	Name      string   `json:"name,omitempty"`
	VCard     string   `json:"vcard"`
	Addresses []string `json:"addresses,omitempty"`
}

type inviteJSON struct {
	GroupName string     `json:"group_name,omitempty"`
	Address   string     `json:"address,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type noticeJSON struct {
	Action  int      `json:"action"`
	Text    string   `json:"text"`
	Actor   string   `json:"actor,omitempty"`
	Targets []string `json:"targets,omitempty"`
	Old     string   `json:"old_value,omitempty"`
	New     string   `json:"new_value,omitempty"`
	// Identified says whether this build knows what the action code means. A false
	// here is an honest admission rather than a silent guess.
	Identified bool `json:"identified"`
}

type deletionJSON struct {
	At      *time.Time `json:"at,omitempty"`
	By      string     `json:"by,omitempty"`
	ByAdmin bool       `json:"by_admin,omitempty"`
}

// MessageValue renders one message in the same structured form the JSON export
// writes, for anything that serves an archive rather than writing it.
//
// It returns an opaque value on purpose. The contract is the shape of the JSON,
// which is promised to stay stable, not a Go type somebody could depend on the
// fields of. Sharing it is what keeps a file and an HTTP response from drifting
// apart and quietly disagreeing about what an archive contains.
func MessageValue(m model.Message, opts Options) any {
	opts = opts.withDefaults()
	return jsonMessage(m, newRenderer(opts.Names, opts), opts)
}

// ChatValue renders one conversation in that same form.
func ChatValue(c model.Chat, opts Options) any {
	opts = opts.withDefaults()
	return jsonChat(c, newRenderer(opts.Names, opts))
}

// jsonChat converts a conversation.
func jsonChat(c model.Chat, _ renderer) chatJSON {
	out := chatJSON{
		ID:          c.ID,
		Address:     c.JID.String(),
		Kind:        c.Kind.String(),
		Name:        c.Title(),
		Description: c.Description,
		Archived:    c.Archived,
		Messages:    c.Messages,
		CreatedAt:   optionalTime(c.CreatedAt),
		LastAt:      optionalTime(c.LastAt),
	}
	for _, p := range c.Participants {
		out.Participants = append(out.Participants, personJSON{
			Address: p.JID.String(), Name: p.Name, Admin: p.Admin,
		})
	}
	return out
}

// jsonMessage converts a message, keeping everything the reader recovered.
func jsonMessage(m model.Message, r renderer, opts Options) messageJSON {
	out := messageJSON{
		ID:           m.ID,
		Key:          m.Key,
		SentAt:       m.SentAt.UTC(),
		FromMe:       m.IsFromMe(),
		Kind:         m.Kind.String(),
		Text:         m.Text,
		Rendered:     r.body(m),
		AlbumSize:    m.AlbumSize,
		Starred:      m.Starred,
		Forwarded:    m.Forwarded,
		ForwardScore: m.ForwardScore,
		EditedAt:     optionalTime(m.EditedAt),
		SourceType:   m.SourceType,
	}
	if !m.IsFromMe() && !m.Sender.IsZero() {
		out.Sender = m.Sender.String()
		out.SenderName = r.sender(m)
	}
	if m.Expires > 0 {
		out.ExpiresAfter = formatDuration(m.Expires)
	}
	for _, j := range m.Mentions {
		out.Mentions = append(out.Mentions, j.String())
	}

	out.Attachment = r.jsonAttachment(m.Attachment)
	if m.Quote != nil {
		out.Quote = &quoteJSON{
			FromMe:     m.Quote.FromMe,
			Kind:       m.Quote.Kind.String(),
			Text:       m.Quote.Text,
			Attachment: r.jsonAttachment(m.Quote.Attachment),
		}
		if !m.Quote.FromMe && !m.Quote.Sender.IsZero() {
			out.Quote.Sender = m.Quote.Sender.String()
			out.Quote.SenderName = r.name(m.Quote.Sender, false)
		}
	}
	for _, reaction := range m.Reactions {
		entry := reactionJSON{Emoji: reaction.Emoji, FromMe: reaction.FromMe, At: optionalTime(reaction.At)}
		if !reaction.FromMe && !reaction.Sender.IsZero() {
			entry.Sender = reaction.Sender.String()
			entry.SenderName = r.name(reaction.Sender, false)
		}
		out.Reactions = append(out.Reactions, entry)
	}
	if m.Place != nil {
		out.Place = &placeJSON{
			Latitude: m.Place.Latitude, Longitude: m.Place.Longitude,
			Name: m.Place.Name, Address: m.Place.Address, URL: m.Place.URL, Live: m.Place.Live,
		}
	}
	if m.Poll != nil {
		poll := &pollJSON{Question: m.Poll.Question, Closed: m.Poll.Closed}
		for _, option := range m.Poll.Options {
			poll.Options = append(poll.Options, pollOptionJSON{Name: option.Name, Votes: option.Votes})
		}
		out.Poll = poll
	}
	if m.Call != nil {
		call := &callJSON{Video: m.Call.Video, Group: m.Call.Group, Outcome: m.Call.Outcome.String()}
		if m.Call.Duration > 0 {
			call.Duration = formatDuration(m.Call.Duration)
		}
		out.Call = call
	}
	if m.Link != nil {
		out.Link = &linkJSON{URL: m.Link.URL, Title: m.Link.Title, Description: m.Link.Description}
	}
	for _, c := range m.Contacts {
		card := cardJSON{Name: c.Name, VCard: c.VCard}
		for _, j := range c.JIDs {
			card.Addresses = append(card.Addresses, j.String())
		}
		out.Contacts = append(out.Contacts, card)
	}
	if m.Invite != nil {
		out.Invite = &inviteJSON{
			GroupName: m.Invite.GroupName,
			Address:   m.Invite.Group.String(),
			ExpiresAt: optionalTime(m.Invite.ExpiresAt),
		}
	}
	if m.Notice != nil {
		notice := &noticeJSON{
			Action: m.Notice.Action, Text: m.SystemText,
			Old: m.Notice.Old, New: m.Notice.New,
			Identified: opts.NoticeIdentified(m.Notice.Action),
		}
		if !m.Notice.Actor.IsZero() {
			notice.Actor = m.Notice.Actor.String()
		}
		for _, j := range m.Notice.Targets {
			notice.Targets = append(notice.Targets, j.String())
		}
		out.Notice = notice
	}
	if m.Deleted != nil {
		deletion := &deletionJSON{At: optionalTime(m.Deleted.At), ByAdmin: m.Deleted.ByAdmin()}
		if m.Deleted.ByAdmin() {
			deletion.By = m.Deleted.By.String()
		}
		out.Deleted = deletion
	}
	return out
}

// jsonAttachment converts a file description, carrying the surviving preview — and
// the file itself, when the archive has it, copied out under the very path recorded
// here. So `file` says both what the phone called it and, read from beside this
// archive, where it now is.
func (r renderer) jsonAttachment(a *model.Attachment) *attachmentJSON {
	if a == nil {
		return nil
	}
	out := &attachmentJSON{
		MediaType: a.MediaType, FileName: a.FileName, Size: a.Size,
		Width: a.Width, Height: a.Height, Caption: a.Caption,
	}
	if a.Duration > 0 {
		out.Duration = formatDuration(a.Duration)
	}
	if a.HasPreview() {
		out.Preview = base64.StdEncoding.EncodeToString(a.Preview.Data)
	}
	// Recorded whether or not the file travels: it is what the phone called it and
	// where it was, which is worth keeping in a structured archive on its own. When
	// the files did travel, this same path is where the file now sits beside this
	// file, which is why it is not rewritten into something else.
	out.File = a.File
	// The copy itself, for an export of structured data with no page to look at.
	_, _ = r.files.carry(a.File)
	return out
}

// optionalTime omits a time that was never recorded, so a consumer can tell the
// difference between the beginning of 1970 and no answer.
func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	utc := t.UTC()
	return &utc
}
