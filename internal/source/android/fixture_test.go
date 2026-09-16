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
	file_length INTEGER,
	file_hash TEXT,
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
CREATE TABLE message_thumbnail (message_row_id INTEGER PRIMARY KEY, thumbnail BLOB);
CREATE TABLE media_hash_thumbnail (media_hash TEXT PRIMARY KEY, thumbnail BLOB);
CREATE TABLE message_location (
	message_row_id INTEGER PRIMARY KEY,
	chat_row_id INTEGER,
	latitude REAL,
	longitude REAL,
	place_name TEXT,
	place_address TEXT,
	url TEXT,
	live_location_share_duration INTEGER
);
CREATE TABLE message_poll (
	message_row_id INTEGER PRIMARY KEY,
	selectable_options_count INTEGER,
	end_time INTEGER
);
CREATE TABLE message_poll_option (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	message_row_id INTEGER,
	option_name TEXT,
	vote_total INTEGER
);
CREATE TABLE call_log (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	jid_row_id INTEGER,
	from_me INTEGER,
	timestamp INTEGER,
	video_call INTEGER,
	duration INTEGER,
	call_result INTEGER,
	group_jid_row_id INTEGER
);
CREATE TABLE message_call_log (message_row_id INTEGER PRIMARY KEY, call_log_row_id INTEGER);
CREATE TABLE missed_call_logs (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	message_row_id INTEGER,
	video_call INTEGER,
	group_jid_row_id INTEGER
);
CREATE TABLE message_text (
	message_row_id INTEGER PRIMARY KEY,
	description TEXT,
	page_title TEXT,
	url TEXT
);
CREATE TABLE message_vcard (_id INTEGER PRIMARY KEY AUTOINCREMENT, message_row_id INTEGER, vcard TEXT);
CREATE TABLE message_vcard_jid (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	vcard_jid_row_id INTEGER,
	vcard_row_id INTEGER,
	message_row_id INTEGER
);
CREATE TABLE message_revoked (
	message_row_id INTEGER PRIMARY KEY,
	revoked_key_id TEXT,
	admin_jid_row_id INTEGER,
	revoke_timestamp INTEGER
);
CREATE TABLE message_forwarded (message_row_id INTEGER PRIMARY KEY, forward_score INTEGER, forward_origin INTEGER);
CREATE TABLE message_album (message_row_id INTEGER PRIMARY KEY, image_count INTEGER, video_count INTEGER);
CREATE TABLE message_ephemeral (message_row_id INTEGER PRIMARY KEY, duration INTEGER, expire_timestamp INTEGER);
CREATE TABLE message_group_invite (
	message_row_id INTEGER PRIMARY KEY,
	group_jid_row_id INTEGER,
	group_name TEXT,
	expiration INTEGER
);
CREATE TABLE message_quoted_media (
	message_row_id INTEGER PRIMARY KEY,
	mime_type TEXT,
	media_name TEXT,
	media_caption TEXT,
	media_duration INTEGER,
	thumbnail BLOB
);
CREATE TABLE message_system (message_row_id INTEGER PRIMARY KEY, action_type INTEGER);
CREATE TABLE message_system_group (message_row_id INTEGER PRIMARY KEY, is_me_joined INTEGER);
CREATE TABLE message_system_chat_participant (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	message_row_id INTEGER,
	user_jid_row_id INTEGER
);
CREATE TABLE message_system_value_change (message_row_id INTEGER PRIMARY KEY, old_data TEXT);
CREATE TABLE message_system_number_change (
	message_row_id INTEGER PRIMARY KEY,
	old_jid_row_id INTEGER,
	new_jid_row_id INTEGER
);
CREATE TABLE message_system_device_change (
	message_row_id INTEGER PRIMARY KEY,
	device_added_count INTEGER,
	device_removed_count INTEGER
);
CREATE TABLE message_system_business_state (
	message_row_id INTEGER PRIMARY KEY,
	privacy_message_type INTEGER,
	business_name TEXT
);
CREATE TABLE message_system_username_change (
	message_row_id INTEGER PRIMARY KEY,
	old_username TEXT,
	new_username TEXT
);
CREATE TABLE message_system_block_contact (message_row_id INTEGER PRIMARY KEY, is_blocked INTEGER);
CREATE TABLE message_system_with_group_nodes (message_row_id INTEGER PRIMARY KEY, group_subject TEXT);
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
	exec(`INSERT INTO message_media (message_row_id, mime_type, media_name, media_caption, media_duration, file_size, width, height, file_path)
		VALUES (3, 'image/jpeg', 'IMG-0001.jpg', 'at the beach', 0, 12345, 1200, 900,
			'Media/WhatsApp Images/IMG-0001.jpg')`)
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

	// Everything a message can carry beyond words, each in the table WhatsApp uses.
	msg(20, chatAlice, false, jidAlice, 20, typeLocation, nil, 0)
	msg(21, chatAlice, true, nil, 21, typePoll, "Where shall we eat?", 0)
	msg(22, chatAlice, false, jidAlice, 22, typeCall, nil, 0)
	msg(23, chatAlice, false, jidAlice, 23, typeMissedCall, nil, 0)
	msg(24, chatAlice, true, nil, 24, typeText, "look at this", 0)
	msg(25, chatAlice, false, jidAlice, 25, typeContact, nil, 0)
	msg(26, chatAlice, false, jidAlice, 26, typeText, "will vanish", 0)
	msg(27, chatAlice, true, nil, 27, typeGroupInvite, nil, 0)
	msg(28, chatAlice, false, jidAlice, 28, typeAlbum, nil, 0)
	msg(29, chatAlice, false, jidAlice, 29, typeImage, nil, 0)
	msg(30, chatAlice, true, nil, 30, typeText, "forwarded along", 0)
	msg(31, chatAlice, false, jidAlice, 31, typeText, nil, 0)
	msg(32, chatAlice, false, jidAlice, 32, typeNotDisplayed, nil, 0)
	msg(33, chatAlice, true, nil, 33, typeText, "replying to a photo", 0)

	exec(`INSERT INTO message_location (message_row_id, latitude, longitude, place_name, place_address, live_location_share_duration)
		VALUES (20, 41.3851, 2.1734, 'Sagrada Familia', 'Carrer de Mallorca', 0)`)
	exec(`INSERT INTO message_poll (message_row_id, selectable_options_count, end_time) VALUES (21, 1, 0)`)
	exec(`INSERT INTO message_poll_option (message_row_id, option_name, vote_total) VALUES
		(21, 'Pizza', 3), (21, 'Sushi', 1)`)
	exec(`INSERT INTO call_log (_id, jid_row_id, from_me, timestamp, video_call, duration, call_result)
		VALUES (1, ?, 0, ?, 1, 125, 5)`, jidAlice, tsBase)
	exec(`INSERT INTO message_call_log (message_row_id, call_log_row_id) VALUES (22, 1)`)
	exec(`INSERT INTO missed_call_logs (message_row_id, video_call) VALUES (23, 0)`)
	exec(`INSERT INTO message_text (message_row_id, url, page_title, description)
		VALUES (24, 'https://example.org/article', 'An article', 'What the article was about')`)
	exec(`INSERT INTO message_vcard (_id, message_row_id, vcard) VALUES
		(1, 25, 'BEGIN:VCARD' || char(10) || 'VERSION:3.0' || char(10) || 'FN:Dana Smith' || char(10) || 'END:VCARD')`)
	exec(`INSERT INTO message_vcard_jid (vcard_jid_row_id, vcard_row_id, message_row_id) VALUES (?, 1, 25)`, jidCarol)
	exec(`INSERT INTO message_ephemeral (message_row_id, duration) VALUES (26, 604800)`)
	exec(`INSERT INTO message_group_invite (message_row_id, group_jid_row_id, group_name, expiration)
		VALUES (27, ?, 'Book club', ?)`, jidGroup, tsBase+1000*tsStep)
	exec(`INSERT INTO message_album (message_row_id, image_count, video_count) VALUES (28, 3, 1)`)
	// A picture whose file is gone but whose preview survives in the database.
	exec(`INSERT INTO message_thumbnail (message_row_id, thumbnail) VALUES (29, ?)`, fakeJPEG())

	// A picture whose preview WhatsApp filed under the hash of the file rather than
	// against the message. On a real archive this is where two thirds of the
	// surviving pictures are, and no message that has one here has one above.
	msg(53, chatAlice, false, jidAlice, 53, typeImage, "", 0)
	exec(`INSERT INTO message_media (message_row_id, mime_type, file_hash, file_length)
		VALUES (53, 'image/jpeg', 'aGFzaC1vZi10aGUtZmlsZQ==', 61234)`)
	exec(`INSERT INTO media_hash_thumbnail (media_hash, thumbnail) VALUES ('aGFzaC1vZi10aGUtZmlsZQ==', ?)`, fakeJPEG())

	// A file whose size is recorded in the other of the two columns that hold it.
	msg(54, chatAlice, true, nil, 54, typeDocument, "", 0)
	exec(`INSERT INTO message_media (message_row_id, mime_type, media_name, file_length)
		VALUES (54, 'application/pdf', 'plan.pdf', 88000)`)
	exec(`INSERT INTO message_forwarded (message_row_id, forward_score) VALUES (30, 7)`)
	exec(`INSERT INTO message_revoked (message_row_id, admin_jid_row_id, revoke_timestamp)
		VALUES (31, ?, ?)`, jidAlice, tsBase+50*tsStep)
	exec(`INSERT INTO message_quoted (message_row_id, chat_row_id, from_me, sender_jid_row_id, message_type, text_data)
		VALUES (33, ?, 0, ?, ?, NULL)`, chatAlice, jidAlice, typeImage)
	exec(`INSERT INTO message_quoted_media (message_row_id, mime_type, media_name, media_caption, thumbnail)
		VALUES (33, 'image/jpeg', 'IMG-0002.jpg', 'the photo replied to', ?)`, fakeJPEG())

	// System notices, one per phrasing worth proving.
	notice := func(id int64, step int64, action int) {
		t.Helper()
		msg(id, chatGroup, false, jidAlice, step, typeSystem, nil, 0)
		exec(`INSERT INTO message_system (message_row_id, action_type) VALUES (?, ?)`, id, action)
	}
	notice(40, 40, 18) // the security code changed
	notice(41, 41, 12) // people were added
	exec(`INSERT INTO message_system_chat_participant (message_row_id, user_jid_row_id) VALUES (41, ?), (41, ?)`, jidCarol, jidHidden)
	notice(42, 42, 1) // the subject changed
	exec(`UPDATE message SET text_data = 'Weekend plans' WHERE _id = 42`)
	exec(`INSERT INTO message_system_value_change (message_row_id, old_data) VALUES (42, 'Old subject')`)
	notice(43, 43, 12) // the owner was added, which reads backwards if is_me_joined is ignored
	exec(`INSERT INTO message_system_group (message_row_id, is_me_joined) VALUES (43, 1)`)
	notice(44, 44, 57) // linked devices changed
	exec(`INSERT INTO message_system_device_change (message_row_id, device_added_count, device_removed_count) VALUES (44, 2, 1)`)
	notice(45, 45, 67)  // the encryption banner
	notice(46, 46, 111) // a code no source identifies
	notice(47, 47, 10)  // a contact changed their phone number
	exec(`INSERT INTO message_system_number_change (message_row_id, old_jid_row_id, new_jid_row_id)
		VALUES (47, ?, ?)`, jidAlice, jidCarol)
	notice(48, 48, 69) // a business notice
	exec(`INSERT INTO message_system_business_state (message_row_id, privacy_message_type, business_name)
		VALUES (48, 1, 'A shop')`)
	notice(49, 49, 58) // a contact was blocked
	exec(`INSERT INTO message_system_block_contact (message_row_id, is_blocked) VALUES (49, 1)`)
	notice(50, 50, 110) // a community change
	exec(`INSERT INTO message_system_with_group_nodes (message_row_id, group_subject) VALUES (50, 'Neighbours')`)
	notice(51, 51, 165) // the code the data names as a username change
	exec(`INSERT INTO message_system_username_change (message_row_id, old_username, new_username)
		VALUES (51, 'oldname', 'newname')`)

	// A group message whose sender column is empty, which real databases contain.
	msg(52, chatGroup, false, nil, 52, typeText, "sender not recorded", 0)

	if err := db.Close(); err != nil {
		t.Fatalf("closing the fixture: %v", err)
	}
	return path
}

// fakeJPEG is a byte sequence shaped like a small JPEG. The reader never decodes
// a preview, so its contents only have to be recognisable and stable.
func fakeJPEG() []byte {
	out := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	for i := range 64 {
		out = append(out, byte(i))
	}
	return append(out, 0xFF, 0xD9)
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
