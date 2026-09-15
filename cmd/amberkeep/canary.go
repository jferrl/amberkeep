package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jferrl/amberkeep/internal/canary"
)

// runCanary reports what a database holds that this build has never seen.
//
// The command exists because the readers are deliberately quiet: they introspect
// rather than assume, so a database carrying something new opens perfectly and says
// nothing about the part that was not read. When WhatsApp changes its schema — which
// it does every few months — somebody finds out here, and the report is short enough
// and safe enough to paste into an issue without reading it first.
func runCanary(ctx context.Context, args []string) error {
	fs := newFlagSet("canary",
		"report what a database holds that this build has never seen: names and counts, never content")
	var (
		db       = fs.String("db", "", "the decrypted message database: msgstore.db, or ChatStorage.sqlite")
		asJSON   = fs.Bool("json", false, "write the report as JSON instead of as a page")
		emit     = fs.Bool("emit", false, "write this database's shape as a corpus entry, to send to the project")
		whatsApp = fs.String("whatsapp", "", "the WhatsApp version that wrote it, for --emit, such as 2.26.35.75")
		system   = fs.String("os", "", "the operating system version, for --emit, such as \"Android 16\"")
		device   = fs.String("device", "", "the phone model, for --emit, such as SM-A566B. A model, never a name")
		shapes   = fs.Bool("shapes", false, "list the shapes this build carries, and stop")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *shapes {
		fmt.Print(carried())
		return nil
	}
	if *db == "" {
		fs.Usage()
		return fmt.Errorf("--db is needed")
	}

	report, err := canary.Look(ctx, *db, version)
	if err != nil {
		return err
	}

	switch {
	case *emit:
		if *whatsApp == "" {
			return fmt.Errorf(
				"--whatsapp is needed to emit a shape: which version wrote a database is the one thing " +
					"it does not say about itself, and an entry without it tells nobody anything")
		}
		entry := report.Entry(canary.About{WhatsApp: *whatsApp, OS: *system, Device: *device})
		return write(os.Stdout, entry)
	case *asJSON:
		return write(os.Stdout, report)
	default:
		fmt.Print(page(report))
		return nil
	}
}

// write is the machine-readable form, indented because a person reads it before
// sending it to a stranger.
func write(to *os.File, what any) error {
	out := json.NewEncoder(to)
	out.SetIndent("", "  ")
	return out.Encode(what)
}

// page is the report as somebody reads it.
func page(r canary.Report) string {
	var b strings.Builder

	tables, columns := 0, 0
	for _, cols := range r.Schema.Tables {
		tables++
		columns += len(cols)
	}

	fmt.Fprintf(&b, "\nAmberkeep %s · %s · %d tables, %d columns\n",
		r.Build, phone(r.Schema.Platform), tables, columns)

	if len(r.Against) == 0 {
		fmt.Fprintf(&b, "\nThis build carries no shape to compare a %s database against, so nothing\n"+
			"below can be called new. Sending one with --emit is what fixes that.\n", phone(r.Schema.Platform))
	} else {
		fmt.Fprintf(&b, "Compared against %s\n", strings.Join(r.Against, ", "))
	}

	said := false
	said = list(&b, "Tables this build has never seen", r.NewTables) || said
	said = listed(&b, "Columns this build has never seen", r.NewColumns) || said
	said = list(&b, "Tables every known shape has and this one does not", r.GoneTables) || said
	said = listed(&b, "Columns every known shape has and this one does not", r.GoneColumns) || said

	if r.Counted != "" {
		if len(r.Unknown) == 0 {
			fmt.Fprintf(&b, "\nEvery message in %s carries a type this build has a meaning for.\n", r.Counted)
		} else {
			said = true
			fmt.Fprintf(&b, "\nMessage types with no meaning in this build, from %s\n", r.Counted)
			for _, count := range r.Unknown {
				name := fmt.Sprintf("%d", count.Type)
				if count.Type < 0 {
					name = "none"
				}
				fmt.Fprintf(&b, "  %-6s %8d %s\n", name, count.Rows, plural(count.Rows))
			}
			fmt.Fprintf(&b, "  %-6s %8d recognised\n", "", r.Recognised)
			if r.Schema.Platform == "iphone" {
				// Said because the list above reads worse than it is. An iPhone
				// records a media type beside each message, and a row whose number
				// means nothing here is still read as the picture or the document it
				// is when that type says so. What the list means is that the number
				// alone settled nothing, which is the part worth extending.
				fmt.Fprint(&b, "\n  On an iPhone the number is not the only evidence: a message whose type\n"+
					"  means nothing here is still read as a picture, a recording or a document\n"+
					"  when the media type beside it says so. These are the rows where the\n"+
					"  number alone said nothing.\n")
			}
		}
	}

	if !said {
		fmt.Fprint(&b, "\nNothing here is new to this build.\n")
	}

	fmt.Fprint(&b, "\nEverything above is table names, column names, type codes and counts. Nothing\n"+
		"anybody wrote, was called or was numbered is in it, so it can be pasted into an\n"+
		"issue at github.com/jferrl/amberkeep as it stands.\n\n")
	return b.String()
}

func list(b *strings.Builder, heading string, names []string) bool {
	if len(names) == 0 {
		return false
	}
	fmt.Fprintf(b, "\n%s\n", heading)
	for _, name := range names {
		fmt.Fprintf(b, "  %s\n", name)
	}
	return true
}

func listed(b *strings.Builder, heading string, byTable map[string][]string) bool {
	if len(byTable) == 0 {
		return false
	}
	tables := make([]string, 0, len(byTable))
	for table := range byTable {
		tables = append(tables, table)
	}
	sort.Strings(tables)

	fmt.Fprintf(b, "\n%s\n", heading)
	for _, table := range tables {
		fmt.Fprintf(b, "  %-32s %s\n", table, strings.Join(byTable[table], ", "))
	}
	return true
}

// phone says which kind of database this was in words a person uses.
func phone(platform string) string {
	switch platform {
	case "android":
		return "an Android database"
	case "iphone":
		return "an iPhone store"
	default:
		return "a database"
	}
}

func plural(n int) string {
	if n == 1 {
		return "message"
	}
	return "messages"
}

// carried is what this build has to compare a database against.
//
// It is also where the compatibility matrix in docs/ comes from, which is why it
// prints the counts: a shape with three hundred tables in it is a phone somebody
// used, and one with eighteen is an iPhone store, and the difference is worth seeing
// at a glance.
func carried() string {
	var b strings.Builder
	fmt.Fprint(&b, "\nThe shapes this build was written against\n\n")

	for _, entry := range canary.Corpus() {
		columns := 0
		for _, cols := range entry.Tables {
			columns += len(cols)
		}
		fmt.Fprintf(&b, "  %-8s WhatsApp %-12s %-12s %4d tables %5d columns  seen %s",
			entry.Platform, entry.WhatsApp, entry.OS, len(entry.Tables), columns, entry.Seen)
		if entry.Device != "" {
			fmt.Fprintf(&b, "  on a %s", entry.Device)
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprint(&b, "\nOne of these is what --emit writes. Sending yours is what makes the next\n"+
		"person's report mean something.\n\n")
	return b.String()
}
