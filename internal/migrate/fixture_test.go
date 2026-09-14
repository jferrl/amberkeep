package migrate

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"howett.net/plist"

	"github.com/jferrl/amberkeep/internal/model"
)

// The two sides of a migration, small enough to reason about.
//
// The iPhone store is written as a real SQLite file rather than mocked, because what
// is being tested is a decision made by reading one: a stand-in would prove that the
// planner agrees with the stand-in.

var (
	ana   = model.ParseJID("34600111222@s.whatsapp.net")
	luis  = model.ParseJID("34600333444@s.whatsapp.net")
	hiden = model.ParseJID("99887766554433@lid")
	group = model.ParseJID("120363001@g.us")
	start = time.Date(2019, 6, 14, 9, 0, 0, 0, time.UTC)
)

// The two addresses spelled out, for the tests that damage a store with SQL.
var (
	anaAddress  = ana.String()
	luisAddress = luis.String()
)

// phone is an iPhone store as a test builds one.
type phone struct {
	path     string
	sessions map[string]int64
	nextPK   int64
}

// buildPhone writes the smallest store this package will agree to write into.
func buildPhone(t *testing.T) *phone {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ChatStorage.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the store: %v", err)
	}
	defer func() { _ = db.Close() }()

	schema := `
CREATE TABLE Z_PRIMARYKEY (Z_ENT INTEGER PRIMARY KEY, Z_NAME VARCHAR, Z_SUPER INTEGER, Z_MAX INTEGER);
CREATE TABLE ZWACHATSESSION (
	Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER, ZSESSIONTYPE INTEGER,
	ZMESSAGECOUNTER INTEGER, ZREMOVED INTEGER, ZLASTMESSAGE INTEGER, ZLASTMESSAGEDATE TIMESTAMP,
	ZGROUPINFO INTEGER, ZCONTACTJID VARCHAR, ZPARTNERNAME VARCHAR, ZLASTMESSAGETEXT VARCHAR,
	ZFLAGS INTEGER, ZARCHIVED INTEGER, ZHIDDEN INTEGER, ZSPOTLIGHTSTATUS INTEGER,
	ZUNREADCOUNT INTEGER, ZCONTACTABID INTEGER, ZIDENTITYVERIFICATIONEPOCH INTEGER,
	ZIDENTITYVERIFICATIONSTATE INTEGER);
CREATE TABLE ZWAMESSAGE (
	Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER, ZCHATSESSION INTEGER,
	ZSORT INTEGER, ZISFROMME INTEGER, ZMESSAGETYPE INTEGER, ZMESSAGEDATE TIMESTAMP,
	ZSTANZAID VARCHAR, ZTEXT VARCHAR, ZFROMJID VARCHAR, ZTOJID VARCHAR, ZLASTSESSION INTEGER,
	ZGROUPMEMBER INTEGER, ZPUSHNAME VARCHAR, ZFLAGS INTEGER, ZMESSAGESTATUS INTEGER,
	ZSPOTLIGHTSTATUS INTEGER, ZSENTDATE TIMESTAMP, ZSTARRED INTEGER);
CREATE TABLE ZWAGROUPMEMBER (Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER,
	ZCHATSESSION INTEGER, ZMEMBERJID VARCHAR, ZCONTACTNAME VARCHAR, ZCONTACTABID INTEGER,
	ZISACTIVE INTEGER, ZISADMIN INTEGER);
CREATE TABLE ZWAGROUPINFO (Z_PK INTEGER PRIMARY KEY, Z_ENT INTEGER, Z_OPT INTEGER,
	ZSTATE INTEGER, ZCHATSESSION INTEGER, ZCREATIONDATE TIMESTAMP);
INSERT INTO Z_PRIMARYKEY (Z_ENT, Z_NAME, Z_SUPER, Z_MAX) VALUES
	(1,'WAChatSession',0,0), (2,'WAMessage',0,0), (3,'WAGroupMember',0,0), (4,'WAGroupInfo',0,0);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("creating the schema: %v", err)
	}
	return &phone{path: path, sessions: map[string]int64{}, nextPK: 1}
}

// holds puts a conversation on the phone, with messages already in it.
func (p *phone) holds(t *testing.T, address string, kind int, ids ...string) int64 {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+p.path)
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	defer func() { _ = db.Close() }()

	session := p.nextPK
	p.nextPK++
	if _, err := db.Exec(
		`INSERT INTO ZWACHATSESSION (Z_PK, Z_ENT, ZSESSIONTYPE, ZMESSAGECOUNTER, ZREMOVED, ZCONTACTJID)
		 VALUES (?, 1, ?, ?, 0, ?)`, session, kind, len(ids)+1, address); err != nil {
		t.Fatalf("adding a conversation: %v", err)
	}
	for i, id := range ids {
		if _, err := db.Exec(
			`INSERT INTO ZWAMESSAGE
			 (Z_PK, Z_ENT, ZCHATSESSION, ZSORT, ZISFROMME, ZMESSAGETYPE, ZFLAGS, ZMESSAGESTATUS,
			  ZMESSAGEDATE, ZSTANZAID, ZTEXT)
			 VALUES (?, 2, ?, ?, 0, 0, 16777216, 13, ?, ?, ?)`,
			p.nextPK, session, i+1, toCoreData(start.Add(time.Duration(i)*time.Minute)), id,
			"already here"); err != nil {
			t.Fatalf("adding a message: %v", err)
		}
		p.nextPK++
	}
	// A real store keeps Core Data's counter above its own rows. A fixture that did
	// not would be testing something that cannot happen.
	if _, err := db.Exec(
		`UPDATE Z_PRIMARYKEY SET Z_MAX = ? WHERE Z_NAME = 'WAChatSession'`, session); err != nil {
		t.Fatalf("updating the bookkeeping: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE Z_PRIMARYKEY SET Z_MAX = ? WHERE Z_NAME = 'WAMessage'`, p.nextPK-1); err != nil {
		t.Fatalf("updating the bookkeeping: %v", err)
	}

	p.sessions[address] = session
	return session
}

