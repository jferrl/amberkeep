// Package fixture builds the small worlds this program's tests need.
//
// It is here rather than beside one package's tests because two suites need the same
// backup and the same archive, and a Go test helper cannot cross a package boundary
// unless it lives somewhere both can import. Nothing here is linked into the program:
// only tests import it.
//
// Everything it writes is invented and belongs to nobody. No real message, number or
// name enters this repository, which is the rule the whole project is built on.
package fixture

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"howett.net/plist"
)

// whatsAppDomain is Apple's name for the group container WhatsApp shares with its
// extensions. Written out here rather than imported so that a fixture cannot drift
// into agreeing with a bug in the code it is testing.
const (
	whatsAppDomain = "AppDomainGroup-group.net.whatsapp.WhatsApp.shared"
	chatStorage    = "ChatStorage.sqlite"
)

// BackupWithAStore writes the smallest thing Apple's tools would recognise as
// a backup, holding a WhatsApp message store.
func BackupWithAStore(t *testing.T, parent string) string {
	t.Helper()

	backup := filepath.Join(parent, "00008030-000000000000000E")
	if err := os.MkdirAll(backup, 0o700); err != nil {
		t.Fatalf("creating the backup folder: %v", err)
	}

	manifest, err := sql.Open("sqlite", "file:"+filepath.Join(backup, "Manifest.db"))
	if err != nil {
		t.Fatalf("creating the manifest: %v", err)
	}
	if _, err := manifest.Exec(
		`CREATE TABLE Files (fileID TEXT PRIMARY KEY, domain TEXT, relativePath TEXT, flags INTEGER, file BLOB)`); err != nil {
		t.Fatalf("creating the Files table: %v", err)
	}

	// file puts one file into the backup the way Apple does: the bytes under a hash
	// of where they belong, and a row in the index saying what that hash means.
	file := func(relativePath string, contents []byte) {
		t.Helper()

		sum := sha1.Sum([]byte(whatsAppDomain + "-" + relativePath)) // #nosec G401 -- Apple's own naming scheme
		fileID := hex.EncodeToString(sum[:])

		shard := filepath.Join(backup, fileID[:2])
		if err := os.MkdirAll(shard, 0o700); err != nil {
			t.Fatalf("creating the shard folder: %v", err)
		}
		if err := os.WriteFile(filepath.Join(shard, fileID), contents, 0o600); err != nil {
			t.Fatalf("writing the stored file: %v", err)
		}
		if _, err := manifest.Exec(
			`INSERT INTO Files (fileID, domain, relativePath, flags, file) VALUES (?, ?, ?, 1, ?)`,
			fileID, whatsAppDomain, relativePath, mbFileRecord(t, int64(len(contents)))); err != nil {
			t.Fatalf("recording %s: %v", relativePath, err)
		}
	}

	store := filepath.Join(parent, "ChatStorage.sqlite")
	buildTinyStore(t, store)
	contents, err := os.ReadFile(store)
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}
	file(chatStorage, contents)

	// A picture, filed the way a backup files one. The store records it as
	// "Media/..." and the backup keeps it one directory further in, under Message/,
	// which is the offset nothing documents and everything depends on.
	file("Message/"+ThumbInStore, OnePixelJPEG)

	if err := manifest.Close(); err != nil {
		t.Fatalf("closing the manifest: %v", err)
	}

	plists := map[string]string{
		"Manifest.plist": `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>IsEncrypted</key><false/></dict></plist>`,
		"Info.plist": `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Device Name</key><string>A Test Phone</string>
<key>Product Type</key><string>iPhone14,2</string>
<key>Product Version</key><string>26.0</string>
</dict></plist>`,
		"Status.plist": `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>IsFullBackup</key><true/></dict></plist>`,
	}
	for name, body := range plists {
		if err := os.WriteFile(filepath.Join(backup, name), []byte(body), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	return backup
}

// ThumbInStore is where the store says its one picture is, and OnePixelJPEG is what
// is actually there. What it depicts does not matter; that it arrives does.
const ThumbInStore = "Media/a/b/one.thumb"

// OnePixelJPEG is the smallest thing a reader will accept as a photograph. What it
// depicts does not matter and it depicts nothing: it exists so that a test can watch
// a picture travel, without a real one entering this repository.
var OnePixelJPEG = []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0xff, 0xd9}

