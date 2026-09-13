// Package android reads a decrypted WhatsApp Android message database.
//
// The database is opened read-only and is never written to. Which columns exist
// changes between WhatsApp releases, so every query is built from what the file
// actually contains rather than from a fixed schema; see schema.go.
//
// A reader resolves who wrote what as it goes. Recent WhatsApp versions hide phone
// numbers behind opaque identifiers, and recovering a real name means combining the
// address table, the hidden-identifier mapping, the names people chose for
// themselves, and whatever the caller supplies from the phone's address book.
package android

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
	GuidanceNotAMessageDatabase = "android.not-a-message-database"
	GuidanceLegacyUnsupported   = "android.legacy-schema-unsupported"
	GuidanceUnreadable          = "android.unreadable"
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
	// ErrNotAMessageDatabase reports a SQLite file that is not a WhatsApp message
	// database, which usually means the wrong file was chosen.
	ErrNotAMessageDatabase = &Error{Guidance: GuidanceNotAMessageDatabase,
		msg: "this file is a database but not a WhatsApp message database"}

	// ErrLegacyUnsupported reports the pre-2021 schema, which this build cannot read yet.
	ErrLegacyUnsupported = &Error{Guidance: GuidanceLegacyUnsupported,
		msg: "this backup uses WhatsApp's older database layout, which this version cannot read yet"}

	// ErrUnreadable reports a file that could not be opened or queried at all.
	ErrUnreadable = &Error{Guidance: GuidanceUnreadable,
		msg: "the message database could not be read"}
)

// SQLite is left on its own defaults on purpose.
//
// A larger page cache and memory-mapped reads are the obvious thing to reach for,
// and measuring them says not to. On a database without its indexes they took a
// quarter off the time, because the work was a full scan and caching a scan helps.
// With the indexes restored the same settings bought 2 per cent and cost three
// times the memory: 486 MB of resident pages against 152, for one second in
// fifty-five. The indexes are the fix; this was the symptom.

// Reader answers questions about one message database.
//
// It holds the address book and the conversation list in memory, which is a few
// megabytes even for very large archives, and streams messages on demand.
type Reader struct {
	db     *sql.DB
	schema schema

	directory *model.Directory
	// jids maps a row identifier to the address it stands for. Every reference to a
	// person in this database is a row identifier, so this is consulted constantly.
	jids map[int64]model.JID
	// jidRows is the same mapping the other way round, for the two places that have
	// an address and need the row it came from.
	jidRows map[string]int64
}

// Open reads the database at path. The file is opened read-only, so the caller's
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
		return nil, ErrUnreadable.withCause(err)
	}

	s, err := introspect(ctx, db)
	if err != nil {
		_ = db.Close()
		return nil, ErrUnreadable.withCause(err)
	}

	switch s.layout {
	case layoutModern:
	case layoutLegacy:
		_ = db.Close()
		return nil, ErrLegacyUnsupported
	case layoutUnknown:
		_ = db.Close()
		return nil, ErrNotAMessageDatabase.withCause(
			fmt.Errorf("expected a message table; found %d tables", len(s.columns)))
	}

	if absent := s.missing("message", "chat", "jid"); len(absent) > 0 {
		_ = db.Close()
		return nil, ErrNotAMessageDatabase.withCause(
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

// Directory returns who this database knows about. A caller can add names from the
// phone's address book before reading conversations, and those take precedence over
// the names people chose for themselves.
func (r *Reader) Directory() *model.Directory { return r.directory }

// Layout names the database generation, for diagnostics and the schema report.
func (r *Reader) Layout() string { return r.schema.layout.String() }

// loadDirectory builds the address book from every source inside the database:
// the address table, the mapping from hidden identifiers to phone numbers, the
// names people chose for themselves, and the owner's own name.
func (r *Reader) loadDirectory(ctx context.Context) error {
	if err := r.loadJIDs(ctx); err != nil {
		return err
	}
	if err := r.loadAliases(ctx); err != nil {
		return err
	}
	if err := r.loadPushNames(ctx); err != nil {
		return err
	}
	return r.loadOwner(ctx)
}

// loadJIDs reads every address the database refers to.
func (r *Reader) loadJIDs(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx, `SELECT _id, user, server, raw_string FROM jid`)
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading addresses: %w", err))
	}
	defer rows.Close()

	r.jids = make(map[int64]model.JID)
	r.jidRows = make(map[string]int64)
	for rows.Next() {
		var (
			id     int64
			user   sql.NullString
			server sql.NullString
			raw    sql.NullString
		)
		if err := rows.Scan(&id, &user, &server, &raw); err != nil {
			return ErrUnreadable.withCause(fmt.Errorf("reading an address: %w", err))
		}

		j := model.JID{User: user.String, Server: model.Server(server.String), Raw: raw.String}
		if j.Raw == "" && j.User != "" {
			j.Raw = j.User + "@" + string(j.Server)
		}
		r.jids[id] = j
		r.jidRows[j.String()] = id

		// Only people belong in the address book; groups and channels are named by
		// their subject instead.
		if j.Server == model.ServerUser || j.Server == model.ServerHidden {
			r.directory.Add(model.Contact{JID: j})
		}
	}
	if err := rows.Err(); err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading addresses: %w", err))
	}
	return nil
}

