package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/contacts"
	"github.com/jferrl/amberkeep/internal/export"
	"github.com/jferrl/amberkeep/internal/model"
	"github.com/jferrl/amberkeep/internal/source"
	"github.com/jferrl/amberkeep/internal/source/android"
)

// runExport writes the archive out in the formats the caller asked for.
func runExport(ctx context.Context, args []string) error {
	fs := newFlagSet("export", "write the archive out as web pages, text and structured data")
	var (
		db       = fs.String("db", "", "the decrypted message database, usually msgstore.db")
		out      = fs.String("out", "archive", "directory to write the archive into")
		formats  = fs.String("format", "html", "which formats to write: html, text, json, or all")
		bookPath = fs.String("contacts", "", "an address book, so conversations show names instead of numbers")
		waPath   = fs.String("whatsapp-contacts", "", "WhatsApp's own contacts database, usually wa.db")
		country  = fs.String("country", "", "dialling code for numbers saved without one, such as 34")
		zone     = fs.String("timezone", "", "time zone for timestamps (default: this machine's)")
		me       = fs.String("me", "You", "what to call yourself in the archive")
		notices  = fs.Bool("notices", false, "include what WhatsApp did as well as what people said")
		groups   = fs.Bool("groups", true, "include group conversations")
		force    = fs.Bool("force", false, "replace files that are already there")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *db == "" {
		fs.Usage()
		return fmt.Errorf("--db is needed")
	}

	wanted, err := parseFormats(*formats)
	if err != nil {
		return err
	}
	location, err := parseZone(*zone)
	if err != nil {
		return err
	}

	reader, err := source.Open(ctx, *db)
	if err != nil {
		return err
	}
	defer reader.Close()

	names, err := loadNames(ctx, reader.Directory(), *bookPath, *waPath, *country)
	if err != nil {
		return err
	}

	chats, err := reader.Chats(ctx)
	if err != nil {
		return err
	}

	// The index only makes sense when there are pages for it to link to.
	var indexed bool
	for _, format := range wanted {
		if format.name == "html" {
			indexed = true
		}
	}

	opts := export.Options{
		Directory:        *out,
		Names:            names,
		Location:         location,
		Me:               *me,
		IncludeNotices:   *notices,
		Overwrite:        *force,
		NoticeIdentified: noticeIdentifier(reader),
	}

	var (
		total   export.Result
		skipped int
		entries []export.Entry
		started = time.Now()
	)
	for _, chat := range chats {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !chat.Includable() || (!*groups && chat.Kind == model.ChatGroup) {
			skipped++
			continue
		}

		conv := export.Conversation{
			Chat: chat,
			Messages: func(yield func(model.Message, error) bool) {
				for m, err := range reader.Messages(ctx, chat) {
					if !yield(m, err) {
						return
					}
				}
			},
		}

		var written int
		for i, format := range wanted {
			result, err := format.write(conv, opts)
			if err != nil {
				return fmt.Errorf("exporting %s: %w", chat.Title(), err)
			}
			// Every format writes the same messages, so counting them once per
			// format would report several times what the archive actually holds.
			if i == 0 {
				total.Messages += result.Messages
				total.Skipped += result.Skipped
				written = result.Messages
			}
			total.Bytes += result.Bytes
			total.Files = append(total.Files, result.Files...)
		}
		total.Conversations++

		if indexed {
			entries = append(entries, export.Entry{
				Chat:     chat,
				File:     export.FileName(chat, ".html"),
				Messages: written,
			})
		}

		// A large archive takes minutes, and silence looks like a hang.
		if total.Conversations%100 == 0 {
			fmt.Fprintf(os.Stderr, "\r%d conversations, %d messages...", total.Conversations, total.Messages)
		}
	}
	fmt.Fprint(os.Stderr, "\r\033[K")

	// The index is written last because only now is it known what the archive
	// holds. Several thousand files in a folder is a pile; this is what makes it
	// something somebody can open.
	if indexed {
		result, err := export.WriteIndex(entries, opts)
		if err != nil {
			return fmt.Errorf("writing the archive index: %w", err)
		}
		total.Bytes += result.Bytes
		total.Files = append(total.Files, result.Files...)
	}

	fmt.Printf("wrote %d conversations and %d messages to %s\n",
		total.Conversations, total.Messages, abbreviate(*out))
	fmt.Printf("  %d files, %s\n", len(total.Files), humanSize(total.Bytes))
	if total.Skipped > 0 {
		fmt.Printf("  %d housekeeping notices left out (pass --notices to keep them)\n", total.Skipped)
	}
	if skipped > 0 {
		fmt.Printf("  %d empty or excluded conversations skipped\n", skipped)
	}
	fmt.Printf("  took %s\n", time.Since(started).Round(time.Second))
	if indexed {
		fmt.Printf("\nopen %s to read the archive\n",
			abbreviate(filepath.Join(*out, export.IndexName)))
	}

	if named := names.Identified(); named == 0 {
		fmt.Printf("\nNo names were available, so conversations are labelled by phone number.\n")
		fmt.Printf("Pass --contacts with an address book export to fix that.\n")
	}
	return nil
}

// format is one way of writing a conversation.
type format struct {
	name  string
	write func(export.Conversation, export.Options) (export.Result, error)
}

// parseFormats turns the option into the writers to run.
func parseFormats(s string) ([]format, error) {
	available := map[string]format{
		"html": {"html", export.WriteHTML},
		"text": {"text", export.WriteText},
		"json": {"json", export.WriteJSON},
	}

	var wanted []format
	for _, name := range strings.Split(s, ",") {
		name = strings.ToLower(strings.TrimSpace(name))
		switch name {
		case "":
			continue
		case "all":
			return []format{available["html"], available["text"], available["json"]}, nil
		case "both":
			// It means two, and it meant these two before there were three.
			return []format{available["text"], available["json"]}, nil
		}
		f, ok := available[name]
		if !ok {
			return nil, fmt.Errorf("there is no format called %q; choose html, text, json, or all", name)
		}
		wanted = append(wanted, f)
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("no format was chosen")
	}
	return wanted, nil
}

// parseZone resolves the time zone to show timestamps in.
//
// Getting this wrong shifts every conversation by hours without any sign that it
// happened, so an unknown name is refused rather than quietly ignored.
func parseZone(name string) (*time.Location, error) {
	if name == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("there is no time zone called %q; use a name like Europe/Madrid: %w", name, err)
	}
	return loc, nil
}