// TinyArchive writes the smallest database the reader will accept, with two
// messages in one conversation.
func TinyArchive(t *testing.T, path string) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the database: %v", err)
	}
	defer db.Close()

	schema := `
CREATE TABLE jid (_id INTEGER PRIMARY KEY, user TEXT, server TEXT, raw_string TEXT);
CREATE TABLE chat (_id INTEGER PRIMARY KEY, jid_row_id INTEGER, subject TEXT);
CREATE TABLE message (
	_id INTEGER PRIMARY KEY, chat_row_id INTEGER, from_me INTEGER, key_id TEXT,
	sender_jid_row_id INTEGER, timestamp INTEGER, message_type INTEGER, text_data TEXT);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("creating the schema: %v", err)
	}

	at := time.Date(2026, 9, 12, 18, 46, 9, 0, time.UTC).UnixMilli()
	data := fmt.Sprintf(`
INSERT INTO jid (_id, user, server, raw_string)
	VALUES (1, '34600111222', 's.whatsapp.net', '34600111222@s.whatsapp.net');
INSERT INTO chat (_id, jid_row_id) VALUES (1, 1);
INSERT INTO message (_id, chat_row_id, from_me, key_id, sender_jid_row_id, timestamp, message_type, text_data)
	VALUES (1, 1, 0, 'K1', 1, %d, 0, 'hello there'),
	       (2, 1, 1, 'K2', NULL, %d, 0, 'hello back');`, at, at+60000)
	if _, err := db.Exec(data); err != nil {
		t.Fatalf("populating the database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the database: %v", err)
	}
}

// mbFileRecord builds what Apple stores in a Files row: the file's metadata as a
// keyed archive, in the flat UID-referenced shape NSKeyedArchiver produces.
func mbFileRecord(t *testing.T, size int64) []byte {
	t.Helper()

	document := map[string]any{
		"$archiver": "NSKeyedArchiver",
		"$version":  uint64(100000),
		"$top":      map[string]any{"root": plist.UID(1)},
		"$objects": []any{
			"$null",
			map[string]any{
				"$class": plist.UID(2),
				"Size":   uint64(size), //nolint:gosec // a fixture size is never negative
				"Mode":   uint64(0o100644),
			},
			map[string]any{"$classname": "MBFile", "$classes": []any{"MBFile", "NSObject"}},
		},
	}
	data, err := plist.Marshal(document, plist.BinaryFormat)
	if err != nil {
		t.Fatalf("building a file record: %v", err)
	}
	return data
}

// buildTinyStore writes the smallest iPhone message store the reader will accept.
func buildTinyStore(t *testing.T, path string) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the store: %v", err)
	}
	defer func() { _ = db.Close() }()

	// The shape a real store has, not the smallest one a reader will accept. Reading
	// forgives a missing column; writing must not, so a fixture that only satisfies
	// the reader cannot be used to test a migration at all.
	if _, err := db.Exec(`
CREATE TABLE Z_PRIMARYKEY (Z_ENT INTEGER PRIMARY KEY, Z_NAME VARCHAR, Z_SUPER INTEGER, Z_MAX INTEGER);
CREATE TABLE ZWACHATSESSION (Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER,
	ZSESSIONTYPE INTEGER, ZCONTACTJID VARCHAR, ZPARTNERNAME VARCHAR, ZLASTMESSAGEDATE TIMESTAMP,
	ZARCHIVED INTEGER, ZMESSAGECOUNTER INTEGER, ZREMOVED INTEGER, ZLASTMESSAGE INTEGER,
	ZLASTMESSAGETEXT VARCHAR, ZGROUPINFO INTEGER, ZFLAGS INTEGER, ZHIDDEN INTEGER,
	ZSPOTLIGHTSTATUS INTEGER, ZUNREADCOUNT INTEGER, ZCONTACTABID INTEGER,
	ZIDENTITYVERIFICATIONEPOCH INTEGER, ZIDENTITYVERIFICATIONSTATE INTEGER);
