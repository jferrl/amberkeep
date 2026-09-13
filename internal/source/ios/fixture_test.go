package ios

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// The fixture is a real Core Data store built from the schema a real iPhone had,
// with invented content. No message anybody actually sent appears in this
// repository, here or anywhere else.

// The identifiers the tests refer to.
const (
	sessionAna   = 1
	sessionGroup = 2
	sessionEmpty = 3
	sessionMuted = 4 // a status feed, which an archive leaves out
)

// Addresses. The numbers are invented and belong to nobody.
const (
	anaJID   = "34600111222@s.whatsapp.net"
	luisJID  = "34600333444@s.whatsapp.net"
	groupJID = "120363001@g.us"
	hiddenID = "192837465564738@lid"
)

// fixtureSchema is the shape a real store has, trimmed to the tables a reader
// touches. The column lists are copied from a real device so that a test proves
// the reader works against what WhatsApp actually writes.
const fixtureSchema = `
CREATE TABLE ZWACHATSESSION (
	Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER,
	ZARCHIVED INTEGER, ZHIDDEN INTEGER, ZMESSAGECOUNTER INTEGER, ZSESSIONTYPE INTEGER,
	ZGROUPINFO INTEGER, ZLASTMESSAGE INTEGER,
	ZLASTMESSAGEDATE TIMESTAMP, ZCONTACTJID VARCHAR, ZPARTNERNAME VARCHAR, ZLASTMESSAGETEXT VARCHAR);

CREATE TABLE ZWAMESSAGE (
	Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER,
	ZGROUPEVENTTYPE INTEGER, ZISFROMME INTEGER, ZMESSAGESTATUS INTEGER, ZMESSAGETYPE INTEGER,
	ZSORT INTEGER, ZSTARRED INTEGER, ZFLAGS INTEGER,
	ZCHATSESSION INTEGER, ZGROUPMEMBER INTEGER, ZMEDIAITEM INTEGER, ZPARENTMESSAGE INTEGER,
	ZMESSAGEDATE TIMESTAMP, ZFROMJID VARCHAR, ZPUSHNAME VARCHAR, ZSTANZAID VARCHAR,
	ZTEXT VARCHAR, ZTOJID VARCHAR);

CREATE TABLE ZWAMEDIAITEM (
	Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER,
	ZFILESIZE INTEGER, ZMOVIEDURATION INTEGER, ZMESSAGE INTEGER,
	ZLATITUDE FLOAT, ZLONGITUDE FLOAT,
	ZMEDIALOCALPATH VARCHAR, ZTITLE VARCHAR, ZVCARDNAME VARCHAR, ZVCARDSTRING VARCHAR,
	ZXMPPTHUMBPATH VARCHAR, ZMETADATA BLOB);

CREATE TABLE ZWAGROUPMEMBER (
	Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER,
	ZISADMIN INTEGER, ZCHATSESSION INTEGER, ZCONTACTNAME VARCHAR, ZFIRSTNAME VARCHAR, ZMEMBERJID VARCHAR);

CREATE TABLE ZWAGROUPINFO (
	Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER,
	ZCHATSESSION INTEGER, ZCREATIONDATE TIMESTAMP, ZCREATORJID VARCHAR, ZSUBJECTOWNERJID VARCHAR);

CREATE TABLE ZWAPROFILEPUSHNAME (
	Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER, ZJID VARCHAR, ZPUSHNAME VARCHAR);

CREATE TABLE ZWAMESSAGEDATAITEM (
	Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER, ZTYPE INTEGER, ZMESSAGE INTEGER,
	ZCONTENT1 VARCHAR, ZMATCHEDTEXT VARCHAR, ZSUMMARY VARCHAR, ZTITLE VARCHAR);

CREATE INDEX Z_WAMessage_compoundIndex ON ZWAMESSAGE (ZCHATSESSION, ZSORT);
`

// at converts a plain date into the store's own count of seconds since 2001.
func at(year int, month time.Month, day, hour, minute int) float64 {
	t := time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
	return t.Sub(appleEpoch).Seconds()
}

