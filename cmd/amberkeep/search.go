package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/app"
	"github.com/jferrl/amberkeep/internal/search"
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

	index, err := app.OpenIndex(ctx, *db, app.IndexPath(*db, *indexAt), app.IndexSettings{
		Rebuild:  *rebuild,
		Notices:  *notices,
		Me:       *me,
		Book:     *bookPath,
		WhatsApp: *waPath,
		Country:  *country,
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
		fmt.Printf("%s.\n", app.Plural(total, "match", "matches"))
	}
	return nil
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