// loadAliases links hidden identifiers to the phone addresses they stand for.
// Without this, a large share of a modern archive is signed by strangers.
func (r *Reader) loadAliases(ctx context.Context) error {
	if !r.schema.hasColumn("jid_map", "lid_row_id") {
		return nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT lid_row_id, jid_row_id FROM jid_map`)
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading hidden identifiers: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var hidden, phone int64
		if err := rows.Scan(&hidden, &phone); err != nil {
			return ErrUnreadable.withCause(fmt.Errorf("reading a hidden identifier: %w", err))
		}
		h, okHidden := r.jids[hidden]
		p, okPhone := r.jids[phone]
		if okHidden && okPhone {
			r.directory.Alias(h, p)
		}
	}
	if err := rows.Err(); err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading hidden identifiers: %w", err))
	}
	return nil
}

// loadPushNames reads the names people chose for themselves. They are the only
// names available for anyone not in the phone's address book, which in a real
// archive is most people.
func (r *Reader) loadPushNames(ctx context.Context) error {
	if !r.schema.hasColumn("lid_display_name", "display_name") {
		return nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT lid_row_id, display_name FROM lid_display_name
		 WHERE display_name IS NOT NULL AND display_name <> ''`)
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading self-chosen names: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return ErrUnreadable.withCause(fmt.Errorf("reading a self-chosen name: %w", err))
		}
		j, ok := r.jids[id]
		if !ok {
			continue
		}
		// Recording it against the hidden identifier is enough: the directory links
		// that to the person's phone address in both directions, so the name is found
		// whichever address a message was signed with.
		r.directory.Add(model.Contact{JID: j, PushName: name})
	}
	if err := rows.Err(); err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading self-chosen names: %w", err))
	}
	return nil
}

// loadOwner reads the archive owner's own display name.
func (r *Reader) loadOwner(ctx context.Context) error {
	if !r.schema.has("props") {
		return nil
	}
	var name sql.NullString
	// The owner's own name is a convenience, not a requirement: an archive whose
	// author is labelled generically is still complete and correct. A failure here
	// is therefore recorded as "not found" rather than failing the whole open.
	if err := r.db.QueryRowContext(ctx,
		`SELECT value FROM props WHERE key = 'user_push_name'`).Scan(&name); err != nil {
		return nil //nolint:nilerr // an absent optional name is not a failure to open
	}
	if name.Valid && name.String != "" {
		r.directory.SetOwner(model.Contact{Name: name.String})
	}
	return nil
}

