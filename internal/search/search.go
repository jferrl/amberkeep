// Package search builds and queries a full-text index over an archive.
//
// The index is a separate SQLite file, derived and disposable: it is never the
// archive, and losing it costs only the time to build it again. That is what lets
// it be written with the durability settings turned down, which is most of why a
// million messages index in minutes rather than hours.
//
// What goes in is deliberately more than what people typed. A photograph is
// findable by the name of its file, a poll by its question and its answers, a
// shared link by the title the page had at the time. Indexing only the text column
// would hide most of what the reader worked to recover; see Message.SearchText.
//
// Accents are folded, so "jose" finds "José" and "anos" finds "años". That is not
// a nicety in Spanish: it is the difference between a search that works and one
// that quietly returns nothing.
package search

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"net/url"
	"os"
	"time"

	_ "modernc.org/sqlite" // registers the pure-Go SQLite driver

	"github.com/jferrl/amberkeep/internal/model"
)

// Guidance identifiers, as described in the principles: every user-facing failure
// maps to an explanation rather than to raw text.
const (
	GuidanceIndexUnreadable = "search.index-unreadable"
	GuidanceNoFullText      = "search.no-full-text-support"
	GuidanceEmptyQuery      = "search.nothing-to-search-for"
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
	// ErrUnreadable reports an index that could not be opened, built or queried.
	ErrUnreadable = &Error{Guidance: GuidanceIndexUnreadable,
		msg: "the search index could not be read"}

	// ErrNoFullText reports a SQLite build without the full-text extension, which
	// makes an index impossible rather than merely slow.
	ErrNoFullText = &Error{Guidance: GuidanceNoFullText,
		msg: "this build of SQLite has no full-text search"}

	// ErrEmptyQuery reports a search with no words in it.
	ErrEmptyQuery = &Error{Guidance: GuidanceEmptyQuery,
		msg: "there is nothing to search for"}
)

// version is the shape of an index file. An index built by an older version is
// rebuilt rather than read, because a half-understood index is worse than none.
const version = 1

// Options say how an index is built.
type Options struct {
	// Names resolves addresses to people, so a search can be for a person rather
	// than for a phone number.
	Names *model.Directory

	// Me is what to call the archive's owner.
	Me string

	// IncludeNotices indexes what WhatsApp did as well as what people said. They
	// are left out by default: a search for a name should not return two hundred
	// security-code notices.
	IncludeNotices bool

	// Progress is called as conversations are indexed, for a tool that wants to
	// show it. It may be nil.
	Progress func(conversations, messages int)
}

func (o Options) withDefaults() Options {
	if o.Me == "" {
		o.Me = "You"
	}
	if o.Names == nil {
		o.Names = model.NewDirectory()
	}
	if o.Progress == nil {
		o.Progress = func(int, int) {}
	}
	return o
}

// Source is an archive an index can be built from.
//
// It is the part of a reader an index needs and no more, so that indexing does not
// depend on which platform an archive came from.
type Source interface {
	Chats(ctx context.Context) ([]model.Chat, error)
	Messages(ctx context.Context, chat model.Chat) iter.Seq2[model.Message, error]
}

// Index is a searchable copy of an archive's words.
type Index struct {
	db   *sql.DB
	path string
}

// Build writes an index of src to path, replacing whatever was there.
//
// The file is built under a temporary name and moved into place, so an interrupted
// build leaves the previous index intact rather than a half-written one that would
// answer questions wrongly.
func Build(ctx context.Context, src Source, path, source string, opts Options) (*Index, error) {
	opts = opts.withDefaults()

	// Recorded so a later run can tell whether the archive has changed underneath
	// the index. Comparing the two files' timestamps would not do it: an index is
	// always newer than the file it was built from.
	var size, modified int64
	if info, err := os.Stat(source); err == nil {
		size, modified = info.Size(), info.ModTime().UnixMilli()
	}

	tmp := path + ".building"
	// A previous attempt that was killed leaves this behind.
	_ = os.Remove(tmp)

	db, err := open(ctx, tmp, false)
	if err != nil {
		return nil, err
	}

	if err := build(ctx, db, src, origin{path: source, size: size, modified: modified}, opts); err != nil {
		_ = db.Close()
		_ = os.Remove(tmp)
		return nil, err
	}
	if err := db.Close(); err != nil {
		_ = os.Remove(tmp)
		return nil, ErrUnreadable.withCause(err)
	}

	// An index is a copy of every word in somebody's history.
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return nil, ErrUnreadable.withCause(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return nil, ErrUnreadable.withCause(err)
	}
	return Open(ctx, path)
}