CREATE TABLE ZWAMESSAGE (Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER,
	ZCHATSESSION INTEGER, ZISFROMME INTEGER, ZMESSAGETYPE INTEGER, ZSORT INTEGER,
	ZMESSAGEDATE TIMESTAMP, ZSENTDATE TIMESTAMP, ZTEXT VARCHAR, ZSTANZAID VARCHAR,
	ZMEDIAITEM INTEGER, ZFROMJID VARCHAR, ZTOJID VARCHAR, ZPUSHNAME VARCHAR,
	ZGROUPMEMBER INTEGER, ZLASTSESSION INTEGER, ZFLAGS INTEGER, ZMESSAGESTATUS INTEGER,
	ZSPOTLIGHTSTATUS INTEGER, ZSTARRED INTEGER);
CREATE TABLE ZWAMEDIAITEM (Z_PK INTEGER PRIMARY KEY, ZMESSAGE INTEGER, ZVCARDSTRING VARCHAR,
	ZXMPPTHUMBPATH VARCHAR, ZFILESIZE INTEGER);
CREATE TABLE ZWAGROUPMEMBER (Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER,
	ZCHATSESSION INTEGER, ZMEMBERJID VARCHAR, ZCONTACTNAME VARCHAR, ZCONTACTABID INTEGER,
	ZISACTIVE INTEGER, ZISADMIN INTEGER);
CREATE TABLE ZWAGROUPINFO (Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER,
	ZSTATE INTEGER, ZCHATSESSION INTEGER, ZCREATIONDATE TIMESTAMP);
INSERT INTO Z_PRIMARYKEY (Z_ENT, Z_NAME, Z_SUPER, Z_MAX) VALUES
	(1,'WAChatSession',0,1), (2,'WAMessage',0,2), (3,'WAGroupMember',0,0), (4,'WAGroupInfo',0,0);
INSERT INTO ZWACHATSESSION (Z_PK, Z_ENT, ZSESSIONTYPE, ZCONTACTJID, ZPARTNERNAME,
	ZLASTMESSAGEDATE, ZARCHIVED, ZMESSAGECOUNTER, ZREMOVED, ZLASTMESSAGE)
	VALUES (1, 1, 0, '34600111222@s.whatsapp.net', 'Ana Lopez', 580000000, 0, 3, 0, 2);
INSERT INTO ZWAMESSAGE (Z_PK, Z_ENT, ZCHATSESSION, ZISFROMME, ZMESSAGETYPE, ZSORT,
	ZMESSAGEDATE, ZTEXT, ZSTANZAID, ZMEDIAITEM, ZFROMJID, ZFLAGS, ZMESSAGESTATUS)
	VALUES (1, 2, 1, 0, 0, 1, 580000000, 'hello there', 'AAAA1111BBBB2222', NULL,
		'34600111222@s.whatsapp.net', 16777216, 13);
INSERT INTO ZWAMEDIAITEM VALUES (1, 2, 'image/jpeg', '` + ThumbInStore + `', 12);
INSERT INTO ZWAMESSAGE (Z_PK, Z_ENT, ZCHATSESSION, ZISFROMME, ZMESSAGETYPE, ZSORT,
	ZMESSAGEDATE, ZTEXT, ZSTANZAID, ZMEDIAITEM, ZFROMJID, ZFLAGS, ZMESSAGESTATUS, ZLASTSESSION)
	VALUES (2, 2, 1, 0, 1, 2, 580000100, 'look at this', 'AAAA3333BBBB4444', 1,
		'34600111222@s.whatsapp.net', 16777216, 13, 1);`); err != nil {
		t.Fatalf("populating the store: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the store: %v", err)
	}
}
