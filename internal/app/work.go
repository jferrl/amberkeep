package app

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/backupfs"
	"github.com/jferrl/amberkeep/internal/contacts"
	"github.com/jferrl/amberkeep/internal/crypt15"
	"github.com/jferrl/amberkeep/internal/migrate"
	"github.com/jferrl/amberkeep/internal/model"
	"github.com/jferrl/amberkeep/internal/search"
	"github.com/jferrl/amberkeep/internal/source"
	"github.com/jferrl/amberkeep/internal/source/android"
)

// The pieces of work both front doors do.
//
// None of it prints anything. What is left in the command is the part that reads a
// flag and writes a line; everything here is the work itself, which a terminal and a
// window have to do identically or they are two programs wearing one name.

// ExtractPictures copies out the small copies of photographs, and reports how many
// it took and how much they came to.
//
// This is the difference between an iPhone archive that shows a decade of pictures
// and one that shows a decade of the words "image omitted". An Android database
// keeps them inside itself; an iPhone store keeps only the paths.
//
// What to say about the result belongs to the caller, because the two callers say
// it in different places: one to a terminal, one to somebody watching a wizard.
func ExtractPictures(ctx context.Context, archive *backupfs.Archive, out string) (taken, bytes int64, err error) {
	files, err := archive.Domain(ctx, WhatsAppDomain)
	if err != nil {
		return 0, 0, err
	}

	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return taken, bytes, err
		}
		if file.IsDir || !strings.HasSuffix(file.RelativePath, ThumbnailSuffix) {
			continue
		}
		where, ok := strings.CutPrefix(file.RelativePath, MediaPrefix)
		if !ok {
			continue
		}

		// Laid out as the store's own paths expect, so the folder is readable on its
		// own once the backup it came from is gone.
		destination := filepath.Join(out, filepath.FromSlash(where))
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return taken, bytes, fmt.Errorf("preparing somewhere for the pictures: %w", err)
		}
		if err := archive.Extract(ctx, destination, file); err != nil {
			// One picture that cannot be copied is one picture missing, not a reason
			// to abandon the rest.
			continue
		}
		taken++
		bytes += file.Size
	}

	return taken, bytes, nil
}

// Plural renders a count with the right form of its noun.
func Plural(n int, singular, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// HumanSize renders a byte count the way people talk about files.
func HumanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d bytes", n)
	}
	div, exp := int64(unit), 0
	for size := n / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

// LoadKey reads the key from a file, or accepts it directly, and says which it was.
//
// Both spellings are accepted because both are what people have: WhatsApp shows the
// key on screen to be written down, and anybody sensible then saves it in a file.
// Which one arrived matters only to the caller, who may have something to say about
// it, so it is reported rather than remarked on here.
func LoadKey(from string) (key crypt15.Key, fromFile bool, err error) {
	// #nosec G304 -- reading the file the user named is what --key means.
	if contents, err := os.ReadFile(from); err == nil {
		key, err := crypt15.ParseKey(string(contents))
		if err != nil {
			return crypt15.Key{}, true, fmt.Errorf("reading the key from %s: %w", Abbreviate(from), err)
		}
		return key, true, nil
	}

	// Not a file, so treat it as the key itself. The error deliberately does not
	// repeat what was passed, in case it ends up in a log.
	key, err = crypt15.ParseKey(from)
	if err != nil {
		return crypt15.Key{}, false, fmt.Errorf("that is neither a readable file nor a valid key: %w", err)
	}
	return key, false, nil
}

// Abbreviate shortens a path for display, so a summary stays readable when the
// user is working somewhere deep in their home directory.
func Abbreviate(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if after, found := strings.CutPrefix(path, home); found {
		return "~" + after
	}
	return path
}

// WriteDecrypted turns the backup into a database on disk.
//
// The database is written through rather than assembled in memory first. Holding
// it peaked at a gigabyte and a half for a 236 MB backup, because growing the
// plaintext buffer leaves the old copy and the new one live at the same moment
// while the ciphertext is still referenced.
//
// A failure takes the file with it. A half-written database is worse than none:
// it looks like something somebody could open.
func WriteDecrypted(key crypt15.Key, encrypted []byte, target string) (int64, error) {
	// Created for this user only: a decrypted archive is every message they have
	// ever sent or received.
	// #nosec G304 -- the destination is the path the user asked to write to.
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, fmt.Errorf("creating the database: %w", err)
	}

	abandon := func(cause error) (int64, error) {
		_ = file.Close()
		_ = os.Remove(target)
		return 0, cause
	}

	out := bufio.NewWriterSize(file, 1<<20)
	written, err := crypt15.DecryptTo(key, encrypted, out)
	if err != nil {
		return abandon(err)
	}
	if err := out.Flush(); err != nil {
		return abandon(fmt.Errorf("writing the database: %w", err))
	}
	if err := file.Close(); err != nil {
		return abandon(fmt.Errorf("writing the database: %w", err))
	}
	return written, nil
}

