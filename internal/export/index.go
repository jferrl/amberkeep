package export

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

// The front door of an archive.
//
// Several thousand files in a folder is a pile, not an archive. This is the page
// that turns it into one: every conversation listed with who it was with, how many
// messages it holds and when it last had one, searchable by name, each linking to
// its own page. It is the first thing somebody opens and the last thing written,
// because only then is it known what the archive actually contains.

// Entry is one conversation as the index lists it.
type Entry struct {
	Chat model.Chat
	// File is the conversation's page, as a path relative to the index.
	File string
	// Messages is how many were actually written, which can differ from what the
	// source held once notices are left out.
	Messages int
}

// IndexName is what the front page is called. Browsers open it by default when
// given the folder, which is the whole point of the name.
const IndexName = "index.html"

// WriteIndex writes the page that lists every conversation in an archive.
//
// The entries are ordered by when each conversation last had a message, most
// recent first, because that is the order somebody looks for a conversation in.
func WriteIndex(entries []Entry, opts Options) (Result, error) {
	opts = opts.withDefaults()
	path := filepath.Join(opts.Directory, IndexName)

	listed := make([]Entry, len(entries))
	copy(listed, entries)
	sort.SliceStable(listed, func(i, j int) bool {
		return listed[i].Chat.LastAt.After(listed[j].Chat.LastAt)
	})

	page := indexPage{
		Head: head{
			Title:       "Archive",
			Noun:        "conversations",
			Placeholder: "Search for a person or group",
			CSS:         styles,
		},
		Tail: tail{JS: script},
	}

	var (
		messages int
		earliest time.Time
		latest   time.Time
	)
	for _, entry := range listed {
		messages += entry.Messages
		if !entry.Chat.CreatedAt.IsZero() && (earliest.IsZero() || entry.Chat.CreatedAt.Before(earliest)) {
			earliest = entry.Chat.CreatedAt
		}
		if entry.Chat.LastAt.After(latest) {
			latest = entry.Chat.LastAt
		}
		page.Rows = append(page.Rows, rowOf(entry, opts.Location))
	}

	page.Head.Subtitle = indexSubtitle(len(listed), messages, earliest, latest, opts.Location)
	page.Tail.Summary = fmt.Sprintf("%s across %s",
		plural(messages, "message", "messages"),
		plural(len(listed), "conversation", "conversations"))

	written, err := atomicWrite(path, opts.Overwrite, func(w io.Writer) error {
		out := bufio.NewWriterSize(w, 1<<16)
		if err := pages.ExecuteTemplate(out, "index", page); err != nil {
			return err
		}
		return out.Flush()
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Files: []string{path}, Bytes: written}, nil
}

// indexPage is the whole front page, which is small enough to assemble in memory:
// it holds one line per conversation, not one per message.
type indexPage struct {
	Head head
	Rows []indexRow
	Tail tail
}

// indexRow is one conversation in the list.
type indexRow struct {
	Name  string
	About string
	Count string
	File  string
}

func rowOf(entry Entry, loc *time.Location) indexRow {
	var about []string
	if entry.Chat.Kind != model.ChatDirect {
		about = append(about, entry.Chat.Kind.String())
	}
	if len(entry.Chat.Participants) > 0 {
		about = append(about, plural(len(entry.Chat.Participants), "member", "members"))
	}
	if !entry.Chat.LastAt.IsZero() {
		about = append(about, entry.Chat.LastAt.In(loc).Format("2 Jan 2006"))
	}
	if entry.Chat.Archived {
		about = append(about, "archived")
	}

	return indexRow{
		Name:  entry.Chat.Title(),
		About: strings.Join(about, " · "),
		Count: plural(entry.Messages, "message", "messages"),
		File:  entry.File,
	}
}

// indexSubtitle says what the archive holds in one line.
func indexSubtitle(conversations, messages int, earliest, latest time.Time, loc *time.Location) string {
	parts := []string{
		plural(conversations, "conversation", "conversations"),
		plural(messages, "message", "messages"),
	}
	if !earliest.IsZero() && !latest.IsZero() {
		parts = append(parts, earliest.In(loc).Format("January 2006")+" to "+latest.In(loc).Format("January 2006"))
	}
	return strings.Join(parts, " · ")
}