// buildFixture writes a store and returns its path.
func buildFixture(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ChatStorage.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the store: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(fixtureSchema); err != nil {
		t.Fatalf("creating the schema: %v", err)
	}

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("populating the store: %v\n%s", err, query)
		}
	}

	// Conversations. The message counter is deliberately wrong, as it is on a real
	// device, so a test proves the reader counts for itself.
	exec(`INSERT INTO ZWACHATSESSION (Z_PK, ZSESSIONTYPE, ZCONTACTJID, ZPARTNERNAME, ZLASTMESSAGEDATE, ZMESSAGECOUNTER, ZARCHIVED)
	      VALUES (?, 0, ?, 'Ana Lopez', ?, 99, 0)`, sessionAna, anaJID, at(2019, 6, 14, 9, 40))
	exec(`INSERT INTO ZWACHATSESSION (Z_PK, ZSESSIONTYPE, ZCONTACTJID, ZPARTNERNAME, ZLASTMESSAGEDATE, ZMESSAGECOUNTER, ZARCHIVED)
	      VALUES (?, 1, ?, 'Vermut del sabado', ?, 0, 1)`, sessionGroup, groupJID, at(2019, 6, 15, 12, 0))
	exec(`INSERT INTO ZWACHATSESSION (Z_PK, ZSESSIONTYPE, ZCONTACTJID, ZPARTNERNAME, ZMESSAGECOUNTER)
	      VALUES (?, 0, ?, 'Luis', 0)`, sessionEmpty, luisJID)
	exec(`INSERT INTO ZWACHATSESSION (Z_PK, ZSESSIONTYPE, ZCONTACTJID, ZPARTNERNAME, ZMESSAGECOUNTER)
	      VALUES (?, 3, '34600555666@status', '34600555666', 0)`, sessionMuted)

	exec(`INSERT INTO ZWAGROUPINFO (Z_PK, ZCHATSESSION, ZCREATIONDATE, ZCREATORJID)
	      VALUES (1, ?, ?, ?)`, sessionGroup, at(2017, 3, 2, 10, 0), anaJID)

	// Group members. The saved name beats the one somebody chose for themselves.
	exec(`INSERT INTO ZWAGROUPMEMBER (Z_PK, ZCHATSESSION, ZMEMBERJID, ZCONTACTNAME, ZISADMIN) VALUES (1, ?, ?, 'Ana Lopez', 1)`, sessionGroup, anaJID)
	exec(`INSERT INTO ZWAGROUPMEMBER (Z_PK, ZCHATSESSION, ZMEMBERJID, ZCONTACTNAME, ZISADMIN) VALUES (2, ?, ?, 'Luis', 0)`, sessionGroup, luisJID)
	exec(`INSERT INTO ZWAGROUPMEMBER (Z_PK, ZCHATSESSION, ZMEMBERJID, ZFIRSTNAME, ZISADMIN) VALUES (3, ?, ?, 'Marta', 0)`, sessionGroup, hiddenID)

	exec(`INSERT INTO ZWAPROFILEPUSHNAME (Z_PK, ZJID, ZPUSHNAME) VALUES (1, ?, 'Anita')`, anaJID)
	exec(`INSERT INTO ZWAPROFILEPUSHNAME (Z_PK, ZJID, ZPUSHNAME) VALUES (2, ?, 'Marta R')`, hiddenID)

	// A one-to-one conversation: one each way, then a reply, then a photograph.
	exec(`INSERT INTO ZWAMESSAGE (Z_PK, ZCHATSESSION, ZISFROMME, ZMESSAGETYPE, ZSORT, ZMESSAGEDATE, ZTEXT, ZSTANZAID, ZFROMJID)
	      VALUES (1, ?, 0, 0, 100, ?, 'are you up?', 'AAAA1111BBBB2222', ?)`, sessionAna, at(2019, 6, 14, 9, 12), anaJID)
	exec(`INSERT INTO ZWAMESSAGE (Z_PK, ZCHATSESSION, ZISFROMME, ZMESSAGETYPE, ZSORT, ZMESSAGEDATE, ZTEXT, ZSTANZAID, ZSTARRED)
	      VALUES (2, ?, 1, 0, 200, ?, 'just about', 'CCCC3333DDDD4444', 1)`, sessionAna, at(2019, 6, 14, 9, 16))

	// A reply. What it answers lives in the protobuf beside it, never in
	// ZPARENTMESSAGE, which is exactly what a real store does.
	exec(`INSERT INTO ZWAMEDIAITEM (Z_PK, ZMESSAGE, ZMETADATA) VALUES (10, 3, ?)`,
		replyContext("AAAA1111BBBB2222", anaJID, "are you up?"))
	exec(`INSERT INTO ZWAMESSAGE (Z_PK, ZCHATSESSION, ZISFROMME, ZMESSAGETYPE, ZSORT, ZMESSAGEDATE, ZTEXT, ZSTANZAID, ZMEDIAITEM)
	      VALUES (3, ?, 1, 0, 300, ?, 'barely', 'EEEE5555FFFF6666', 10)`, sessionAna, at(2019, 6, 14, 9, 20))

	exec(`INSERT INTO ZWAMEDIAITEM (Z_PK, ZMESSAGE, ZVCARDSTRING, ZTITLE, ZFILESIZE, ZMEDIALOCALPATH)
	      VALUES (11, 4, 'image/jpeg', NULL, 51234, 'Media/34600111222/7/IMG-0007.jpg')`)
	exec(`INSERT INTO ZWAMESSAGE (Z_PK, ZCHATSESSION, ZISFROMME, ZMESSAGETYPE, ZSORT, ZMESSAGEDATE, ZTEXT, ZSTANZAID, ZMEDIAITEM)
	      VALUES (4, ?, 0, 1, 400, ?, 'the beach', 'AAAA7777BBBB8888', 11)`, sessionAna, at(2019, 6, 14, 9, 40), anaJID)

	// A group conversation: a member's message, a shared place, a card, a document
	// this build has no number for, a deletion, and a notice.
	exec(`INSERT INTO ZWAMESSAGE (Z_PK, ZCHATSESSION, ZISFROMME, ZMESSAGETYPE, ZSORT, ZMESSAGEDATE, ZTEXT, ZSTANZAID, ZGROUPMEMBER, ZFROMJID, ZPUSHNAME)
	      VALUES (5, ?, 0, 0, 100, ?, 'quien se apunta', 'BBBB1111CCCC2222', 2, ?, 'Luisito')`,
		sessionGroup, at(2019, 6, 15, 11, 0), groupJID)

	exec(`INSERT INTO ZWAMEDIAITEM (Z_PK, ZMESSAGE, ZLATITUDE, ZLONGITUDE, ZTITLE) VALUES (12, 6, 41.38506, 2.1734, 'Casa Blanca')`)
	exec(`INSERT INTO ZWAMESSAGE (Z_PK, ZCHATSESSION, ZISFROMME, ZMESSAGETYPE, ZSORT, ZMESSAGEDATE, ZSTANZAID, ZGROUPMEMBER, ZFROMJID, ZMEDIAITEM)
	      VALUES (6, ?, 0, 5, 200, ?, 'BBBB3333CCCC4444', 1, ?, 12)`, sessionGroup, at(2019, 6, 15, 11, 10), groupJID)

	exec(`INSERT INTO ZWAMEDIAITEM (Z_PK, ZMESSAGE, ZVCARDNAME, ZVCARDSTRING) VALUES (13, 7, 'Marta Ruiz', ?)`,
		"BEGIN:VCARD\nVERSION:3.0\nFN:Marta Ruiz\nTEL:+34600999888\nEND:VCARD")
	exec(`INSERT INTO ZWAMESSAGE (Z_PK, ZCHATSESSION, ZISFROMME, ZMESSAGETYPE, ZSORT, ZMESSAGEDATE, ZSTANZAID, ZMEDIAITEM)
	      VALUES (7, ?, 1, 4, 300, ?, 'BBBB5555CCCC6666', 13)`, sessionGroup, at(2019, 6, 15, 11, 20))

	// A type number this build does not know, carrying a media type that says what
	// it is anyway. This is the case that proves the fallback works.
	exec(`INSERT INTO ZWAMEDIAITEM (Z_PK, ZMESSAGE, ZVCARDSTRING, ZTITLE, ZFILESIZE) VALUES (14, 8, 'application/pdf', 'plan.pdf', 88000)`)
	exec(`INSERT INTO ZWAMESSAGE (Z_PK, ZCHATSESSION, ZISFROMME, ZMESSAGETYPE, ZSORT, ZMESSAGEDATE, ZSTANZAID, ZMEDIAITEM)
	      VALUES (8, ?, 1, 91, 400, ?, 'BBBB7777CCCC8888', 14)`, sessionGroup, at(2019, 6, 15, 11, 30))

	// A deletion, with the address of whoever removed it where a media type would be.
	exec(`INSERT INTO ZWAMEDIAITEM (Z_PK, ZMESSAGE, ZVCARDSTRING, ZTITLE) VALUES (15, 9, ?, 'deleted')`, anaJID)
	exec(`INSERT INTO ZWAMESSAGE (Z_PK, ZCHATSESSION, ZISFROMME, ZMESSAGETYPE, ZSORT, ZMESSAGEDATE, ZSTANZAID, ZGROUPMEMBER, ZFROMJID, ZMEDIAITEM)
	      VALUES (9, ?, 0, 14, 500, ?, 'BBBB9999CCCC0000', 2, ?, 15)`, sessionGroup, at(2019, 6, 15, 11, 40), groupJID)

	// A notice the store wrote itself.
	exec(`INSERT INTO ZWAMESSAGE (Z_PK, ZCHATSESSION, ZISFROMME, ZMESSAGETYPE, ZGROUPEVENTTYPE, ZSORT, ZMESSAGEDATE, ZTEXT, ZSTANZAID)
	      VALUES (10, ?, 0, 6, 12, 600, ?, 'Luis added Marta', 'CCCC1111DDDD2222')`, sessionGroup, at(2019, 6, 15, 12, 0))

	// A link, whose preview lives in its own table because a message may carry more
	// than one and joining them would double the message.
	exec(`INSERT INTO ZWAMESSAGE (Z_PK, ZCHATSESSION, ZISFROMME, ZMESSAGETYPE, ZSORT, ZMESSAGEDATE, ZTEXT, ZSTANZAID)
	      VALUES (11, ?, 1, 7, 700, ?, 'look https://example.org/casa', 'DDDD1111EEEE2222')`,
		sessionGroup, at(2019, 6, 15, 12, 10))
	exec(`INSERT INTO ZWAMESSAGEDATAITEM (Z_PK, ZTYPE, ZMESSAGE, ZTITLE, ZSUMMARY, ZMATCHEDTEXT)
	      VALUES (1, 0, 11, 'Casa Blanca', 'A small restaurant', 'https://example.org/casa')`)

	// A status update, which an archive leaves out.
	exec(`INSERT INTO ZWAMESSAGE (Z_PK, ZCHATSESSION, ZISFROMME, ZMESSAGETYPE, ZSORT, ZMESSAGEDATE, ZTEXT, ZSTANZAID)
	      VALUES (12, ?, 0, 0, 100, ?, 'a status nobody archives', 'EEEE1111FFFF2222')`,
		sessionMuted, at(2019, 6, 16, 8, 0))

	if err := db.Close(); err != nil {
		t.Fatalf("closing the store: %v", err)
	}
	return path
}