// PrepareOutput adds the indexes to a database this program itself just wrote.
//
// No permission is asked because nothing was asked to be preserved: the file did
// not exist a second ago, this command made it, and the backup it came from is
// untouched. A failure is worth a line and nothing more, since the database is
// perfectly readable without them, only slower.
func PrepareOutput(ctx context.Context, path string) {
	prepared, err := android.Prepare(ctx, path)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"note: could not add the indexes that make reading quick (%v); everything still works\n", err)
		return
	}
	if prepared.NothingToDo() {
		return
	}
	fmt.Fprintf(os.Stderr, "added %s so reading it is quick (%s, %s larger)\n",
		Plural(len(prepared.Created), "index", "indexes"),
		prepared.Took.Round(time.Millisecond), HumanSize(prepared.Grew))
}

// LoadNames builds the address book from whatever the user has, and says what
// each source contributed.
//
// A conversation labelled with a phone number is the single most noticeable way
// an archive can disappoint, so this reports its results rather than working
// quietly.
func LoadNames(ctx context.Context, names *model.Directory, bookPath, waPath, country string) (*model.Directory, error) {
	book := contacts.New(country)
	if waPath != "" {
		read, err := book.ReadWhatsAppContacts(ctx, waPath)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "read %d names from %s\n", read, Abbreviate(waPath))
	}
	if bookPath != "" {
		read, err := book.ReadVCardFile(bookPath)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "read %d names from %s\n", read, Abbreviate(bookPath))
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

// MentionPreparation tells somebody reading a slow archive that it need not be.
//
// The check is an optional one: a reader that cannot answer is a reader for which
// the question does not arise.
func MentionPreparation(reader any, path string) {
	indexed, asks := reader.(interface{ Indexed() bool })
	if !asks || indexed.Indexed() {
		return
	}
	fmt.Fprintf(os.Stderr,
		"note: this database has none of the indexes that make it quick to read, which\n"+
			"      is how WhatsApp's backups arrive. Reading it will take several times\n"+
			"      longer than it needs to. To fix that once, in about a second:\n\n"+
			"        amberkeep prepare --db %s\n\n", Abbreviate(path))
}

