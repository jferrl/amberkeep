package android

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// The fixture below reproduces the shape of a real WhatsApp Android database: the
// same table and column names, the same relationships, the same conventions for
// hidden identifiers and voice messages. Only the content is invented.
//
// Structure is copied from a real device; message content never is, and no test may
// depend on a real database to pass.

// fixtureSchema holds only the tables the reader touches. A real database has more
// than three hundred, and reproducing them would test nothing extra.
const fixtureSchema = `
CREATE TABLE jid (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	user TEXT NOT NULL,
	server TEXT NOT NULL,
	agent INTEGER,
	device INTEGER,
	type INTEGER,
	raw_string TEXT
);
CREATE TABLE chat (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	jid_row_id INTEGER,
	hidden INTEGER,
	subject TEXT,
	created_timestamp INTEGER,
	archived INTEGER,
	sort_timestamp INTEGER
);
CREATE TABLE message (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	chat_row_id INTEGER,
	from_me INTEGER,
	key_id TEXT,
	sender_jid_row_id INTEGER,
	status INTEGER,
	origination_flags INTEGER,
	origin INTEGER,
	timestamp INTEGER,
	received_timestamp INTEGER,
	message_type INTEGER,
	text_data TEXT,
	starred INTEGER,
	sort_id INTEGER
);
CREATE TABLE jid_map (lid_row_id INTEGER, jid_row_id INTEGER, sort_id INTEGER);
CREATE TABLE lid_display_name (lid_row_id INTEGER PRIMARY KEY, display_name TEXT, username TEXT);
CREATE TABLE props (_id INTEGER PRIMARY KEY AUTOINCREMENT, key TEXT, value TEXT);
CREATE TABLE group_participant_user (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	group_jid_row_id INTEGER,
	user_jid_row_id INTEGER,
	rank INTEGER,
	pending INTEGER
);
CREATE TABLE message_media (
	message_row_id INTEGER PRIMARY KEY,
	chat_row_id INTEGER,
	mime_type TEXT,
	media_name TEXT,
	media_caption TEXT,
	media_duration INTEGER,
	file_size INTEGER,
	width INTEGER,
	height INTEGER,
	file_path TEXT
);
CREATE TABLE message_quoted (
	message_row_id INTEGER PRIMARY KEY,
	chat_row_id INTEGER,
	from_me INTEGER,
	sender_jid_row_id INTEGER,
	key_id TEXT,
	timestamp INTEGER,
	message_type INTEGER,
	text_data TEXT
);
CREATE TABLE message_add_on (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	chat_row_id INTEGER,
	from_me INTEGER,
	key_id TEXT,
	sender_jid_row_id INTEGER,
	parent_message_row_id INTEGER,
	timestamp INTEGER,
	message_add_on_type INTEGER
);
CREATE TABLE message_add_on_reaction (
	message_add_on_row_id INTEGER PRIMARY KEY,
	reaction TEXT,
	sender_timestamp INTEGER
);
CREATE TABLE message_edit_info (
	message_row_id INTEGER PRIMARY KEY,
	original_key_id TEXT,
	edited_timestamp INTEGER,
	sender_timestamp INTEGER
);
CREATE TABLE message_mentions (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	message_row_id INTEGER,
	jid_row_id INTEGER,
	display_name TEXT
);
`

// Row identifiers the tests refer to by name, so an assertion reads as a statement
// about people and conversations rather than about numbers.
const (
	jidAlice  = 1 // a contact addressed by phone number
	jidGroup  = 2
	jidHidden = 3 // the same person as jidCarol, behind a hidden identifier
	jidCarol  = 4
	jidStatus = 5

	chatAlice  = 1
	chatGroup  = 2
	chatStatus = 3
	chatEmpty  = 4
)

// Fixed instants, so ordering assertions are exact.
const (
	tsBase = int64(1_700_000_000_000)
	tsStep = int64(60_000)
)

