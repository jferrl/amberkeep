// Package source opens an archive without the caller having to know which phone
// it came from.
//
// An Android message database and an iPhone message store answer the same
// questions in completely different shapes, and everything above this point only
// wants the answers. So this package sniffs which one it has been handed and
// returns a reader behind one interface. Adding a third platform is a case in one
// switch and a new package; nothing above changes.
package source

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the pure-Go SQLite driver

	"github.com/jferrl/amberkeep/internal/media"
	"github.com/jferrl/amberkeep/internal/model"
	"github.com/jferrl/amberkeep/internal/source/android"
	"github.com/jferrl/amberkeep/internal/source/ios"
)

// GuidanceUnrecognised is the identifier a failure to recognise a file carries.
const GuidanceUnrecognised = "source.unrecognised-archive"

// GuidanceNoFilesThere is the identifier a folder that holds no files carries.
const GuidanceNoFilesThere = "source.no-files-there"

// Error is what this package returns for problems a user can act on.
type Error struct {
	Guidance string
	msg      string
	err      error
}

func (e *Error) Error() string {
	if e.err != nil {
		return e.msg + ": " + e.err.Error()
	}
	return e.msg
}

func (e *Error) Unwrap() error { return e.err }

// Is matches on the guidance identifier.
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return e.Guidance == t.Guidance
}

func (e *Error) withCause(cause error) *Error {
	return &Error{Guidance: e.Guidance, msg: e.msg, err: cause}
}

// ErrUnrecognised reports a file that is a database but not one this build reads.
var ErrUnrecognised = &Error{Guidance: GuidanceUnrecognised,
	msg: "this file is a database, but not one this version can read"}

// ErrNoFilesThere reports a folder somebody named that is not the phone's.
//
// Said rather than ignored: a person who typed a path and was told nothing would
// conclude the program had looked and found nothing, when in fact they had pointed
// at the wrong folder and everything they wanted is one level up.
var ErrNoFilesThere = &Error{Guidance: GuidanceNoFilesThere,
	msg: "that folder holds no WhatsApp files"}

// Archive is everything the rest of the program needs from a reader.
//
// It is the same set for every platform, which is what lets the viewer, the
// exporter and the search index be written once.
type Archive interface {
	// Chats lists every conversation, most recently used first.
	Chats(ctx context.Context) ([]model.Chat, error)

	// Messages streams one conversation in the order it was written.
	Messages(ctx context.Context, chat model.Chat) iter.Seq2[model.Message, error]

	// Page reads a conversation backwards from a position, which is how somebody
	// reads one: open at the end, then scroll up.
	Page(ctx context.Context, chat model.Chat, before model.Cursor, limit int) ([]model.Message, model.Cursor, error)

	// Directory says who the archive knows about.
	Directory() *model.Directory

	// Layout names the shape of the file, for diagnostics.
	Layout() string

	// Platform names the phone the archive came from.
	Platform() string

	// Close releases the file.
	Close() error
}

// Option changes how an archive is opened.
type Option func(*opening)

// opening is what the options collect.
type opening struct{ files string }

// WithMedia names the folder holding the files the messages refer to.
//
// For somebody whose photographs are not where an archive would find them on its
// own: the database in one place and the phone's folder in another, which is what a
// person who copied the two across separately has. Either the WhatsApp folder or the
// Media folder inside it, the two things people are equally likely to hand over.
//
// A folder that turns out to hold no files is refused rather than ignored. Silence
// would read as "there is nothing there", when what happened is that the wrong
// folder was named and everything is one level up.
func WithMedia(where string) Option {
	return func(o *opening) { o.files = where }
}