// OpenIndex returns a usable index, building one if there is none or the one there
// no longer matches the archive.
func OpenIndex(ctx context.Context, db, at string, settings IndexSettings) (*search.Index, error) {
	if !settings.Rebuild {
		index, err := search.Open(ctx, at)
		if err == nil {
			stats, err := index.Stats(ctx)
			switch {
			case err != nil:
				_ = index.Close()
			case !stats.MatchesSource(db):
				_ = index.Close()
				settings.announce("The archive has changed since the index was built; building it again.")
			case stats.Notices != settings.Notices:
				_ = index.Close()
				settings.announce("The index was built with different settings; building it again.")
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

	MentionPreparation(reader, db)

	names, err := LoadNames(ctx, reader.Directory(), settings.Book, settings.WhatsApp, settings.Country)
	if err != nil {
		return nil, err
	}

	settings.announce("Building the search index. This happens once and takes a few minutes.")
	started := time.Now()

	index, err := search.Build(ctx, reader, at, db, search.Options{
		Names:          names,
		Me:             settings.Me,
		IncludeNotices: settings.Notices,
		Progress: func(conversations, messages int) {
			if conversations%100 == 0 {
				settings.counting(conversations, messages)
			}
		},
	})
	if err != nil {
		return nil, err
	}
	settings.finished()

	stats, err := index.Stats(ctx)
	if err != nil {
		_ = index.Close()
		return nil, err
	}
	settings.announce(fmt.Sprintf("Indexed %d messages from %d conversations in %s.",
		stats.Messages, stats.Conversations, time.Since(started).Round(time.Second)))
	return index, nil
}

// IndexPath is where the index lives. Beside the archive by default, so a second
// search finds it without being told where it went.
func IndexPath(db, chosen string) string {
	if chosen != "" {
		return chosen
	}
	return filepath.Join(filepath.Dir(db), filepath.Base(db)+".amberkeep-index")
}

// IndexSettings are what building an index needs, gathered so the signature of
// OpenIndex stays readable.
type IndexSettings struct {
	Rebuild  bool
	Notices  bool
	Me       string
	Book     string
	WhatsApp string
	Country  string

	// Say is where to put the running commentary. Nothing means the terminal, which
	// is where a command belongs; the wizard sets it so the same sentences reach
	// somebody watching a browser instead.
	Say func(string)
}

// PlanMigration opens both sides and works out what would happen.
func PlanMigration(ctx context.Context, p Planning) (migrate.Plan, string, source.Archive, error) {
	from, err := source.Open(ctx, p.Android)
	if err != nil {
		return migrate.Plan{}, "", nil, err
	}
	if _, err := LoadNames(ctx, from.Directory(), p.Book, p.WhatsApp, p.Country); err != nil {
		_ = from.Close()
		return migrate.Plan{}, "", nil, err
	}
	MentionPreparation(from, p.Android)

	// The store is taken out of the backup into a place of its own. The backup is
	// never opened for writing and never will be.
	work, err := os.MkdirTemp("", "amberkeep-migrate-")
	if err != nil {
		_ = from.Close()
		return migrate.Plan{}, "", nil, fmt.Errorf("making somewhere to work: %w", err)
	}
	store, err := TakeStoreOut(ctx, p.Backup, work)
	if err != nil {
		_ = from.Close()
		return migrate.Plan{}, "", nil, err
	}

	to, err := migrate.OpenTarget(ctx, store, p.Pairing)
	if err != nil {
		_ = from.Close()
		return migrate.Plan{}, "", nil, err
	}
	defer func() { _ = to.Close() }()

	plan, err := migrate.Build(ctx, from, to, p.Opts)
	if err != nil {
		_ = from.Close()
		return migrate.Plan{}, "", nil, err
	}
	return plan, store, from, nil
}

// Planning is what working out a migration needs.
type Planning struct {
	Android, Backup, Pairing string
	Book, WhatsApp, Country  string
	Opts                     migrate.Options
}

// ThumbnailSuffix marks the small copies, which are the ones worth having.
//
// The full-size files are in the same place and are not taken: the same device
// holds 5.7 GB of them against 13.9 MB of these, they are already in the phone's
// own gallery, and copying gigabytes to say what is already said helps nobody.
const ThumbnailSuffix = ".thumb"

// MediaPrefix is where a backup keeps what the store calls "Media/...".
//
// The store records a picture as `Media/<conversation>/5/e/<name>.thumb` and the
// backup files it one directory further in. That offset is written down nowhere;
// it was established by hashing the store's own paths against a real backup's index,
// where 9,422 of 9,941 then resolved and every one of them was a JPEG.
const MediaPrefix = "Message/"

// announce reports one thing that has happened.
func (s IndexSettings) announce(line string) {
	if s.Say != nil {
		s.Say(line)
		return
	}
	fmt.Fprintln(os.Stderr, line)
}

// counting reports how far the build has got.
//
// A terminal gets one line rewritten in place, because a few hundred of them
// scrolling past is noise. Anywhere else gets the sentence.
func (s IndexSettings) counting(conversations, messages int) {
	if s.Say != nil {
		s.Say(fmt.Sprintf("Indexed %d conversations and %d messages so far.", conversations, messages))
		return
	}
	fmt.Fprintf(os.Stderr, "\r%d conversations, %d messages...", conversations, messages)
}

// finished clears whatever counting left on the terminal.
func (s IndexSettings) finished() {
	if s.Say == nil {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
}

// TakeStoreOut copies WhatsApp's message store out of a backup, into a place of this
// command's own. The backup is opened read-only and is never written to.
func TakeStoreOut(ctx context.Context, backup, work string) (string, error) {
	archive, err := backupfs.Open(ctx, backup)
	if err != nil {
		return "", err
	}
	defer func() { _ = archive.Close() }()

	return archive.ExtractDatabase(ctx, work, WhatsAppDomain, ChatStorage)
}

// NoticeIdentifier answers whether a notice code is one the reader that produced
// the archive can phrase, so an export can admit what it could not put into words
// instead of leaving a consumer to guess.
//
// Only the Android reader has a verified table of these. An iPhone store records
// its own codes and no reliable public source says what they mean, so nothing is
// claimed for them rather than sentences being invented.
func NoticeIdentifier(reader source.Archive) func(int) bool {
	if reader.Platform() == "android" {
		return android.IsIdentifiedNotice
	}
	return func(int) bool { return false }
}