// pairs writes the file WhatsApp uses to remember which hidden identity is which
// number, so a conversation filed under one on this phone and the other on the
// Android can still be recognised as the same person.
func (p *phone) pairs(t *testing.T, lid, number string) string {
	t.Helper()

	path := filepath.Join(filepath.Dir(p.path), "LID.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the pairing file: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(
		`CREATE TABLE ZWAPHONENUMBERLIDPAIR (Z_PK INTEGER PRIMARY KEY, ZLID VARCHAR, ZPHONENUMBER VARCHAR);
		 INSERT INTO ZWAPHONENUMBERLIDPAIR (ZLID, ZPHONENUMBER) VALUES (?, ?)`, lid, number); err != nil {
		t.Fatalf("writing the pairing: %v", err)
	}
	return path
}

// open returns the store as a migration target.
func (p *phone) open(t *testing.T, pairing string) *Target {
	t.Helper()

	target, err := OpenTarget(context.Background(), p.path, pairing)
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })
	return target
}

// android is the history being moved, held in memory.
type android struct {
	chats     []model.Chat
	messages  map[int64][]model.Message
	directory *model.Directory
}

func (a *android) Chats(context.Context) ([]model.Chat, error) { return a.chats, nil }
func (a *android) Directory() *model.Directory                 { return a.directory }

func (a *android) Messages(_ context.Context, chat model.Chat) iter.Seq2[model.Message, error] {
	return func(yield func(model.Message, error) bool) {
		for _, m := range a.messages[chat.ID] {
			if !yield(m, nil) {
				return
			}
		}
	}
}

// said builds one ordinary message.
func said(id int64, key string, minutes int, text string) model.Message {
	return model.Message{
		ID: id, Key: key, Kind: model.KindText, Sender: ana, Text: text,
		SentAt: start.Add(time.Duration(minutes) * time.Minute),
	}
}

