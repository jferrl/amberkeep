package export

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"html/template"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "embed"

	"github.com/jferrl/amberkeep/internal/model"
)

// A conversation as a web page that works with the network off.
//
// This is the format that shows what the reader actually recovered. A photograph
// whose file was deleted years ago still has a small copy inside the message
// database, and here it appears as a picture rather than as the words "preview
// recovered". The stylesheet, the script and every one of those small copies are
// part of the file.
//
// When the archive has the phone's own folder, the photographs, videos and
// recordings are copied out beside the pages and linked to, because a copy of
// somebody's history with the pictures taken out is not a copy of it. What is
// self-contained is then the folder rather than each page alone: nothing is ever
// fetched from anywhere, so it opens the same on a laptop with no internet, from a
// USB stick, or in ten years.
//
// The page is written a message at a time, like the other formats. The one thing
// that cannot stream is the browser's own memory, and a conversation here reaches
// ninety thousand messages, so the layout leans on content-visibility to let the
// browser skip what is off screen, and the search index is built in the page on
// first use rather than written into the file.

//go:embed assets/page.css
var pageCSS string

//go:embed assets/page.js
var pageJS string

// The stylesheet and the script as the templates need them. Both are files in
// this repository, compiled into the binary, and nothing a message contains can
// reach them.
//
// #nosec G203 -- the content is a compile-time constant of this program.
var (
	styles = template.CSS(pageCSS)
	script = template.JS(pageJS)
)

