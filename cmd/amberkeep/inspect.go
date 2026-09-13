package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
	"github.com/jferrl/amberkeep/internal/source/android"
)

// runInspect reports what an archive holds without writing anything.
//
// It exists so somebody can see what they have before committing to an export,
// and so a problem report can carry facts rather than impressions. It is also
// the honest place to say what this build could not recognise.
func runInspect(ctx context.Context, args []string) error {
	fs := newFlagSet("inspect", "report what an archive contains, without changing anything")
	var (
		db       = fs.String("db", "", "the decrypted message database, usually msgstore.db")
		contacts = fs.String("contacts", "", "an address book, to see how many conversations it would name")
		country  = fs.String("country", "", "dialling code for numbers saved without one, such as 34")
		full     = fs.Bool("full", false, "read every message; slower, but reports what was recovered")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *db == "" {
		fs.Usage()
		return fmt.Errorf("--db is needed")
	}

	reader, err := android.Open(ctx, *db)
	if err != nil {
		return err
	}
	defer reader.Close()

	if _, err := loadNames(ctx, reader, *contacts, "", *country); err != nil {
		return err
	}

	chats, err := reader.Chats(ctx)
	if err != nil {
		return err
	}

	// The columns are only aligned once everything has been written, so a report
	// that is never flushed is a report nobody sees. That failure is reported even
	// when the survey itself failed, rather than being swallowed by a defer.
	out := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	err = report(ctx, out, reader, chats, *db, *full)
	if flushErr := out.Flush(); err == nil {
		err = flushErr
	}
	return err
}

// report writes what the archive holds. A full report reads every message, which
// takes minutes on a large archive, so the quick one is the default.
func report(
	ctx context.Context,
	out *tabwriter.Writer,
	reader *android.Reader,
	chats []model.Chat,
	db string,
	full bool,
) error {
	fmt.Fprintf(out, "Archive\t%s\n", abbreviate(db))
	fmt.Fprintf(out, "Layout\t%s\n", reader.Layout())

	summary := summarise(chats)
	fmt.Fprintf(out, "Conversations\t%d (%d worth exporting)\n", len(chats), summary.included)
	fmt.Fprintf(out, "  direct\t%d\n", summary.byKind[model.ChatDirect])
	fmt.Fprintf(out, "  groups\t%d\n", summary.byKind[model.ChatGroup])
	if n := summary.byKind[model.ChatNewsletter]; n > 0 {
		fmt.Fprintf(out, "  channels\t%d\n", n)
	}
	fmt.Fprintf(out, "Messages\t%d\n", summary.messages)
	if !summary.earliest.IsZero() {
		fmt.Fprintf(out, "Span\t%s to %s\n",
			summary.earliest.Format(time.DateOnly), summary.latest.Format(time.DateOnly))
	}

	people := reader.Directory()
	fmt.Fprintf(out, "People\t%d known, %d with a name\n", people.Len(), people.Identified())
	fmt.Fprintf(out, "Named conversations\t%d of %d direct chats\n",
		summary.namedDirect(people), summary.byKind[model.ChatDirect])

	if !full {
		fmt.Fprintf(out, "\nPass --full to read every message and report what can be recovered.\n")
		return nil
	}

	// Flushed before the wait so the summary above is on screen while the slow
	// part runs, rather than appearing all at once at the end.
	fmt.Fprintln(out)
	if err := out.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "reading every message, which takes a few minutes on a large archive...")

	found, err := surveyContent(ctx, reader, chats)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Recovered beyond plain text\t\n")
	fmt.Fprintf(out, "  picture previews\t%d (%s of images whose files are gone)\n",
		found.previews, humanSize(found.previewBytes))
	fmt.Fprintf(out, "  system notices explained\t%d\n", found.notices)
	fmt.Fprintf(out, "  link previews\t%d\n", found.links)
	fmt.Fprintf(out, "  polls\t%d with %d answers\n", found.polls, found.pollOptions)
	fmt.Fprintf(out, "  shared places\t%d\n", found.places)
	fmt.Fprintf(out, "  contact cards\t%d\n", found.cards)
	fmt.Fprintf(out, "  calls\t%d\n", found.calls)
	fmt.Fprintf(out, "  replies\t%d\n", found.replies)
	fmt.Fprintf(out, "  reactions\t%d messages\n", found.reactions)
	fmt.Fprintf(out, "  deletions recorded\t%d\n", found.deletions)

	if len(found.unrecognised) > 0 {
		fmt.Fprintf(out, "\nNot recognised\t%d messages\n", found.unrecognisedTotal)
		for _, code := range sortedKeys(found.unrecognised) {
			fmt.Fprintf(out, "  type %d\t%d messages\n", code, found.unrecognised[code])
		}
		fmt.Fprintf(out, "\nThese are carried through with their type number rather than dropped.\n")
		fmt.Fprintf(out, "Reporting them is what gets them recognised in a later version.\n")
	}
	return nil
}