// Chats returns every conversation in the database, most recently active first,
// including empty ones. Callers decide what to keep; model.Chat.Includable states
// the usual default.
func (r *Reader) Chats(ctx context.Context) ([]model.Chat, error) {
	subject := r.schema.columnOrNull("chat", "subject")
	created := r.schema.columnOrNull("chat", "created_timestamp")
	archived := r.schema.columnOrNull("chat", "archived")

	query := fmt.Sprintf(`
		SELECT chat._id, chat.jid_row_id, %s, %s, %s,
		       COALESCE(stats.n, 0), stats.last_at
		FROM chat
		LEFT JOIN (
			SELECT chat_row_id, COUNT(*) AS n, MAX(timestamp) AS last_at
			FROM message GROUP BY chat_row_id
		) AS stats ON stats.chat_row_id = chat._id
		ORDER BY stats.last_at DESC NULLS LAST, chat._id`, subject, created, archived)

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, ErrUnreadable.withCause(fmt.Errorf("listing conversations: %w", err))
	}
	defer rows.Close()

	var chats []model.Chat
	for rows.Next() {
		var (
			id        int64
			jidRowID  int64
			subject   sql.NullString
			createdAt sql.NullInt64
			archived  sql.NullBool
			count     int
			lastAt    sql.NullInt64
		)
		if err := rows.Scan(&id, &jidRowID, &subject, &createdAt, &archived, &count, &lastAt); err != nil {
			return nil, ErrUnreadable.withCause(fmt.Errorf("reading a conversation: %w", err))
		}

		jid := r.jids[jidRowID]
		chat := model.Chat{
			ID:        id,
			JID:       jid,
			Kind:      model.ChatKindOf(jid),
			CreatedAt: epochMillis(createdAt),
			LastAt:    epochMillis(lastAt),
			Archived:  archived.Bool,
			Messages:  count,
		}
		chat.Name = r.nameFor(chat, subject)
		chats = append(chats, chat)
	}
	if err := rows.Err(); err != nil {
		return nil, ErrUnreadable.withCause(fmt.Errorf("listing conversations: %w", err))
	}

	if err := r.loadParticipants(ctx, chats); err != nil {
		return nil, err
	}
	return chats, nil
}

// nameFor chooses what to call a conversation: a group's subject, or the best name
// known for the other person.
func (r *Reader) nameFor(chat model.Chat, subject sql.NullString) string {
	if subject.Valid && subject.String != "" {
		return subject.String
	}
	if chat.Kind == model.ChatDirect {
		if c := r.directory.Lookup(chat.JID); c.IsIdentified() {
			return c.DisplayName()
		}
	}
	return ""
}

// loadParticipants fills in group membership for the conversations that have any,
// in one pass rather than one query per group.
func (r *Reader) loadParticipants(ctx context.Context, chats []model.Chat) error {
	if !r.schema.hasColumn("group_participant_user", "group_jid_row_id") {
		return nil
	}

	byJIDRow := make(map[int64]int, len(chats))
	for i, c := range chats {
		if c.Kind == model.ChatGroup {
			byJIDRow[r.jidRowOf(c.JID)] = i
		}
	}
	if len(byJIDRow) == 0 {
		return nil
	}

	rank := r.schema.columnOrNull("group_participant_user", "rank")
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT group_jid_row_id, user_jid_row_id, %s FROM group_participant_user`, rank))
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading group members: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var groupRow, userRow int64
		var rank sql.NullInt64
		if err := rows.Scan(&groupRow, &userRow, &rank); err != nil {
			return ErrUnreadable.withCause(fmt.Errorf("reading a group member: %w", err))
		}
		idx, ok := byJIDRow[groupRow]
		if !ok {
			continue
		}
		jid := r.jids[userRow]
		chats[idx].Participants = append(chats[idx].Participants, model.Participant{
			JID:   jid,
			Name:  r.directory.NameOf(jid),
			Admin: rank.Valid && rank.Int64 > 0,
		})
	}
	if err := rows.Err(); err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading group members: %w", err))
	}
	return nil
}

// jidRowOf finds the row identifier an address came from.
//
// The comment that used to be here said the map was small and a scan was cheaper
// than a second index. The map holds 100,464 addresses on a real archive and this
// runs once per group, which was 83 milliseconds of the half second it takes to
// list the conversations. The reverse map is built while the forward one is, so it
// costs nothing that was not already being paid.
func (r *Reader) jidRowOf(j model.JID) int64 {
	return r.jidRows[j.String()]
}

// epochMillis converts WhatsApp's millisecond timestamps. Zero and absent both mean
// "no time recorded", which is common for conversations that were never opened.
func epochMillis(v sql.NullInt64) time.Time {
	if !v.Valid || v.Int64 <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(v.Int64).UTC()
}