// loadNames builds the address book from whatever the user has, and says what
// each source contributed.
//
// A conversation labelled with a phone number is the single most noticeable way
// an archive can disappoint, so this reports its results rather than working
// quietly.
func loadNames(ctx context.Context, names *model.Directory, bookPath, waPath, country string) (*model.Directory, error) {
	book := contacts.New(country)
	if waPath != "" {
		read, err := book.ReadWhatsAppContacts(ctx, waPath)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "read %d names from %s\n", read, abbreviate(waPath))
	}
	if bookPath != "" {
		read, err := book.ReadVCardFile(bookPath)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "read %d names from %s\n", read, abbreviate(bookPath))
		if read == 0 {
			fmt.Fprintf(os.Stderr,
				"warning: that address book named nobody. Check it is a .vcf export,\n"+
					"         and that --country is set if the numbers have no country code.\n")
		}
	}

	if book.Len() > 0 {
		applied := book.ApplyTo(names)
		fmt.Fprintf(os.Stderr, "matched %d of them to this archive\n", applied)
	}
	return names, nil
}

// noticeIdentifier answers whether a notice code is one the reader that produced
// the archive can phrase, so an export can admit what it could not put into words
// instead of leaving a consumer to guess.
//
// Only the Android reader has a verified table of these. An iPhone store records
// its own codes and no reliable public source says what they mean, so nothing is
// claimed for them rather than sentences being invented.
func noticeIdentifier(reader source.Archive) func(int) bool {
	if reader.Platform() == "android" {
		return android.IsIdentifiedNotice
	}
	return func(int) bool { return false }
}