// sent builds one message of some other kind, which is how the untranslatable and
// the placeholders get into a fixture.
func sent(id int64, key string, minutes int, kind model.Kind) model.Message {
	m := said(id, key, minutes, "")
	m.Kind = kind
	if kind == model.KindImage {
		m.Attachment = &model.Attachment{}
	}
	return m
}

// history builds an Android side with one direct conversation of n messages.
func history(messages ...model.Message) *android {
	directory := model.NewDirectory()
	directory.Add(model.Contact{JID: ana, Name: "Ana Lopez"})
	directory.Add(model.Contact{JID: luis, Name: "Luis"})

	chat := model.Chat{
		ID: 1, JID: ana, Kind: model.ChatDirect, Name: "Ana Lopez",
		Messages: len(messages), LastAt: start.Add(time.Hour),
	}
	return &android{
		chats:     []model.Chat{chat},
		messages:  map[int64][]model.Message{1: messages},
		directory: directory,
	}
}

// with adds another conversation to an Android side.
func (a *android) with(chat model.Chat, messages ...model.Message) *android {
	chat.Messages = len(messages)
	a.chats = append(a.chats, chat)
	a.messages[chat.ID] = messages
	return a
}

// accounted checks the promise the whole report rests on: every message in the
// source is counted once and once only.
func accounted(t *testing.T, plan Plan, total int) {
	t.Helper()

	if got := plan.Adding + plan.AlreadyThere + plan.Untranslatable; got != total {
		t.Errorf("the plan accounts for %d messages, and the source holds %d: %s",
			got, total, fmt.Sprintf("adding=%d already=%d untranslatable=%d",
				plan.Adding, plan.AlreadyThere, plan.Untranslatable))
	}
}

// writeStore puts an arbitrary schema on disk, for the cases where the point is
// that this package refuses it.
func writeStore(t *testing.T, schema string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ChatStorage.sqlite")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("creating the store: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("creating the schema: %v", err)
	}
	return path
}

// errorIs is errors.Is, named so the tests read as sentences.
func errorIs(err, want error) bool { return errors.Is(err, want) }

// wal puts the store into the mode WhatsApp's own is in, and leaves no log beside
// it. Both are things the checks look at, so a fixture that skipped them would make
// two of them untestable.
func (p *phone) wal(t *testing.T) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+p.path)
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("switching to write-ahead logging: %v", err)
	}
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatalf("folding the log back in: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the store: %v", err)
	}
	for _, side := range []string{"-wal", "-shm"} {
		_ = os.Remove(p.path + side)
	}
}

// migrated hand-builds what a correct migration of a store looks like: the original
// untouched, plus one conversation added with its numbering, counters and pointers
// all as they have to be.
//
// Built by hand rather than by the thing that will do it for real, because the
// checks have to be tested before the writer exists — and because a fixture written
// independently is the only way "correct" means something other than "whatever the
// writer produced".
func migrated(t *testing.T) (original, result string, plan Plan) {
	t.Helper()

	p := buildPhone(t)
	existing := p.holds(t, luis.String(), 0, "OLD1", "OLD2")
	p.wal(t)

	original = filepath.Join(t.TempDir(), "original.sqlite")
	copyFile(t, p.path, original)

	db, err := sql.Open("sqlite", "file:"+p.path)
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	defer func() { _ = db.Close() }()

	// A conversation that did not exist before, with two messages in it.
	session := p.nextPK
	p.nextPK++
	exec(t, db, `INSERT INTO ZWACHATSESSION
		(Z_PK, Z_ENT, ZSESSIONTYPE, ZMESSAGECOUNTER, ZREMOVED, ZCONTACTJID, ZLASTMESSAGE, ZLASTMESSAGEDATE)
		VALUES (?, 1, 0, 3, 0, ?, ?, ?)`, session, ana.String(), p.nextPK+1, 1.0)

	for i, id := range []string{"NEW1", "NEW2"} {
		exec(t, db, `INSERT INTO ZWAMESSAGE
			(Z_PK, Z_ENT, ZCHATSESSION, ZSORT, ZISFROMME, ZMESSAGEDATE, ZSTANZAID, ZTEXT, ZFROMJID, ZLASTSESSION)
			VALUES (?, 2, ?, ?, 0, ?, ?, ?, ?, ?)`,
			p.nextPK, session, i+1, float64(i), id, "carried across", ana.String(),
			nullIfNotLast(i, 1, session))
		p.nextPK++
	}

	exec(t, db, `UPDATE Z_PRIMARYKEY SET Z_MAX = ? WHERE Z_NAME = 'WAChatSession'`, session)
	exec(t, db, `UPDATE Z_PRIMARYKEY SET Z_MAX = ? WHERE Z_NAME = 'WAMessage'`, p.nextPK-1)
	if err := db.Close(); err != nil {
		t.Fatalf("closing the store: %v", err)
	}
	p.wal(t)

	plan = Plan{Conversations: []Conversation{
		{Address: ana.String(), Adding: 2},
		{Address: luis.String(), Into: luis.String(), Session: existing, Adding: 0},
	}}
	return original, p.path, plan
}