// Open reads an index that is already built.
func Open(ctx context.Context, path string) (*Index, error) {
	db, err := open(ctx, path, true)
	if err != nil {
		return nil, err
	}

	var found int
	row := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'version'`)
	if err := row.Scan(&found); err != nil {
		_ = db.Close()
		return nil, ErrUnreadable.withCause(fmt.Errorf("this file is not a search index: %w", err))
	}
	if found != version {
		_ = db.Close()
		return nil, ErrUnreadable.withCause(
			fmt.Errorf("this index was built by another version (%d, not %d)", found, version))
	}
	return &Index{db: db, path: path}, nil
}

// Close releases the index.
func (i *Index) Close() error {
	if err := i.db.Close(); err != nil {
		return ErrUnreadable.withCause(err)
	}
	return nil
}

// Path is where the index lives, for a tool that wants to say so.
func (i *Index) Path() string { return i.path }

// open connects to an index file, read-only when it is being queried.
func open(ctx context.Context, path string, readOnly bool) (*sql.DB, error) {
	dsn := "file:" + url.PathEscape(path) + "?_pragma=busy_timeout(5000)"
	if readOnly {
		dsn += "&mode=ro&_pragma=query_only(1)"
	} else {
		// An index is derived and disposable: if a build is interrupted the file is
		// thrown away and built again, so there is nothing for a journal to protect
		// and a great deal of time to be saved by not keeping one.
		dsn += "&_pragma=journal_mode(off)&_pragma=synchronous(off)"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, ErrUnreadable.withCause(err)
	}
	// One connection, because the pragmas above are per connection and a desktop
	// tool has one user.
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, ErrUnreadable.withCause(err)
	}
	return db, nil
}

// schema is the whole index. It is small on purpose: an index answers "where was
// this said", and the archive itself answers everything else.
const schema = `
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);

CREATE TABLE chats (
	id       INTEGER PRIMARY KEY,
	jid      TEXT    NOT NULL,
	title    TEXT    NOT NULL,
	kind     TEXT    NOT NULL,
	messages INTEGER NOT NULL,
	last_at  INTEGER
);

CREATE TABLE messages (
	id      INTEGER PRIMARY KEY,
	chat_id INTEGER NOT NULL,
	sender  TEXT    NOT NULL,
	from_me INTEGER NOT NULL,
	sent_at INTEGER NOT NULL,
	kind    TEXT    NOT NULL,
	body    TEXT    NOT NULL
);

