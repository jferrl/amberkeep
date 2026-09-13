// Package ios reads a WhatsApp iPhone message store.
//
// The file is ChatStorage.sqlite, a Core Data database: tables named ZWAMESSAGE
// and ZWACHATSESSION, columns prefixed with Z, timestamps counted in seconds from
// the first of January 2001. Which columns exist changes between WhatsApp releases,
// so every query is built from what the file actually contains.
//
// It is opened read-only and never written to.
//
// The shape of the reader is the Android one: a conversation list held in memory,
// which is a few megabytes even for a large archive, and messages streamed a page at
// a time so that a conversation of any length costs the same to read.
package ios

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the pure-Go SQLite driver

	"github.com/jferrl/amberkeep/internal/model"
)

// Guidance identifiers, as described in the principles: every user-facing failure
// maps to an explanation rather than to raw text.
const (
	GuidanceNotAMessageStore = "ios.not-a-message-store"
	GuidanceUnreadable       = "ios.unreadable"
	GuidanceEncryptedBackup  = "ios.encrypted-backup"
)

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

// Is matches on the guidance identifier, so a wrapped error still compares equal
// to the sentinel it came from.
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

var (
	// ErrNotAMessageStore reports a database that is not WhatsApp's iPhone store.
	ErrNotAMessageStore = &Error{Guidance: GuidanceNotAMessageStore,
		msg: "this file is a database but not a WhatsApp iPhone message store"}

	// ErrUnreadable reports a file that could not be opened or queried at all.
	ErrUnreadable = &Error{Guidance: GuidanceUnreadable,
		msg: "the iPhone message store could not be read"}

	// ErrLocked reports a file that is still encrypted, which is what an encrypted
	// iPhone backup hands out in place of a database.
	ErrLocked = &Error{Guidance: GuidanceEncryptedBackup,
		msg: "this file is encrypted, so it is not a readable database"}
)

// Reader answers questions about one iPhone message store.
type Reader struct {
	db     *sql.DB
	schema schema

	directory *model.Directory
	// sessions maps a session row to the conversation it stands for, because every
	// message names its conversation by row rather than by address.
	sessions map[int64]model.Chat
	// members maps a group member row to the address it stands for. In a group every
	// incoming message names its sender this way and nowhere else.
	members map[int64]model.JID
}

// Open reads the store at path. The file is opened read-only, so the caller's
// original is safe even if it is the only copy in existence.
//
// The caller closes the reader.
func Open(ctx context.Context, path string) (*Reader, error) {
	// query_only is belt and braces alongside the read-only mode: neither this code
	// nor the driver's own bookkeeping may write to a file we were handed.
	dsn := "file:" + url.PathEscape(path) +
		"?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, ErrUnreadable.withCause(err)
	}
	// One connection is enough for a single-user desktop tool and keeps the
	// read-only pragmas from having to be reapplied per connection.
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		if looksEncrypted(err) {
			return nil, ErrLocked.withCause(err)
		}
		return nil, ErrUnreadable.withCause(err)
	}

	s, err := introspect(ctx, db)
	if err != nil {
		_ = db.Close()
		return nil, ErrUnreadable.withCause(err)
	}
	if absent := s.missing(tableMessage, tableSession); len(absent) > 0 {
		_ = db.Close()
		return nil, ErrNotAMessageStore.withCause(
			fmt.Errorf("missing tables: %s", strings.Join(absent, ", ")))
	}

	r := &Reader{db: db, schema: s, directory: model.NewDirectory()}
	if err := r.loadDirectory(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return r, nil
}

// Close releases the database.
func (r *Reader) Close() error {
	if err := r.db.Close(); err != nil {
		return ErrUnreadable.withCause(err)
	}
	return nil
}

// Directory returns who this store knows about.
//
// An iPhone store is unusual in naming almost everybody already: WhatsApp on iOS
// copies the phone's address book into the conversation rows, so an archive read
// from one rarely needs a separate list of contacts.
func (r *Reader) Directory() *model.Directory { return r.directory }