// nullIfNotLast marks only the final message of a conversation as the one the
// session's own pointer comes back to.
func nullIfNotLast(i, last int, session int64) any {
	if i == last {
		return session
	}
	return nil
}

func exec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("preparing the fixture: %v\n%s", err, query)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatalf("copying the fixture: %v", err)
	}
	if err := os.WriteFile(to, data, 0o600); err != nil {
		t.Fatalf("copying the fixture: %v", err)
	}
}

// damage applies one deliberate fault to a store, so a check can be tested by
// watching it fail.
func damage(t *testing.T, path, statement string) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("opening the store to damage it: %v", err)
	}
	exec(t, db, statement)
	if err := db.Close(); err != nil {
		t.Fatalf("closing the store: %v", err)
	}
	for _, side := range []string{"-wal", "-shm"} {
		_ = os.Remove(path + side)
	}
}

// backupHolding writes the smallest folder backupfs will agree is an iPhone backup,
// recording one file in it. Enough for the checks that run before anything else.
func backupHolding(t *testing.T, domain, relativePath string, size int64, when time.Time) string {
	t.Helper()

	dir := t.TempDir()
	plists := map[string]any{
		"Manifest.plist": map[string]any{"IsEncrypted": false},
		"Info.plist": map[string]any{
			"Device Name": "A Test Phone", "Product Type": "iPhone14,2",
			"Product Version": "26.0", "Last Backup Date": when,
		},
		"Status.plist": map[string]any{"IsFullBackup": true},
	}
	for name, body := range plists {
		data, err := plist.Marshal(body, plist.BinaryFormat)
		if err != nil {
			t.Fatalf("building %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "Manifest.db"))
	if err != nil {
		t.Fatalf("creating the index: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(
		`CREATE TABLE Files (fileID TEXT PRIMARY KEY, domain TEXT, relativePath TEXT, flags INTEGER, file BLOB)`,
	); err != nil {
		t.Fatalf("creating the Files table: %v", err)
	}

	sum := sha1.Sum([]byte(domain + "-" + relativePath)) // #nosec G401 -- Apple's own naming scheme
	if _, err := db.Exec(
		`INSERT INTO Files (fileID, domain, relativePath, flags, file) VALUES (?, ?, ?, 1, ?)`,
		hex.EncodeToString(sum[:]), domain, relativePath, mbFile(t, size)); err != nil {
		t.Fatalf("recording the file: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the index: %v", err)
	}
	return dir
}

// mbFile builds what Apple stores against a file: its metadata as a keyed archive.
func mbFile(t *testing.T, size int64) []byte {
	t.Helper()

	data, err := plist.Marshal(map[string]any{
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
	}, plist.BinaryFormat)
	if err != nil {
		t.Fatalf("building the file's metadata: %v", err)
	}
	return data
}
