package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/api"
	"github.com/jferrl/amberkeep/internal/backupfs"
	"github.com/jferrl/amberkeep/internal/search"
	"github.com/jferrl/amberkeep/internal/source"
)

// The wizard's hands.
//
// The server knows what to ask somebody and when to say so; it does not know how a
// crypt15 file is decrypted, where Apple hides a backup, or which of two databases
// it has been handed. All of that already lives in these commands, and this is the
// one place where the two meet. Everything here is a few lines calling a command's
// own helper, on purpose: the wizard must not become a second implementation that
// can disagree with the command line about what a backup is.

// Importer does the work the wizard asks for.
type Importer struct {
	// Me is what to call the archive's owner in exports and search results.
	Me string
	// Country is the dialling code for numbers an address book saved without one.
	Country string
	// Notices says whether the search index covers system messages as well.
	Notices bool
	// NoSearch skips building the index, which is the long part of an import.
	NoSearch bool
}

// Locations reports where Apple's own software puts backups on this platform, which
// is nowhere at all on the ones Apple ships nothing for.
func (i Importer) Locations() []string { return backupfs.Locations() }

// Backups lists the iPhone backups on this computer.
//
// A permission problem is returned beside the list rather than instead of it. On
// Windows there are two places backups live, and one being unreadable must not hide
// what was found in the other; on macOS a refusal is nearly always Full Disk Access,
// which is a thing somebody can go and fix, so it is worth saying rather than
// showing an empty list and letting them conclude their backup is gone.
func (i Importer) Backups() (backups []api.Backup, problem string) {
	found, err := backupfs.Backups()

	list := make([]api.Backup, 0, len(found))
	for _, b := range found {
		when := ""
		if !b.LastBackup.IsZero() {
			when = b.LastBackup.UTC().Format(time.RFC3339)
		}
		list = append(list, api.Backup{
			Path:        b.Path,
			DeviceName:  FirstNonEmpty(b.DeviceName, b.UDID),
			ProductType: b.ProductType,
			IOSVersion:  b.IOSVersion,
			LastBackup:  when,
			Encrypted:   b.Encrypted,
		})
	}
	if err == nil {
		return list, ""
	}

	problem = err.Error()
	if help := AdviseOn(err); help != "" {
		problem += "\n\n" + help
	}
	return list, problem
}

// Extract takes WhatsApp's message store out of an iPhone backup, with the small
// copies of photographs that the store itself only holds the paths to.
func (i Importer) Extract(ctx context.Context, backup, into string, say api.Progress) (string, error) {
	archive, err := backupfs.Open(ctx, backup)
	if err != nil {
		return "", err
	}
	defer func() { _ = archive.Close() }()

	say(api.StepExtracting, fmt.Sprintf("Reading the backup of %s, made %s.",
		FirstNonEmpty(archive.DeviceName, archive.UDID),
		archive.LastBackup.Local().Format(time.DateOnly)))

	path, err := archive.ExtractDatabase(ctx, into, WhatsAppDomain, ChatStorage)
	if err != nil {
		return "", err
	}

	// Without this an iPhone archive shows a decade of the words "image omitted".
	// A failure here is not a failure of the import: the messages are already out,
	// and an archive with no photographs is worth far more than no archive.
	say(api.StepExtracting, "Taking the pictures out as well.")
	taken, bytes, err := ExtractPictures(ctx, archive, into)
	switch {
	case err != nil && ctx.Err() != nil:
		return "", err
	case err != nil:
		fmt.Fprintf(os.Stderr, "note: the pictures could not all be taken out (%v)\n", err)
	case taken > 0:
		say(api.StepExtracting, fmt.Sprintf("Took out %s (%s).",
			Plural(int(taken), "picture", "pictures"), HumanSize(bytes)))
	}
	return path, nil
}