// Layout names the database generation, for diagnostics and the schema report.
func (r *Reader) Layout() string { return r.schema.layout.String() }

// looksEncrypted reports whether a failure to open is the one an encrypted iPhone
// backup produces: the file is not a database at all, it is ciphertext.
func looksEncrypted(err error) bool {
	return strings.Contains(err.Error(), "file is not a database") ||
		strings.Contains(err.Error(), "file is encrypted")
}

// appleEpoch is the instant Core Data counts its timestamps from.
var appleEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// coreDataTime converts one of Core Data's timestamps.
//
// They are seconds, possibly fractional, since the first of January 2001. A zero
// is not a date at the start of that year: it is a column nobody filled in, and
// rendering it as a real date would put messages in 2001 that were sent last week.
func coreDataTime(seconds sql.NullFloat64) time.Time {
	if !seconds.Valid || seconds.Float64 == 0 {
		return time.Time{}
	}
	whole := int64(seconds.Float64)
	nanos := int64((seconds.Float64 - float64(whole)) * float64(time.Second))
	return appleEpoch.Add(time.Duration(whole)*time.Second + time.Duration(nanos))
}

// loadDirectory reads who the store knows about, from every place it records a name.
//
// Three sources, in increasing order of trust: the name somebody chose for
// themselves, the name they have in a group, and the name the phone's own address
// book gave them. The Directory itself resolves the precedence.
func (r *Reader) loadDirectory(ctx context.Context) error {
	if err := r.loadPushNames(ctx); err != nil {
		return err
	}
	return r.loadMembers(ctx)
}

// loadPushNames reads the names people chose for themselves.
func (r *Reader) loadPushNames(ctx context.Context) error {
	if !r.schema.has(tablePushName) {
		return nil
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT ZJID, ZPUSHNAME FROM `+tablePushName+` WHERE ZJID IS NOT NULL`)
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading names: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var address, name sql.NullString
		if err := rows.Scan(&address, &name); err != nil {
			return ErrUnreadable.withCause(fmt.Errorf("reading a name: %w", err))
		}
		if jid := model.ParseJID(address.String); !jid.IsZero() {
			r.directory.Add(model.Contact{JID: jid, PushName: name.String})
		}
	}
	if err := rows.Err(); err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading names: %w", err))
	}
	return nil
}

// loadMembers reads who belongs to which group, and what they are called there.
//
// This is also the only place a group message's sender is recorded: the message
// itself names a member row, not an address.
func (r *Reader) loadMembers(ctx context.Context) error {
	r.members = make(map[int64]model.JID)
	if !r.schema.has(tableMember) {
		return nil
	}

	contactName := r.schema.columnOrNull(tableMember, "ZCONTACTNAME")
	firstName := r.schema.columnOrNull(tableMember, "ZFIRSTNAME")

	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT Z_PK, ZMEMBERJID, %s, %s FROM %s`,
		contactName, firstName, tableMember))
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading group members: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var (
			pk               int64
			address          sql.NullString
			saved, firstOnly sql.NullString
		)
		if err := rows.Scan(&pk, &address, &saved, &firstOnly); err != nil {
			return ErrUnreadable.withCause(fmt.Errorf("reading a group member: %w", err))
		}
		jid := model.ParseJID(address.String)
		if jid.IsZero() {
			continue
		}
		r.members[pk] = jid

		// A member row carries the name from the phone's address book, which is the
		// best name there is. The first name alone is a poor second and only used
		// when nothing else was saved.
		name := saved.String
		if name == "" {
			name = firstOnly.String
		}
		if name != "" {
			r.directory.Add(model.Contact{JID: jid, Name: name})
		}
	}
	if err := rows.Err(); err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading group members: %w", err))
	}
	return nil
}