// WriteHTML writes one conversation as a self-contained web page.
func WriteHTML(conv Conversation, opts Options) (Result, error) {
	opts = opts.withDefaults()
	path := filepath.Join(opts.Directory, FileName(conv.Chat, ".html"))
	r := newRenderer(opts.Names, opts)

	var result Result
	written, err := atomicWrite(path, opts.Overwrite, func(w io.Writer) error {
		out := bufio.NewWriterSize(w, 1<<16)

		if err := pages.ExecuteTemplate(out, "head", headOf(conv.Chat, r)); err != nil {
			return err
		}

		var day string
		for m, err := range conv.Messages {
			if err != nil {
				return fmt.Errorf("reading %s: %w", conv.Chat.Title(), err)
			}
			if skip(m, opts) {
				result.Skipped++
				continue
			}
			if on := r.day(m.SentAt); on != day {
				day = on
				if err := pages.ExecuteTemplate(out, "day", on); err != nil {
					return err
				}
			}
			if err := pages.ExecuteTemplate(out, "message", r.htmlMessage(m)); err != nil {
				return err
			}
			result.Messages++
		}

		if err := pages.ExecuteTemplate(out, "tail", tailOf(result, opts)); err != nil {
			return err
		}
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

// head is what introduces a page: enough that a file found on its own years later
// still says what it is and who is in it.
type head struct {
	Title       string
	Subtitle    string
	Noun        string
	Placeholder string
	CSS         template.CSS
}

func headOf(chat model.Chat, r renderer) head {
	var facts []string
	facts = append(facts, strings.ToUpper(chat.Kind.String()[:1])+chat.Kind.String()[1:]+" conversation")
	if len(chat.Participants) > 0 {
		facts = append(facts, plural(len(chat.Participants), "member", "members"))
	}
	if chat.Messages > 0 {
		facts = append(facts, plural(chat.Messages, "message", "messages"))
	}
	if !chat.LastAt.IsZero() {
		facts = append(facts, "last on "+chat.LastAt.In(r.loc).Format("2 January 2006"))
	}
	return head{
		Title:       chat.Title(),
		Subtitle:    strings.Join(facts, " · "),
		Noun:        "messages",
		Placeholder: "Search this conversation",
		CSS:         styles,
	}
}

// tail closes a page with what it actually holds and where it came from.
type tail struct {
	Summary string
	JS      template.JS
}

func tailOf(result Result, opts Options) tail {
	parts := []string{plural(result.Messages, "message", "messages")}
	if result.Skipped > 0 && !opts.IncludeNotices {
		parts = append(parts, plural(result.Skipped, "notice", "notices")+" left out")
	}
	return tail{
		Summary: strings.Join(parts, ", "),
		JS:      script,
	}
}

// htmlOption is one answer a poll offered.
type htmlOption struct {
	Name  string
	Votes string
}

// htmlQuote is the message a reply pointed at.
type htmlQuote struct {
	Who     string
	Text    string
	Preview template.URL
}

// htmlCard is a link preview: what a page said at the moment it was shared, which
// is often the only surviving record of it.
type htmlCard struct {
	Title string
	Text  string
	URL   string
}

// htmlMessage is one message prepared for the page. Every display decision is made
// here rather than in the template, so the template stays something a person can
// read and the escaping stays the template's job.
type htmlMessage struct {
	Mine   bool
	Notice bool
	Who    string
	When   string
	Full   string // the complete timestamp, shown on hover

	Quote *htmlQuote

	Preview    template.URL
	PreviewAlt string

	// Source is the archive's own file, when it travelled with the export: the
	// photograph rather than the thumbnail of it. Shown instead of the preview,
	// because they are the same picture and one of them is legible.
	Source template.URL
	// Playing says which element to write: "image", "video", "audio", or empty for
	// a file a page can only name.
	Playing string

	// Note describes what the message was when it was not words: a file that is
	// not in the archive, a call, a place, a message that was deleted.
	Note string
	// Body is what somebody actually typed, with any addresses made clickable.
	Body template.HTML

	Question  string
	Options   []htmlOption
	Card      *htmlCard
	Reactions []string
	Tags      []string
}

// htmlMessage prepares one message for the page.
func (r renderer) htmlMessage(m model.Message) htmlMessage {
	out := htmlMessage{
		Mine:   m.IsFromMe(),
		Notice: m.Kind == model.KindSystem,
		Who:    r.sender(m),
		When:   r.clock(m.SentAt),
		Full:   r.timestamp(m.SentAt),
		Note:   r.describe(m),
	}

	if out.Notice {
		out.Body = linkify(firstNonEmpty(m.SystemText, m.Text, "System notice"))
		return out
	}

	switch {
	case m.Poll != nil:
		out.Question = firstNonEmpty(m.Poll.Question, m.Text, "(no question recorded)")
		for _, option := range m.Poll.Options {
			out.Options = append(out.Options, htmlOption{
				Name:  option.Name,
				Votes: plural(option.Votes, "vote", "votes"),
			})
		}
	case m.HasText() && !m.WasDeleted():
		out.Body = linkify(m.Text)
	}

	if m.Attachment != nil {
		// The file itself when it is here, the thumbnail when it is not. Both when
		// the file is here and is not something a page can show — a document keeps
		// whatever preview survived beside the link to it.
		if file, ok := r.files.carry(m.Attachment.File); ok {
			out.Source = template.URL(file.Link) // #nosec G203 -- a path escaped by linkTo
			out.Playing = shows(file.Kind)
			out.PreviewAlt = attachmentNoun(m.Kind) + " sent in this message"
		}
		if m.Attachment.HasPreview() && out.Playing == "" {
			out.Preview = dataURI(m.Attachment.Preview)
			if out.Source == "" {
				out.PreviewAlt = "recovered preview of " + attachmentNoun(m.Kind)
			}
		}
	}

	if m.Quote != nil {
		quote := &htmlQuote{Who: r.name(m.Quote.Sender, m.Quote.FromMe), Text: r.quoteText(*m.Quote)}
		if m.Quote.Attachment != nil && m.Quote.Attachment.HasPreview() {
			quote.Preview = dataURI(m.Quote.Attachment.Preview)
		}
		out.Quote = quote
	}

	if m.Link != nil && !m.Link.IsEmpty() {
		out.Card = &htmlCard{
			Title: m.Link.Title,
			Text:  oneLine(m.Link.Description, 240),
			URL:   m.Link.URL,
		}
	}

	out.Reactions = r.reactionChips(m.Reactions)
	out.Tags = r.htmlTags(m)
	return out
}

// describe is the part of a message that is not the words somebody typed: the
// file that is not here, the call, the place, the fact that it was deleted.
//
// It is empty for a plain message, and for the kinds the page draws itself.
func (r renderer) describe(m model.Message) string {
	switch {
	case m.WasDeleted():
		return r.deleted(m)
	case m.Kind == model.KindSystem, m.Poll != nil:
		return ""
	case m.Call != nil:
		return r.call(m)
	case m.Place != nil:
		return r.place(m)
	case m.Invite != nil:
		return r.invite(m)
	case len(m.Contacts) > 0:
		return r.contacts(m)
	case m.Kind == model.KindAlbum:
		return fmt.Sprintf("<album of %d items>", m.AlbumSize)
	case m.Kind.HasAttachment() || m.Attachment != nil:
		return r.attachmentNote(m)
	case m.HasText():
		return ""
	case m.Kind == model.KindUnknown:
		return fmt.Sprintf("<unrecognised message, type %d>", m.SourceType)
	default:
		return "<" + m.Kind.String() + ">"
	}
}

// quoteText is what a reply was answering, without the sender's name, which the
// page shows separately.
func (r renderer) quoteText(q model.Quote) string {
	switch {
	case q.Text != "":
		return oneLine(q.Text, 200)
	case q.Attachment != nil && q.Attachment.FileName != "":
		return "<" + q.Attachment.FileName + ">"
	default:
		return "<" + attachmentNoun(q.Kind) + ">"
	}
}

// reactionChips renders each emoji with the people who chose it.
func (r renderer) reactionChips(reactions []model.Reaction) []string {
	if len(reactions) == 0 {
		return nil
	}
	var (
		order []string
		who   = make(map[string][]string)
	)
	for _, reaction := range reactions {
		if _, seen := who[reaction.Emoji]; !seen {
			order = append(order, reaction.Emoji)
		}
		who[reaction.Emoji] = append(who[reaction.Emoji], r.name(reaction.Sender, reaction.FromMe))
	}
	chips := make([]string, 0, len(order))
	for _, emoji := range order {
		chips = append(chips, emoji+" "+strings.Join(who[emoji], ", "))
	}
	return chips
}

// htmlTags are the small facts shown beneath a message.
func (r renderer) htmlTags(m model.Message) []string {
	var tags []string
	if m.WasEdited() {
		tags = append(tags, "edited")
	}
	if m.WasForwardedMany() {
		tags = append(tags, "forwarded many times")
	} else if m.Forwarded {
		tags = append(tags, "forwarded")
	}
	if m.IsDisappearing() {
		tags = append(tags, "disappears after "+formatDuration(m.Expires))
	}
	if m.Starred {
		tags = append(tags, "starred")
	}
	if m.Place != nil && m.Place.Address != "" && m.Place.Address != m.Place.Name {
		tags = append(tags, m.Place.Address)
	}
	return tags
}

// clock is the time of day a message was sent. The date is on the separator above
// it, so repeating it on every message only adds noise.
func (r renderer) clock(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(r.loc).Format("15:04")
}

// day is the separator a message belongs under.
func (r renderer) day(t time.Time) string {
	if t.IsZero() {
		return "Unknown date"
	}
	return t.In(r.loc).Format("Monday, 2 January 2006")
}

// linkPattern finds the addresses in a message. Only http and https are made
// clickable: those are what people share, and restricting the scheme means no
// message can turn itself into a link that runs something.
var linkPattern = regexp.MustCompile(`(?i)\bhttps?://[^\s<>"']+`)

// trailing is the punctuation that ends a sentence rather than an address.
const trailing = `.,;:!?)]}'"`

// linkify escapes a message and makes the addresses in it clickable.
//
// Every part of the result is escaped here, including the address inside the
// attribute, so the only markup in the output is the anchor this function wrote.
func linkify(s string) template.HTML {
	var b strings.Builder
	b.Grow(len(s) + 32)

	last := 0
	for _, at := range linkPattern.FindAllStringIndex(s, -1) {
		b.WriteString(html.EscapeString(s[last:at[0]]))

		url := strings.TrimRight(s[at[0]:at[1]], trailing)
		escaped := html.EscapeString(url)
		b.WriteString(`<a href="`)
		b.WriteString(escaped)
		b.WriteString(`" rel="noreferrer noopener">`)
		b.WriteString(escaped)
		b.WriteString(`</a>`)
		// Whatever was trimmed is punctuation belonging to the sentence.
		b.WriteString(html.EscapeString(s[at[0]+len(url) : at[1]]))
		last = at[1]
	}
	b.WriteString(html.EscapeString(s[last:]))

	// #nosec G203 -- every segment above is escaped; the only markup is this
	// function's own anchor, whose href is restricted to http and https.
	return template.HTML(b.String())
}

// dataURI turns a recovered preview into something a page can show without
// reaching for a file that is not there.
func dataURI(t model.Thumbnail) template.URL {
	if t.IsEmpty() {
		return ""
	}
	// #nosec G203 -- the scheme is fixed and the payload is this program's own
	// base64 of the bytes, so there is nothing here a message could steer.
	return template.URL("data:" + imageType(t.Data) + ";base64," + base64.StdEncoding.EncodeToString(t.Data))
}

// imageType identifies a preview from its first bytes. WhatsApp stores these as
// JPEG almost without exception, but an archive that mislabels one shows a broken
// picture where a recovered one should be, so they are checked.
func imageType(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G'}):
		return "image/png"
	case bytes.HasPrefix(data, []byte("GIF8")):
		return "image/gif"
	case len(data) >= 12 && bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

// pages holds every page template: the conversation and the index share a head,
// a search box and a footer, so an archive looks and behaves like one thing. They
// are parsed once, because a large archive renders millions of messages through
// them.
var pages = template.Must(template.New("pages").Parse(pageTemplates))

const pageTemplates = `
{{define "head"}}<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>{{.Title}}</title>
<style>{{.CSS}}</style>
</head>
<body>
<header class="top"><div class="inner">
<h1>{{.Title}}</h1>
<p class="about">{{.Subtitle}}</p>
<div class="find">
<input type="search" spellcheck="false" autocomplete="off" data-noun="{{.Noun}}"
       placeholder="{{.Placeholder}}" aria-label="{{.Placeholder}}">
<button type="button" class="clear">Clear</button>
<span class="count" aria-live="polite"></span>
</div>
</div></header>
<main>
{{end}}

{{define "day"}}<div class="day"><span>{{.}}</span></div>
{{end}}

{{define "message"}}<article class="msg{{if .Mine}} mine{{end}}{{if .Notice}} notice{{end}}" data-find>
<div class="bubble">
{{- if not .Notice}}<div><span class="who">{{.Who}}</span><span class="when" title="{{.Full}}">{{.When}}</span></div>{{end}}
{{- with .Quote}}<blockquote>{{if .Preview}}<img class="preview" loading="lazy" src="{{.Preview}}" alt="">{{end}}<span class="who">{{.Who}}</span><div class="body">{{.Text}}</div></blockquote>{{end}}
{{- if .Source}}{{if eq .Playing "image"}}<img class="shot" loading="lazy" src="{{.Source}}" alt="{{.PreviewAlt}}">{{else if eq .Playing "video"}}<video class="shot" controls preload="metadata" src="{{.Source}}"></video>{{else if eq .Playing "audio"}}<audio controls preload="metadata" src="{{.Source}}"></audio>{{else}}<div class="note"><a href="{{.Source}}">{{.PreviewAlt}}</a></div>{{end}}{{end}}
{{- if .Preview}}<img class="preview" loading="lazy" src="{{.Preview}}" alt="{{.PreviewAlt}}">{{if not .Source}}<div class="recovered">recovered preview &mdash; the file itself is not in this archive</div>{{end}}{{end}}
{{- with .Note}}<div class="note">{{.}}</div>{{end}}
{{- with .Question}}<div class="body">{{.}}</div>{{end}}
{{- if .Options}}<div class="poll">{{range .Options}}<div class="opt"><span>{{.Name}}</span><span class="votes">{{.Votes}}</span></div>{{end}}</div>{{end}}
{{- if .Body}}<div class="body">{{.Body}}</div>{{end}}
{{- with .Card}}<div class="card">{{with .Title}}<div class="title">{{.}}</div>{{end}}{{with .Text}}<div>{{.}}</div>{{end}}{{with .URL}}<div class="url">{{.}}</div>{{end}}</div>{{end}}
{{- if .Reactions}}<div class="rx">{{range .Reactions}}<span>{{.}}</span>{{end}}</div>{{end}}
{{- if .Tags}}<div class="tags">{{range $i, $t := .Tags}}{{if $i}} &middot; {{end}}{{$t}}{{end}}</div>{{end}}
</div></article>
{{end}}

{{define "index"}}{{template "head" .Head}}
<ul class="chats">
{{range .Rows}}<li data-find><a href="{{.File}}"><span class="name">{{.Name}}{{with .About}}<span class="sub"><br>{{.}}</span>{{end}}</span><span class="n">{{.Count}}</span></a></li>
{{end}}</ul>
{{template "tail" .Tail}}{{end}}

{{define "tail"}}</main>
<dialog class="zoom"><img alt=""></dialog>
<footer class="foot"><div style="max-width:46rem;margin:0 auto;padding:0 1rem">
<p>{{.Summary}}. This page needs no internet connection: everything it shows is inside the file.</p>
<p>Exported with Amberkeep. Not affiliated with or endorsed by WhatsApp LLC or Meta Platforms, Inc.</p>
</div></footer>
<script>{{.JS}}</script>
</body>
</html>
{{end}}
`