// replyContext builds the protobuf a real store puts beside a reply.
func replyContext(stanza, sender, quoted string) []byte {
	var out []byte
	out = appendString(out, fieldQuotedID, stanza)
	out = appendString(out, fieldQuotedSender, sender)

	var body []byte
	body = appendString(body, fieldQuotedText, quoted)
	out = appendBytes(out, fieldQuotedBody, body)
	return out
}

func appendString(dst []byte, field int, value string) []byte {
	return appendBytes(dst, field, []byte(value))
}

func appendBytes(dst []byte, field int, value []byte) []byte {
	dst = appendVarint(dst, uint64(field)<<3|2)
	dst = appendVarint(dst, uint64(len(value)))
	return append(dst, value...)
}

func appendVarint(dst []byte, v uint64) []byte {
	for v >= 0x80 {
		dst = append(dst, byte(v)|0x80)
		v >>= 7
	}
	return append(dst, byte(v))
}

// openFixture opens a freshly built store.
func openFixture(t *testing.T) *Reader {
	t.Helper()

	r, err := Open(context.Background(), buildFixture(t))
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// buildBare writes a database with the given schema and nothing in it, for the
// tests about refusing what is not a message store.
func buildBare(t *testing.T, schema string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "other.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// A statement has to run for SQLite to create the file at all, and the tests
	// need a database that exists and holds nothing.
	if schema == "" {
		schema = `CREATE TABLE placeholder (x); DROP TABLE placeholder;`
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("creating the schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the database: %v", err)
	}
	return path
}

// ensure the unused-parameter warning cannot hide a mistake in the helper above.
var _ = fmt.Sprintf
