package android

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"time"
)

// Restoring the indexes WhatsApp's backup leaves out.
//
// A decrypted message database has no indexes at all. Not one, on any of the twenty
// tables a reader touches: the backup strips them to save space, and decrypting
// gives back the rows without them. Every query for a conversation's messages
// therefore scans the whole table, and on a real archive that is 1.12 million rows
// scanned several thousand times over.
//
// Putting them back takes about a second and adds five per cent to the file. It took
// exporting a real archive from five and a half minutes to fifty-five seconds.
//
// Almost all of that comes from one index. The tables holding what a message carried
// beyond words already have the column they are read by as their primary key, which
// SQLite indexes for free, so only the ones that do not are built here.
//
// What this writes is only derived data: an index holds no information that is not
// already in the table it indexes, and removing one again loses nothing. It is
// still a change to a file, so it never happens to a file this program was merely
// pointed at. It happens to the file this program itself just wrote, and otherwise
// only when somebody asks for it by name.

// indexPrefix marks the indexes this program made, so they can be recognised and
// removed again without touching WhatsApp's own.
const indexPrefix = "amberkeep_"

// Preparation is what restoring the indexes did.
type Preparation struct {
	// Created names the indexes that were added.
	Created []string
	// AlreadyThere counts the ones that did not need adding.
	AlreadyThere int
	// Grew is how many bytes the file gained.
	Grew int64
	// Took is how long it all cost.
	Took time.Duration
}

// NothingToDo reports whether the database was already in good shape.
func (p Preparation) NothingToDo() bool { return len(p.Created) == 0 }

// Prepare restores the indexes a decrypted database is missing.
//
// It opens the file for writing, which nothing else in this package does. The
// caller must have decided that is acceptable: it is the right thing for a file
// this program produced, and the wrong thing for somebody's only copy of anything.
func Prepare(ctx context.Context, path string) (Preparation, error) {
	before := sizeOf(path)
	started := time.Now()

	// Journalling is deliberately left on. Turning it off makes this about twice as
	// fast, and an interruption halfway through would then leave a database holding
	// somebody's whole history in an undefined state. A second is not worth that.
	db, err := sql.Open("sqlite", "file:"+url.PathEscape(path)+"?_pragma=busy_timeout(10000)")
	if err != nil {
		return Preparation{}, ErrUnreadable.withCause(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		return Preparation{}, ErrUnreadable.withCause(err)
	}

	s, err := introspect(ctx, db)
	if err != nil {
		return Preparation{}, ErrUnreadable.withCause(err)
	}

	var prepared Preparation
	for _, want := range indexesFor(s) {
		if err := ctx.Err(); err != nil {
			return prepared, err
		}
		if s.hasIndex(want.name) {
			prepared.AlreadyThere++
			continue
		}
		// The table and column names come from the schema this program just read,
		// never from a caller, and SQLite takes neither as a bound parameter.
		statement := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %q ON %q (%s)",
			want.name, want.table, want.columns)
		if _, err := db.ExecContext(ctx, statement); err != nil {
			// One index that cannot be made is one query that stays slow, not a
			// reason to abandon the rest.
			continue
		}
		prepared.Created = append(prepared.Created, want.name)
	}

	if err := db.Close(); err != nil {
		return prepared, ErrUnreadable.withCause(err)
	}

	prepared.Grew = sizeOf(path) - before
	prepared.Took = time.Since(started)
	return prepared, nil
}

// wanted is one index this program would like the database to have.
type wanted struct {
	name    string
	table   string
	columns string
}

// read names every table this package queries by a message row.
//
// It is a written list rather than every table with a message_row_id column,
// because a real database has a hundred and seventy-eight of those and this reader
// touches twenty of them. Indexing the rest would cost time and fifty megabytes to
// speed up queries nobody runs.
//
// A table added to a query belongs here in the same change. The test beside this
// one checks the names against the database rather than against a copy of this
// list, so a name that is wrong shows up as an index that was never built.
var read = []string{
	"message_add_on",
	"message_album",
	"message_call_log",
	"message_edit_info",
	"message_ephemeral",
	"message_forwarded",
	"message_group_invite",
	"message_location",
	"message_media",
	"message_mentions",
	"message_poll",
	"message_quoted",
	"message_quoted_media",
	"message_revoked",
	"message_system",
	"message_system_block_contact",
	"message_system_business_state",
	"message_system_chat_participant",
	"message_system_device_change",
	"message_system_group",
	"message_system_number_change",
	"message_system_username_change",
	"message_system_value_change",
	"message_system_with_group_nodes",
	"message_text",
	"message_thumbnail",
	"message_vcard",
	"message_vcard_jid",
}

// indexesFor works out what to build from what the database actually contains, so
// a table this version does not know about costs nothing and one that has been
// removed is simply skipped.
func indexesFor(s schema) []wanted {
	var out []wanted
	add := func(table, columns string) {
		out = append(out, wanted{name: indexPrefix + table, table: table, columns: columns})
	}

	// The one that matters most: every conversation is read by filtering on the
	// chat and ordering by time, and without this that is a full scan per page.
	if s.hasColumn("message", "chat_row_id") && s.hasColumn("message", "timestamp") {
		add("message", "chat_row_id, timestamp, _id")
	}

	// Everything a message carried beyond words lives in its own table, and each is
	// read by the row of the message it belongs to. Most of those tables already
	// have that column as their primary key, which SQLite indexes for free: of the
	// thirty indexes an earlier version of this built, twenty-three were duplicates
	// of one that already existed, and all of the gain came from the one above.
	for _, table := range read {
		if s.hasColumn(table, "message_row_id") && !s.keyedBy(table, "message_row_id") {
			add(table, "message_row_id")
		}
	}

	// Two that are joined by something else.
	if s.hasColumn("message_poll_option", "message_row_id") {
		add("message_poll_option", "message_row_id")
	}
	if s.hasColumn("message_add_on_reaction", "message_add_on_row_id") {
		add("message_add_on_reaction", "message_add_on_row_id")
	}
	return out
}

// Indexed reports whether the database has the indexes that make it quick to read.
//
// A reader says so rather than silently taking six times as long, because somebody
// waiting five minutes deserves to know it could be under one.
func (r *Reader) Indexed() bool {
	return r.schema.hasIndex(indexPrefix + "message")
}

// sizeOf is the file's size, or zero when it cannot be measured. It only feeds a
// line in a report.
func sizeOf(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