// chatSummary is what the conversation list alone can tell us, which is quick.
type chatSummary struct {
	byKind   map[model.ChatKind]int
	included int
	messages int
	earliest time.Time
	latest   time.Time
	chats    []model.Chat
}

func summarise(chats []model.Chat) chatSummary {
	s := chatSummary{byKind: make(map[model.ChatKind]int), chats: chats}
	for _, c := range chats {
		s.byKind[c.Kind]++
		if !c.Includable() {
			continue
		}
		s.included++
		s.messages += c.Messages
		if !c.LastAt.IsZero() {
			if s.latest.IsZero() || c.LastAt.After(s.latest) {
				s.latest = c.LastAt
			}
		}
		if !c.CreatedAt.IsZero() {
			if s.earliest.IsZero() || c.CreatedAt.Before(s.earliest) {
				s.earliest = c.CreatedAt
			}
		}
	}
	return s
}

// namedDirect counts the one-to-one conversations that show a person rather than
// a phone number, which is the number people actually notice.
func (s chatSummary) namedDirect(people *model.Directory) int {
	var named int
	for _, c := range s.chats {
		if c.Kind == model.ChatDirect && c.Includable() && people.Lookup(c.JID).IsIdentified() {
			named++
		}
	}
	return named
}

// contentSurvey counts what a full read recovered.
type contentSurvey struct {
	previews          int
	previewBytes      int64
	notices           int
	links             int
	polls             int
	pollOptions       int
	places            int
	cards             int
	calls             int
	replies           int
	reactions         int
	deletions         int
	unrecognised      map[int]int
	unrecognisedTotal int
}

// surveyContent reads every message and counts what was recovered.
func surveyContent(ctx context.Context, reader *android.Reader, chats []model.Chat) (contentSurvey, error) {
	found := contentSurvey{unrecognised: make(map[int]int)}

	for _, c := range chats {
		if !c.Includable() {
			continue
		}
		for m, err := range reader.Messages(ctx, c) {
			if err != nil {
				return found, err
			}
			if m.Attachment != nil && m.Attachment.HasPreview() {
				found.previews++
				found.previewBytes += int64(len(m.Attachment.Preview.Data))
			}
			if m.Notice != nil {
				found.notices++
			}
			if m.Link != nil {
				found.links++
			}
			if m.Poll != nil {
				found.polls++
				found.pollOptions += len(m.Poll.Options)
			}
			if m.Place != nil {
				found.places++
			}
			found.cards += len(m.Contacts)
			if m.Call != nil {
				found.calls++
			}
			if m.IsReply() {
				found.replies++
			}
			if len(m.Reactions) > 0 {
				found.reactions++
			}
			if m.Deleted != nil {
				found.deletions++
			}
			if m.Kind == model.KindUnknown {
				found.unrecognised[m.SourceType]++
				found.unrecognisedTotal++
			}
		}
		if err := ctx.Err(); err != nil {
			return found, err
		}
	}
	return found, nil
}

// sortedKeys returns map keys in order, so a report reads the same every time.
func sortedKeys(m map[int]int) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}