// Decrypt turns an encrypted Android backup into a database that can be read.
//
// The key is used and dropped. It is not written anywhere, not logged, and not put
// into any error this returns, because an error is the thing most likely to be
// pasted into a bug report.
func (i Importer) Decrypt(ctx context.Context, file, keySource, into string, say api.Progress) (string, error) {
	// Either spelling: WhatsApp shows the key on screen to be written down, and
	// anybody sensible then saves it in a file.
	key, _, err := LoadKey(keySource)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(into, 0o700); err != nil {
		return "", fmt.Errorf("preparing somewhere to write: %w", err)
	}
	target := filepath.Join(into, filepath.Base(DefaultOutput(file)))
	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("there is already a file at %s; move it somewhere else, "+
			"or choose another folder to write into", Abbreviate(target))
	}

	// The encrypted file is held whole because the cipher authenticates all of it
	// before releasing a single byte. The database it becomes is written through
	// rather than assembled in memory, which is the difference between a gigabyte
	// and a half of it and none.
	// #nosec G304 -- reading the backup the user named is the whole request.
	encrypted, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("reading the backup: %w", err)
	}
	say(api.StepDecrypting, fmt.Sprintf(
		"Decrypting %s. Nothing is being uploaded: this happens on this computer.",
		HumanSize(int64(len(encrypted)))))

	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, err := WriteDecrypted(key, encrypted, target); err != nil {
		return "", err
	}

	// A backup arrives with none of its indexes, which makes reading it several
	// times slower than it needs to be. This file did not exist a moment ago and
	// this program wrote it, so putting them back needs nobody's permission.
	say(api.StepPreparing, "Adding the indexes the backup leaves out, so reading is quick.")
	PrepareOutput(ctx, target)
	return target, nil
}

// Open reads an archive that is already readable, and builds the search index over
// it if there is not one already.
//
// Which platform it came from is worked out from what the file contains rather than
// from what it is called, because by this point it is whatever the user named it.
func (i Importer) Open(ctx context.Context, path, contacts string, say api.Progress) (api.Archive, error) {
	reader, err := source.Open(ctx, path)
	if err != nil {
		return nil, err
	}

	book, whatsApp := addressBook(contacts)
	if _, err := LoadNames(ctx, reader.Directory(), book, whatsApp, i.Country); err != nil {
		_ = reader.Close()
		return nil, err
	}
	MentionPreparation(reader, path)

	index, err := i.index(ctx, path, book, whatsApp, say)
	if err != nil {
		_ = reader.Close()
		return nil, err
	}
	return readable{Archive: reader, index: index}, nil
}

// index returns the full-text index over an archive, or nothing.
//
// Nothing is a perfectly good answer. Building an index writes a file beside the
// archive, and somebody may have put theirs somewhere that cannot be written to; an
// archive that opens and cannot be searched is far better than one that refuses to
// open at all. Being interrupted is the exception, because that was asked for.
func (i Importer) index(ctx context.Context, path, book, whatsApp string, say api.Progress) (*search.Index, error) {
	if i.NoSearch {
		return nil, nil
	}

	index, err := OpenIndex(ctx, path, IndexPath(path, ""), IndexSettings{
		Notices: i.Notices, Me: i.Me, Book: book, WhatsApp: whatsApp, Country: i.Country,
		Say: func(line string) { say(api.StepIndexing, line) },
	})
	switch {
	case err == nil:
		return index, nil
	case ctx.Err() != nil:
		return nil, err
	default:
		fmt.Fprintf(os.Stderr, "note: the search index could not be built (%v)\n", err)
		return nil, nil
	}
}

// addressBook works out which kind of contacts file was handed over.
//
// The wizard asks for one thing, because somebody has one thing: either the address
// book they exported from the phone or WhatsApp's own contacts database that sits
// beside the message store in a backup. Which it is can be read off the name, and
// asking them to classify it themselves would be asking them to know something the
// program can work out.
func addressBook(contacts string) (book, whatsApp string) {
	if contacts == "" {
		return "", ""
	}
	if strings.EqualFold(filepath.Ext(contacts), ".db") {
		return "", contacts
	}
	return contacts, ""
}

// Readable wraps an archive so the server can both read it and write it out.
//
// An archive opened from the command line used to be handed over raw, while one the
// wizard opened arrived wrapped. They behaved identically for reading, so nothing
// noticed — until the page could write an archive out, at which point the same
// program could export the archive it had imported and not the one it had been
// started with. One way in, so the two are the same object.
func Readable(archive source.Archive, index *search.Index) api.Archive {
	return readable{Archive: archive, index: index}
}

// readable is an archive and the index built over it, which are let go of together.
//
// They are one thing to everybody above: an index belongs to the archive it was
// built from and is worthless without it, so closing one and leaving the other open
// would leave a handle behind every time somebody tried a second backup.
type readable struct {
	source.Archive
	index *search.Index
}

// Index is the full-text index over this archive, if it has one.
func (r readable) Index() *search.Index { return r.index }

// Close lets go of both, and reports whichever failure came first.
func (r readable) Close() error {
	var problems []error
	if r.index != nil {
		problems = append(problems, r.index.Close())
	}
	problems = append(problems, r.Archive.Close())
	return errors.Join(problems...)
}
