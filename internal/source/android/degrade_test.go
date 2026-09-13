package android

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jferrl/amberkeep/internal/model"
)

// The tests here cover the promise that matters most for longevity: a database
// missing tables or columns this build expects must still open and still read, with
// whatever it does have. WhatsApp adds and removes things every few months, and a
// reader that insists on a fixed shape stops working the moment that happens.

// minimalSchema is the least a database can contain and still be a conversation
// archive: who exists, which conversations there are, and what was said. Every
// detail table is absent.
const minimalSchema = `
CREATE TABLE jid (_id INTEGER PRIMARY KEY, user TEXT, server TEXT, raw_string TEXT);
CREATE TABLE chat (_id INTEGER PRIMARY KEY, jid_row_id INTEGER);
CREATE TABLE message (
	_id INTEGER PRIMARY KEY,
	chat_row_id INTEGER,
	from_me INTEGER,
	key_id TEXT,
	sender_jid_row_id INTEGER,
	timestamp INTEGER,
	message_type INTEGER,
	text_data TEXT
);
`

func TestReadsADatabaseMissingEverythingOptional(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "minimal.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the database: %v", err)
	}
	if _, err := db.Exec(minimalSchema); err != nil {
		t.Fatalf("creating the schema: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO jid (_id, user, server, raw_string)
			VALUES (1, '34600111222', 's.whatsapp.net', '34600111222@s.whatsapp.net');
		INSERT INTO chat (_id, jid_row_id) VALUES (1, 1);
		INSERT INTO message (_id, chat_row_id, from_me, key_id, sender_jid_row_id, timestamp, message_type, text_data)
			VALUES (1, 1, 0, 'KEY1', 1, 1700000000000, 0, 'still readable'),
			       (2, 1, 1, 'KEY2', NULL, 1700000060000, 0, 'so is this');`); err != nil {
		t.Fatalf("populating the database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the database: %v", err)
	}

	r, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() refused a database that has everything it truly needs: %v", err)
	}
	defer r.Close()

	chats, err := r.Chats(context.Background())
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("read %d conversations, want 1", len(chats))
	}
	if chats[0].Messages != 2 {
		t.Errorf("Messages = %d, want 2", chats[0].Messages)
	}
	if chats[0].Title() != "+34600111222" {
		t.Errorf("Title() = %q", chats[0].Title())
	}
	// A conversation with no subject column and no membership table has no members,
	// which is a fact about the source rather than an error.
	if len(chats[0].Participants) != 0 {
		t.Errorf("Participants = %d, want none", len(chats[0].Participants))
	}

	var read []model.Message
	for m, err := range r.Messages(context.Background(), chats[0]) {
		if err != nil {
			t.Fatalf("Messages() failed: %v", err)
		}
		read = append(read, m)
	}
	if len(read) != 2 {
		t.Fatalf("read %d messages, want 2", len(read))
	}
	if read[0].Text != "still readable" || !read[1].IsFromMe() {
		t.Errorf("messages were not read faithfully: %+v", read)
	}
	for _, m := range read {
		if m.Attachment != nil || m.Quote != nil || len(m.Reactions) != 0 {
			t.Errorf("details were invented for a database that has none: %+v", m)
		}
	}

	// This database records no name for its owner. That is not an error: the archive
	// is complete and readable, and supplying a word for "the person whose archive
	// this is" belongs to the layer that knows the reader's language.
	if got := r.Directory().Owner().DisplayName(); got != "" {
		t.Errorf("owner label = %q, want it left to the presentation layer", got)
	}
}

func TestEditsAreFoundUnderEitherColumnName(t *testing.T) {
	t.Parallel()

	// Some WhatsApp versions record when a message was edited under a different
	// column name. The reader tries them in order rather than assuming one.
	tests := []struct {
		name       string
		columns    string
		insert     string
		wantEdited bool
	}{
		{
			name:       "the current column name",
			columns:    "message_row_id INTEGER PRIMARY KEY, edited_timestamp INTEGER",
			insert:     `INSERT INTO message_edit_info (message_row_id, edited_timestamp) VALUES (1, 1700000600000)`,
			wantEdited: true,
		},
		{
			name:       "the fallback column name",
			columns:    "message_row_id INTEGER PRIMARY KEY, sender_timestamp INTEGER",
			insert:     `INSERT INTO message_edit_info (message_row_id, sender_timestamp) VALUES (1, 1700000600000)`,
			wantEdited: true,
		},
		{
			name:       "neither, so the table is ignored",
			columns:    "message_row_id INTEGER PRIMARY KEY, something_else INTEGER",
			insert:     `INSERT INTO message_edit_info (message_row_id, something_else) VALUES (1, 1)`,
			wantEdited: false,
		},
		{
			name:       "a row with no timestamp still records that an edit happened",
			columns:    "message_row_id INTEGER PRIMARY KEY, edited_timestamp INTEGER",
			insert:     `INSERT INTO message_edit_info (message_row_id, edited_timestamp) VALUES (1, NULL)`,
			wantEdited: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "edits.db")
			db, err := sql.Open("sqlite", "file:"+path)
			if err != nil {
				t.Fatalf("creating the database: %v", err)
			}
			if _, err := db.Exec(minimalSchema + "CREATE TABLE message_edit_info (" + tt.columns + ");"); err != nil {
				t.Fatalf("creating the schema: %v", err)
			}
			if _, err := db.Exec(`
				INSERT INTO jid (_id, user, server, raw_string)
					VALUES (1, '34600111222', 's.whatsapp.net', '34600111222@s.whatsapp.net');
				INSERT INTO chat (_id, jid_row_id) VALUES (1, 1);
				INSERT INTO message (_id, chat_row_id, from_me, key_id, sender_jid_row_id, timestamp, message_type, text_data)
					VALUES (1, 1, 1, 'KEY1', NULL, 1700000000000, 0, 'edited later');`); err != nil {
				t.Fatalf("populating the database: %v", err)
			}
			if _, err := db.Exec(tt.insert); err != nil {
				t.Fatalf("recording the edit: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatalf("closing the database: %v", err)
			}

			r, err := Open(context.Background(), path)
			if err != nil {
				t.Fatalf("Open() failed: %v", err)
			}
			defer r.Close()

			chats, err := r.Chats(context.Background())
			if err != nil {
				t.Fatalf("Chats() failed: %v", err)
			}
			for m, err := range r.Messages(context.Background(), chats[0]) {
				if err != nil {
					t.Fatalf("Messages() failed: %v", err)
				}
				if m.WasEdited() != tt.wantEdited {
					t.Errorf("WasEdited() = %v, want %v", m.WasEdited(), tt.wantEdited)
				}
			}
		})
	}
}

func TestPagingCrossesBatchBoundaries(t *testing.T) {
	t.Parallel()

	// Messages are read in batches, so a conversation larger than one batch exercises
	// the paging cursor. Getting this wrong would silently drop or repeat messages,
	// which is the worst kind of bug an archive can have.
	const total = pageSize + 137

	path := filepath.Join(t.TempDir(), "long.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the database: %v", err)
	}
	if _, err := db.Exec(minimalSchema); err != nil {
		t.Fatalf("creating the schema: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO jid (_id, user, server, raw_string)
			VALUES (1, '34600111222', 's.whatsapp.net', '34600111222@s.whatsapp.net');
		INSERT INTO chat (_id, jid_row_id) VALUES (1, 1);`); err != nil {
		t.Fatalf("populating the database: %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("starting a transaction: %v", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO message
		(_id, chat_row_id, from_me, key_id, sender_jid_row_id, timestamp, message_type, text_data)
		VALUES (?, 1, 0, ?, 1, ?, 0, ?)`)
	if err != nil {
		t.Fatalf("preparing the insert: %v", err)
	}
	for i := 1; i <= total; i++ {
		// Deliberately give several messages the same instant, which real
		// conversations do, so paging cannot rely on the timestamp alone.
		ts := int64(1_700_000_000_000) + int64(i/3)*1000
		if _, err := stmt.Exec(i, keyFor(int64(i)), ts, "message"); err != nil {
			t.Fatalf("inserting a message: %v", err)
		}
	}
	if err := stmt.Close(); err != nil {
		t.Fatalf("closing the insert: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("committing: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the database: %v", err)
	}

	r, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer r.Close()

	chats, err := r.Chats(context.Background())
	if err != nil {
		t.Fatalf("Chats() failed: %v", err)
	}

	seen := make(map[int64]bool, total)
	var count int
	var last model.Message
	for m, err := range r.Messages(context.Background(), chats[0]) {
		if err != nil {
			t.Fatalf("Messages() failed: %v", err)
		}
		if seen[m.ID] {
			t.Fatalf("message %d was read twice", m.ID)
		}
		seen[m.ID] = true
		if count > 0 && m.SentAt.Before(last.SentAt) {
			t.Fatalf("message %d arrived out of order", m.ID)
		}
		last = m
		count++
	}
	if count != total {
		t.Errorf("read %d messages, want %d", count, total)
	}
}