// buildFixture writes a database with the shape of a real one and returns its path.
func buildFixture(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "msgstore.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the fixture: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(fixtureSchema); err != nil {
		t.Fatalf("creating the fixture schema: %v", err)
	}

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("populating the fixture: %v\nquery: %s", err, query)
		}
	}

	// People and conversations.
	exec(`INSERT INTO jid (_id, user, server, raw_string) VALUES
		(?, '34600111222', 's.whatsapp.net', '34600111222@s.whatsapp.net'),
		(?, '123456789-1600000000', 'g.us', '123456789-1600000000@g.us'),
		(?, '99887766554433', 'lid', '99887766554433@lid'),
		(?, '34600333444', 's.whatsapp.net', '34600333444@s.whatsapp.net'),
		(?, 'status', 'broadcast', 'status@broadcast')`,
		jidAlice, jidGroup, jidHidden, jidCarol, jidStatus)

	// The hidden identifier stands for Carol's phone address.
	exec(`INSERT INTO jid_map (lid_row_id, jid_row_id) VALUES (?, ?)`, jidHidden, jidCarol)
	// Carol chose a name for herself; Alice did not.
	exec(`INSERT INTO lid_display_name (lid_row_id, display_name) VALUES (?, 'Carol Q')`, jidHidden)
	exec(`INSERT INTO props (key, value) VALUES ('user_push_name', 'Owner Name')`)

	exec(`INSERT INTO chat (_id, jid_row_id, subject, created_timestamp, archived) VALUES
		(?, ?, NULL, ?, 0),
		(?, ?, 'Weekend plans', ?, 0),
		(?, ?, NULL, NULL, 0),
		(?, ?, NULL, NULL, 1)`,
		chatAlice, jidAlice, tsBase,
		chatGroup, jidGroup, tsBase,
		chatStatus, jidStatus,
		chatEmpty, jidCarol)

	exec(`INSERT INTO group_participant_user (group_jid_row_id, user_jid_row_id, rank) VALUES
		(?, ?, 2), (?, ?, 0)`, jidGroup, jidAlice, jidGroup, jidHidden)

	// A direct conversation: a reply, an edit, a reaction, a photo with a caption,
	// a voice message, an audio file, a deleted message and a notice.
	msg := func(id int64, chat int64, fromMe bool, sender any, step int64, kind int, text any, origin int) {
		t.Helper()
		var from int
		if fromMe {
			from = 1
		}
		exec(`INSERT INTO message
			(_id, chat_row_id, from_me, key_id, sender_jid_row_id, timestamp, message_type, text_data, origin, starred, origination_flags)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0)`,
			id, chat, from, keyFor(id), sender, tsBase+step*tsStep, kind, text, origin)
	}

	msg(1, chatAlice, false, jidAlice, 1, typeText, "first message", 0)
	msg(2, chatAlice, true, nil, 2, typeText, "a reply", 0)
	msg(3, chatAlice, false, jidAlice, 3, typeImage, nil, 0)
	msg(4, chatAlice, false, jidAlice, 4, typeAudio, nil, originVoiceNote)
	msg(5, chatAlice, true, nil, 5, typeAudio, nil, 0)
	msg(6, chatAlice, false, jidAlice, 6, typeDeleted, nil, 0)
	msg(7, chatAlice, false, jidAlice, 7, typeSystem, "security code changed", 0)
	msg(8, chatAlice, true, nil, 8, typeText, "edited later", 0)

	// The group conversation, where one sender is known only by a hidden identifier.
	msg(9, chatGroup, false, jidHidden, 9, typeText, "hello from a hidden id", 0)
	msg(10, chatGroup, false, jidAlice, 10, typeText, "mentioning someone", 0)
	msg(11, chatGroup, true, nil, 11, typeText, "mine", 0)

	msg(12, chatStatus, false, jidAlice, 12, typeText, "a status update", 0)

	// Details attached to those messages.
	exec(`INSERT INTO message_media (message_row_id, mime_type, media_name, media_caption, media_duration, file_size, width, height)
		VALUES (3, 'image/jpeg', 'IMG-0001.jpg', 'at the beach', 0, 12345, 1200, 900)`)
	exec(`INSERT INTO message_media (message_row_id, mime_type, media_duration, file_size)
		VALUES (4, 'audio/ogg; codecs=opus', 7, 2048)`)
	exec(`INSERT INTO message_media (message_row_id, mime_type, media_duration, file_size)
		VALUES (5, 'audio/mpeg', 210, 4096)`)

	exec(`INSERT INTO message_quoted (message_row_id, chat_row_id, from_me, sender_jid_row_id, message_type, text_data)
		VALUES (2, ?, 0, ?, ?, 'first message')`, chatAlice, jidAlice, typeText)

	exec(`INSERT INTO message_add_on (_id, chat_row_id, from_me, sender_jid_row_id, parent_message_row_id, timestamp, message_add_on_type)
		VALUES (1, ?, 0, ?, 1, ?, 56)`, chatAlice, jidAlice, tsBase)
	exec(`INSERT INTO message_add_on_reaction (message_add_on_row_id, reaction, sender_timestamp) VALUES (1, '👍', ?)`, tsBase)
	// A reaction row with no emoji must not become an empty reaction.
	exec(`INSERT INTO message_add_on (_id, chat_row_id, from_me, sender_jid_row_id, parent_message_row_id, timestamp, message_add_on_type)
		VALUES (2, ?, 1, NULL, 1, ?, 56)`, chatAlice, tsBase)
	exec(`INSERT INTO message_add_on_reaction (message_add_on_row_id, reaction, sender_timestamp) VALUES (2, '', ?)`, tsBase)

	exec(`INSERT INTO message_edit_info (message_row_id, edited_timestamp) VALUES (8, ?)`, tsBase+99*tsStep)
	exec(`INSERT INTO message_mentions (message_row_id, jid_row_id) VALUES (10, ?)`, jidHidden)

	if err := db.Close(); err != nil {
		t.Fatalf("closing the fixture: %v", err)
	}
	return path
}

// keyFor builds the stable message identifier WhatsApp assigns.
func keyFor(id int64) string {
	const alphabet = "0123456789ABCDEF"
	out := make([]byte, 0, 20)
	out = append(out, "3EB0"...)
	for i := range 16 {
		out = append(out, alphabet[(int(id)*7+i)%len(alphabet)])
	}
	return string(out)
}

// openFixture opens a reader over a freshly built fixture.
func openFixture(t *testing.T) *Reader {
	t.Helper()

	r, err := Open(context.Background(), buildFixture(t))
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Errorf("Close() failed: %v", err)
		}
	})
	return r
}
