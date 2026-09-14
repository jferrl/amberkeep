package app

import (
	"context"
	"fmt"
	"time"

	"github.com/jferrl/amberkeep/internal/api"
	"github.com/jferrl/amberkeep/internal/export"
	"github.com/jferrl/amberkeep/internal/model"
)

// Writing an archive out, for somebody who did not open a terminal to read it.
//
// The command has done this since the beginning. The page could not, which meant the
// one thing a person is most likely to want after finding their history — a copy of
// it they can keep, print, or send to a solicitor — was reachable only by people who
// did not need the page in the first place.
//
// It is the same work either way. This is the loop `amberkeep export` runs, moved
// somewhere both front doors can reach and told how to report what it is doing,
// because an archive of a million messages takes a minute and silence looks like a
// program that has stopped.

// Export writes the conversations out and reports what it wrote.
//
// `only` names the conversations wanted, by address. Empty means all of them, which
// is the slow case and the reason this reports progress at all.
func (r readable) Export(ctx context.Context, ask api.ExportRequest, say api.Progress) (api.Exported, error) {
	formats, err := formatsFor(ask.Formats)
	if err != nil {
		return api.Exported{}, err
	}

	chats, err := r.Chats(ctx)
	if err != nil {
		return api.Exported{}, err
	}

	wanted := make(map[string]bool, len(ask.Only))
	for _, address := range ask.Only {
		wanted[address] = true
	}

	// A nil zone would render every timestamp in universal time and silently shift
	// a whole archive by hours.
	zone := ask.Location
	if zone == nil {
		zone = time.Local
	}

	opts := export.Options{
		Directory:      ask.Into,
		Names:          r.Directory(),
		Location:       zone,
		Me:             ask.Me,
		IncludeNotices: ask.Notices,
		Words: export.Words{
			Title:       ask.Words.Title,
			Noun:        ask.Words.Noun,
			Placeholder: ask.Words.Placeholder,
		},
		Overwrite:        true,
		NoticeIdentified: NoticeIdentifier(r.Archive),
	}

	// The index only means anything when there are pages for it to link to.
	var indexed bool
	for _, format := range formats {
		if format.name == "html" {
			indexed = true
		}
	}

	var (
		total   export.Result
		entries []export.Entry
		done    int
	)
	for _, chat := range chats {
		if err := ctx.Err(); err != nil {
			return api.Exported{}, err
		}
		if !chat.Includable() {
			continue
		}
		if !ask.Groups && chat.Kind == model.ChatGroup {
			continue
		}
		if len(wanted) > 0 && !wanted[chat.JID.String()] {
			continue
		}

		conv := export.Conversation{
			Chat: chat,
			Messages: func(yield func(model.Message, error) bool) {
				for m, err := range r.Messages(ctx, chat) {
					if !yield(m, err) {
						return
					}
				}
			},
		}

		var written int
		for i, format := range formats {
			result, err := format.write(conv, opts)
			if err != nil {
				return api.Exported{}, fmt.Errorf("writing %s: %w", chat.Title(), err)
			}
			// Every format writes the same messages, so counting them once per
			// format would report several times what the archive actually holds.
			if i == 0 {
				total.Messages += result.Messages
				total.Skipped += result.Skipped
				written = result.Messages
			}
			total.Bytes += result.Bytes
		}
		total.Conversations++
		done++

		if indexed {
			entries = append(entries, export.Entry{
				Chat: chat, File: export.FileName(chat, ".html"), Messages: written,
			})
		}

		// Often enough to prove it is moving, seldom enough not to be the work.
		if done%25 == 0 {
			// "7450 of 10500" is wrong in English as well as Spanish. The numbers go
			// and the page writes the sentence, the way the index build already does.
			say(api.StepWriting,
				fmt.Sprintf("%d of %d conversations written.", done, len(chats)),
				api.Count{Of: "written", N: done},
				api.Count{Of: "conversations", N: len(chats)},
			)
		}
	}

	if indexed && len(entries) > 0 {
		say(api.StepWriting, "Writing the index page.")
		result, err := export.WriteIndex(entries, opts)
		if err != nil {
			return api.Exported{}, fmt.Errorf("writing the index: %w", err)
		}
		total.Bytes += result.Bytes
	}

	return api.Exported{
		Into:          ask.Into,
		Conversations: total.Conversations,
		Messages:      total.Messages,
		Bytes:         total.Bytes,
		Formats:       ask.Formats,
	}, nil
}

// formatsFor turns the names a page asked for into the writers that produce them.
func formatsFor(names []string) ([]format, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("say which formats to write: html, text or json")
	}
	var out []format
	for _, name := range names {
		switch name {
		case "html":
			out = append(out, format{"html", export.WriteHTML})
		case "text":
			out = append(out, format{"text", export.WriteText})
		case "json":
			out = append(out, format{"json", export.WriteJSON})
		default:
			return nil, fmt.Errorf("%q is not a format this writes: html, text or json", name)
		}
	}
	return out, nil
}

// format is one way of writing a conversation out.
type format struct {
	name  string
	write func(export.Conversation, export.Options) (export.Result, error)
}