CREATE INDEX messages_by_chat ON messages (chat_id, sent_at);
CREATE INDEX messages_by_time ON messages (sent_at);
`

// ftsSchema is separate because its failure means something specific: a SQLite
// without the full-text extension, which is worth saying plainly.
//
// remove_diacritics 2 is the whole reason a Spanish archive is searchable: it folds
// accents across the full Unicode range rather than the handful of Latin-1
// characters the older setting covered.
const ftsSchema = `
CREATE VIRTUAL TABLE fts USING fts5(
	body,
	content = 'messages',
	content_rowid = 'id',
	tokenize = "unicode61 remove_diacritics 2"
);
`

// origin identifies the archive an index was built from, closely enough to notice
// that it has been replaced.
type origin struct {
	path     string
	size     int64
	modified int64
}

// build fills a fresh index.
func build(ctx context.Context, db *sql.DB, src Source, from origin, opts Options) error {
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return ErrUnreadable.withCause(err)
	}
	if _, err := db.ExecContext(ctx, ftsSchema); err != nil {
		return ErrNoFullText.withCause(err)
	}

	chats, err := src.Chats(ctx)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ErrUnreadable.withCause(err)
	}
	defer func() { _ = tx.Rollback() }()

	addChat, err := tx.PrepareContext(ctx,
		`INSERT INTO chats (id, jid, title, kind, messages, last_at) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return ErrUnreadable.withCause(err)
	}
	defer func() { _ = addChat.Close() }()

	addMessage, err := tx.PrepareContext(ctx,
		`INSERT INTO messages (chat_id, sender, from_me, sent_at, kind, body) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return ErrUnreadable.withCause(err)
	}
	defer func() { _ = addMessage.Close() }()

	var indexed, conversations int
	for _, chat := range chats {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !chat.Includable() {
			continue
		}

		if _, err := addChat.ExecContext(ctx, chat.ID, chat.JID.String(), chat.Title(),
			chat.Kind.String(), chat.Messages, nullableTime(chat.LastAt)); err != nil {
			return ErrUnreadable.withCause(err)
		}

		for m, err := range src.Messages(ctx, chat) {
			if err != nil {
				return err
			}
			if !m.Displayable() || (m.Kind.IsNotice() && !opts.IncludeNotices) {
				continue
			}
			body := m.SearchText()
			if body == "" {
				// Nothing to find it by. The archive still holds it; an index of
				// empty strings would only slow every search down.
				continue
			}

			sender := opts.Me
			if !m.IsFromMe() {
				sender = opts.Names.NameOf(m.Sender)
			}
			if _, err := addMessage.ExecContext(ctx, chat.ID, sender, m.IsFromMe(),
				m.SentAt.UnixMilli(), m.Kind.String(), body); err != nil {
				return ErrUnreadable.withCause(err)
			}
			indexed++
		}

		conversations++
		opts.Progress(conversations, indexed)
	}

	// The full-text table takes its content from the messages table, so it is built
	// in one pass at the end rather than a row at a time, which is several times
	// faster and is the reason this is not a triggered table.
	if _, err := tx.ExecContext(ctx, `INSERT INTO fts(fts) VALUES ('rebuild')`); err != nil {
		return ErrUnreadable.withCause(err)
	}

	meta := [][2]string{
		{"version", fmt.Sprint(version)},
		{"built_at", time.Now().UTC().Format(time.RFC3339)},
		{"source", from.path},
		{"source_size", fmt.Sprint(from.size)},
		{"source_modified", fmt.Sprint(from.modified)},
		{"conversations", fmt.Sprint(conversations)},
		{"messages", fmt.Sprint(indexed)},
		{"notices", fmt.Sprint(opts.IncludeNotices)},
	}
	for _, pair := range meta {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO meta (key, value) VALUES (?, ?)`, pair[0], pair[1]); err != nil {
			return ErrUnreadable.withCause(err)
		}
	}

	if err := tx.Commit(); err != nil {
		return ErrUnreadable.withCause(err)
	}
	return nil
}

// nullableTime keeps an unknown time out of the index as null rather than as the
// first instant of 1970, which would sort wrongly and read as a real date.
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UnixMilli()
}

// Stats say what an index holds, for reporting it to somebody.
type Stats struct {
	Conversations int
	Messages      int
	Source        string
	BuiltAt       time.Time
	Notices       bool

	sourceSize     int64
	sourceModified int64
}

// MatchesSource reports whether the archive at path is still the one this index
// was built from.
//
// An index that has quietly fallen behind is worse than no index: it answers, and
// the answer is missing everything said since. Comparing the two files' timestamps
// cannot tell, because an index is always the newer file, so the archive's own size
// and date are recorded when it is built and compared here.
func (s Stats) MatchesSource(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Size() == s.sourceSize && info.ModTime().UnixMilli() == s.sourceModified
}

// Stats reads what the index recorded about itself when it was built.
func (i *Index) Stats(ctx context.Context) (Stats, error) {
	rows, err := i.db.QueryContext(ctx, `SELECT key, value FROM meta`)
	if err != nil {
		return Stats{}, ErrUnreadable.withCause(err)
	}
	defer func() { _ = rows.Close() }()

	var stats Stats
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return Stats{}, ErrUnreadable.withCause(err)
		}
		switch key {
		case "conversations":
			stats.Conversations = atoi(value)
		case "messages":
			stats.Messages = atoi(value)
		case "source":
			stats.Source = value
		case "notices":
			stats.Notices = value == "true"
		case "source_size":
			stats.sourceSize = int64(atoi(value))
		case "source_modified":
			stats.sourceModified = int64(atoi(value))
		case "built_at":
			// A date this cannot read leaves the field unset, which reads as "not
			// recorded". It is a line in a summary, not a reason to fail a search.
			if built, err := time.Parse(time.RFC3339, value); err == nil {
				stats.BuiltAt = built
			}
		}
	}
	if err := rows.Err(); err != nil {
		return Stats{}, ErrUnreadable.withCause(err)
	}
	return stats, nil
}

// atoi reads a count the index wrote, treating anything unreadable as zero: a
// wrong number in a summary is not worth failing a search over.
func atoi(s string) int {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0
	}
	return n
}
