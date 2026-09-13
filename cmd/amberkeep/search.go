package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/search"
	"github.com/jferrl/amberkeep/internal/source"
)

// runSearch finds messages across the whole archive.
//
// The index is built the first time and reused afterwards, because building it
// over a million messages takes minutes and searching it takes milliseconds. It is
// rebuilt without being asked when the archive it was built from has changed: an
// index that has quietly fallen behind still answers, and the answer is missing
// everything said since.
func runSearch(ctx context.Context, args []string) error {
	fs := newFlagSet("search", "find messages anywhere in the archive")
	var (
		db       = fs.String("db", "", "the decrypted message database, usually msgstore.db")
		indexAt  = fs.String("index", "", "where to keep the index (default: beside the database)")
		bookPath = fs.String("contacts", "", "an address book, so results show names instead of numbers")
		waPath   = fs.String("whatsapp-contacts", "", "WhatsApp's own contacts database, usually wa.db")
		country  = fs.String("country", "", "dialling code for numbers saved without one, such as 34")
		zone     = fs.String("timezone", "", "time zone for timestamps (default: this machine's)")
		me       = fs.String("me", "You", "what to call yourself in the results")
		chat     = fs.String("chat", "", "restrict the search to one conversation, by its address")
		since    = fs.String("since", "", "only messages on or after this date, as 2019-06-14")
		until    = fs.String("until", "", "only messages on or before this date")
		limit    = fs.Int("limit", 25, "how many results to show")
		notices  = fs.Bool("notices", false, "index what WhatsApp did as well as what people said")
		rebuild  = fs.Bool("rebuild", false, "build the index again even if it looks current")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	term := strings.Join(fs.Args(), " ")
	if *db == "" || strings.TrimSpace(term) == "" {
		fs.Usage()
		return fmt.Errorf("--db and something to search for are needed")
	}

	location, err := parseZone(*zone)
	if err != nil {
		return err
	}
	from, err := parseDate(*since, location)
	if err != nil {
		return err
	}
	to, err := parseDate(*until, location)
	if err != nil {
		return err
	}
	if !to.IsZero() {
		// A day named as an end is inclusive: nobody means "until the first instant
		// of the fourteenth" when they say "until the fourteenth".
		to = to.AddDate(0, 0, 1).Add(-time.Nanosecond)
	}

	index, err := openIndex(ctx, *db, indexPath(*db, *indexAt), indexSettings{
		rebuild:  *rebuild,
		notices:  *notices,
		me:       *me,
		book:     *bookPath,
		whatsApp: *waPath,
		country:  *country,
	})
	if err != nil {
		return err
	}
	defer func() { _ = index.Close() }()

	marks := searchMarks()
	hits, err := index.Search(ctx, term, search.Query{
		Limit:  *limit,
		Chat:   *chat,
		Since:  from,
		Until:  to,
		Before: marks.before,
		After:  marks.after,
	})
	if err != nil {
		return err
	}
	total, err := index.Count(ctx, term, search.Query{Chat: *chat, Since: from, Until: to})
	if err != nil {
		return err
	}

	if len(hits) == 0 {
		fmt.Printf("nothing matched %q.\n", term)
		fmt.Printf("\nTry fewer words, or a part of one with a star: %s*\n", firstWord(term))
		return nil
	}

	for _, hit := range hits {
		fmt.Printf("%s · %s · %s\n",
			hit.Chat, hit.SentAt.In(location).Format("02/01/2006 15:04"), hit.Sender)
		fmt.Printf("  %s\n\n", oneLine(hit.Snippet))
	}

	switch {
	case total > len(hits):
		fmt.Printf("%d of %d matches. Pass --limit to see more.\n", len(hits), total)
	default:
		fmt.Printf("%s.\n", plural(total, "match", "matches"))
	}
	return nil
}

// indexSettings are what building an index needs, gathered so the signature of
// openIndex stays readable.
type indexSettings struct {
	rebuild  bool
	notices  bool
	me       string
	book     string
	whatsApp string
	country  string
}

// openIndex returns a usable index, building one if there is none or the one there
// no longer matches the archive.
func openIndex(ctx context.Context, db, at string, settings indexSettings) (*search.Index, error) {
	if !settings.rebuild {
		index, err := search.Open(ctx, at)
		if err == nil {
			stats, err := index.Stats(ctx)
			switch {
			case err != nil:
				_ = index.Close()
			case !stats.MatchesSource(db):
				_ = index.Close()
				fmt.Fprintln(os.Stderr, "the archive has changed since the index was built; building it again")
			case stats.Notices != settings.notices:
				_ = index.Close()
				fmt.Fprintln(os.Stderr, "the index was built with different settings; building it again")
			default:
				return index, nil
			}
		}
	}

	reader, err := source.Open(ctx, db)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	names, err := loadNames(ctx, reader.Directory(), settings.book, settings.whatsApp, settings.country)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "building the search index; this happens once and takes a few minutes\n")
	started := time.Now()

	index, err := search.Build(ctx, reader, at, db, search.Options{
		Names:          names,
		Me:             settings.me,
		IncludeNotices: settings.notices,
		Progress: func(conversations, messages int) {
			if conversations%100 == 0 {
				fmt.Fprintf(os.Stderr, "\r%d conversations, %d messages...", conversations, messages)
			}
		},
	})
	if err != nil {
		return nil, err
	}
	fmt.Fprint(os.Stderr, "\r\033[K")

	stats, err := index.Stats(ctx)
	if err != nil {
		_ = index.Close()
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "indexed %d messages from %d conversations in %s\n",
		stats.Messages, stats.Conversations, time.Since(started).Round(time.Second))
	return index, nil
}

// indexPath is where the index lives. Beside the archive by default, so a second
// search finds it without being told where it went.
func indexPath(db, chosen string) string {
	if chosen != "" {
		return chosen
	}
	return filepath.Join(filepath.Dir(db), filepath.Base(db)+".amberkeep-index")
}

// marks are what wraps a matched word in the output.
type marks struct {
	before string
	after  string
}

// searchMarks highlights the match when a person is reading the output, and does
// not when something else is: escape codes in a file somebody is grepping are
// worse than no highlight at all.
func searchMarks() marks {
	info, err := os.Stdout.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return marks{before: "[", after: "]"}
	}
	return marks{before: "\033[1;33m", after: "\033[0m"}
}

// parseDate reads a day as somebody would write one.
func parseDate(s string, loc *time.Location) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{"2006-01-02", "02/01/2006", "2006"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%q is not a date this understands; write it as 2019-06-14", s)
}

// oneLine keeps a snippet on one line, because a message with newlines in it would
// otherwise break the shape of the results.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// firstWord is used to suggest a shorter search when nothing matched.
func firstWord(s string) string {
	if fields := strings.Fields(s); len(fields) > 0 {
		return fields[0]
	}
	return s
}

// plural renders a count with the right form of its noun.
func plural(n int, singular, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, many)
}