// Open recognises an archive and returns a reader for it.
//
// Recognition is by what the file contains rather than by what it is called: a
// decrypted Android database and an iPhone store are both handed over as whatever
// the user chose to name them, and the name says nothing.
func Open(ctx context.Context, path string, options ...Option) (Archive, error) {
	var chosen opening
	for _, option := range options {
		option(&chosen)
	}

	tables, err := tablesIn(ctx, path)
	if err != nil {
		return nil, err
	}

	switch {
	case tables["ZWAMESSAGE"], tables["ZWACHATSESSION"]:
		// An iPhone store holds no pictures, only paths to them, and those paths are
		// relative. Relative to what is not written down anywhere, and the only
		// sensible answer is the store itself, which is also what `amberkeep extract`
		// arranges: the pictures land beside the store in the shape the paths expect.
		// A store copied out on its own simply has none, and the archive says so by
		// showing no photographs rather than by failing.
		var settings []ios.Option
		where := chosen.files
		if where == "" {
			where = filepath.Dir(path)
		}
		pictures, found := ios.MediaIn(where)
		switch {
		case found:
			settings = append(settings, ios.WithMedia(pictures))
		case chosen.files != "":
			return nil, ErrNoFilesThere.withCause(fmt.Errorf("%s holds no pictures", chosen.files))
		}

		reader, err := ios.Open(ctx, path, settings...)
		if err != nil {
			return nil, err
		}
		return iphone{reader}, nil

	case tables["message"], tables["messages"]:
		// The photographs an Android database refers to are a folder on the phone,
		// and a database records where they were without containing any of them. If
		// somebody has copied that folder next to the database — or decrypted the
		// database inside it, which is what happens when the whole WhatsApp folder
		// comes across — this finds it and the archive shows the files themselves
		// rather than the stamp-sized copies inside the database.
		var settings []android.Option
		folder, found := mediaFor(chosen, path)
		switch {
		case found:
			settings = append(settings, android.WithMedia(folder))
		case chosen.files != "":
			return nil, ErrNoFilesThere.withCause(
				fmt.Errorf("%s has no Media directory in it", chosen.files))
		}

		reader, err := android.Open(ctx, path, settings...)
		if err != nil {
			return nil, err
		}
		return androidPhone{reader}, nil

	default:
		return nil, ErrUnrecognised.withCause(
			fmt.Errorf("it has neither WhatsApp's Android tables nor its iPhone ones"))
	}
}

// mediaFor is the folder somebody named, or the one lying around the database.
func mediaFor(chosen opening, database string) (media.Folder, bool) {
	if chosen.files != "" {
		return media.In(chosen.files)
	}
	return mediaBeside(database)
}

// mediaBeside looks for the phone's WhatsApp folder around a database.
//
// Three places, all of them shapes people actually produce. The folder holding the
// database, for somebody who copied the pictures next to it. A WhatsApp folder
// beside it, for somebody who copied that whole folder across. And the folder above,
// because the phone keeps the database in `WhatsApp/Databases` and the pictures in
// `WhatsApp/Media`, so a database opened where it was found has its files one level
// up.
//
// Nothing here is configured and nothing is required: an archive with no folder
// anywhere near it is the ordinary case and shows what it always showed.
func mediaBeside(database string) (media.Folder, bool) {
	beside := filepath.Dir(database)
	for _, at := range []string{
		beside,
		filepath.Join(beside, "WhatsApp"),
		filepath.Dir(beside),
	} {
		if folder, found := media.In(at); found {
			return folder, true
		}
	}
	return media.Folder{}, false
}

// tablesIn reads the names of the tables a database has, which is all recognition
// needs and far less than opening it properly would cost.
func tablesIn(ctx context.Context, path string) (map[string]bool, error) {
	dsn := "file:" + url.PathEscape(path) + "?mode=ro&_pragma=query_only(1)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, ErrUnrecognised.withCause(err)
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return nil, ErrUnrecognised.withCause(err)
	}

	rows, err := db.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, ErrUnrecognised.withCause(err)
	}
	defer func() { _ = rows.Close() }()

	tables := make(map[string]bool, 64)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, ErrUnrecognised.withCause(err)
		}
		tables[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, ErrUnrecognised.withCause(err)
	}
	return tables, nil
}

// The two readers differ only in saying where they came from, which the interface
// asks for and neither reader should have to care about.

type androidPhone struct{ *android.Reader }

// Platform says where this archive came from.
func (androidPhone) Platform() string { return "android" }

type iphone struct{ *ios.Reader }

// Platform says where this archive came from.
func (iphone) Platform() string { return "iphone" }
