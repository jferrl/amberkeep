package export

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/jferrl/amberkeep/internal/model"
)

// Plain text is the format that will still open in fifty years.
//
// The shape is WhatsApp's own, so the result is familiar and the tools people
// already use to read exported chats can read this too:
//
//	[12/09/2026, 20:46:09] Ana Lopez: hello
//	[12/09/2026, 20:47:00] You: <image omitted> at the beach
//	    > Ana Lopez: hello
//	    reactions: 👍 Ana Lopez
//
// Two deliberate departures from what WhatsApp writes. The date carries a
// four-digit year so an archive read in another country still says what it means.
// And none of the invisible direction marks WhatsApp scatters through its exports
// appear here, because they break every parser that meets them and help nobody.
//
// Only the first line of a message carries a timestamp. Continuation lines and
// detail lines are indented, exactly as a multi-line message is, so a parser that
// splits on timestamps sees one message.

// textIndent prefixes every line that is not the start of a message.
const textIndent = "    "

// WriteText writes one conversation as plain text and reports the file written.
func WriteText(conv Conversation, opts Options) (Result, error) {
	opts = opts.withDefaults()
	path := filepath.Join(opts.Directory, FileName(conv.Chat, ".txt"))
	r := newRenderer(opts.Names, opts)

	var result Result
	bytes, err := atomicWrite(path, opts.Overwrite, func(w io.Writer) error {
		out := bufio.NewWriter(w)
		writeTextHeader(out, conv.Chat, r)

		for m, err := range conv.Messages {
			if err != nil {
				return fmt.Errorf("reading %s: %w", conv.Chat.Title(), err)
			}
			if skip(m, opts) {
				result.Skipped++
				continue
			}
			writeTextMessage(out, m, r)
			result.Messages++
		}
		return out.Flush()
	})
	if err != nil {
		return Result{}, err
	}

	result.Conversations = 1
	result.Files = []string{path}
	result.Bytes = bytes
	return result, nil
}

// skip reports whether a message is left out of an export.
func skip(m model.Message, opts Options) bool {
	if !m.Displayable() {
		return true
	}
	return m.Kind.IsNotice() && !opts.IncludeNotices
}

// writeTextHeader introduces the conversation, so a file found on its own years
// later still says what it is.
func writeTextHeader(w *bufio.Writer, chat model.Chat, r renderer) {
	fmt.Fprintf(w, "%s\n", chat.Title())
	fmt.Fprintf(w, "%s conversation with %s\n", strings.ToUpper(chat.Kind.String()[:1])+chat.Kind.String()[1:], chat.JID)

	if len(chat.Participants) > 0 {
		names := make([]string, 0, len(chat.Participants))
		for _, p := range chat.Participants {
			names = append(names, p.Name)
		}
		fmt.Fprintf(w, "Members: %s\n", strings.Join(names, ", "))
	}
	if !chat.LastAt.IsZero() {
		fmt.Fprintf(w, "Last message: %s\n", r.timestamp(chat.LastAt))
	}
	fmt.Fprintf(w, "Messages: %d\n", chat.Messages)
	fmt.Fprintf(w, "%s\n\n", strings.Repeat("-", 60))
}

// writeTextMessage writes one message and whatever came with it.
func writeTextMessage(w *bufio.Writer, m model.Message, r renderer) {
	body := r.body(m)
	if m.WasEdited() {
		body += " " + edited
	}

	// A notice is not something a person said, so it carries no name.
	prefix := r.timestamp(m.SentAt) + " "
	if m.Kind != model.KindSystem {
		prefix += r.sender(m) + ": "
	}

	lines := strings.Split(body, "\n")
	fmt.Fprintf(w, "%s%s\n", prefix, lines[0])
	for _, line := range lines[1:] {
		fmt.Fprintf(w, "%s%s\n", textIndent, line)
	}
	for _, detail := range r.details(m) {
		fmt.Fprintf(w, "%s%s\n", textIndent, detail)
	}
}
