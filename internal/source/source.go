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

	_ "modernc.org/sqlite" // registers the pure-Go SQLite driver

	"github.com/jferrl/amberkeep/internal/model"
	"github.com/jferrl/amberkeep/internal/source/android"
	"github.com/jferrl/amberkeep/internal/source/ios"
)

// GuidanceUnrecognised is the identifier a failure to recognise a file carries.
const GuidanceUnrecognised = "source.unrecognised-archive"

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

// Open recognises an archive and returns a reader for it.
//
// Recognition is by what the file contains rather than by what it is called: a
// decrypted Android database and an iPhone store are both handed over as whatever
// the user chose to name them, and the name says nothing.
func Open(ctx context.Context, path string) (Archive, error) {
	tables, err := tablesIn(ctx, path)
	if err != nil {
		return nil, err
	}

	switch {
	case tables["ZWAMESSAGE"], tables["ZWACHATSESSION"]:
		reader, err := ios.Open(ctx, path)
		if err != nil {
			return nil, err
		}
		return iphone{reader}, nil

	case tables["message"], tables["messages"]:
		reader, err := android.Open(ctx, path)
		if err != nil {
			return nil, err
		}
		return androidPhone{reader}, nil

	default:
		return nil, ErrUnrecognised.withCause(
			fmt.Errorf("it has neither WhatsApp's Android tables nor its iPhone ones"))
	}
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
