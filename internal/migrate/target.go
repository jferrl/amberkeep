package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	_ "modernc.org/sqlite" // registers the pure-Go SQLite driver
)

// Target is an iPhone message store seen as somewhere a history could be put.
//
// It is opened read-only and stays that way: this type answers questions and never
// writes. What it is asked is always the same two things — does this conversation
// already exist here, and does it already have this message — because those are the
// two answers that decide whether anything is added at all.
type Target struct {
	db *sql.DB

	// sessions is every conversation the store holds, by the address it files it
	// under. A real store has a few hundred, so holding them costs nothing and saves
	// a query per Android chat.
	sessions map[string]Session

	// lid and phone are the two halves of WhatsApp's hidden-identity mapping, when
	// this device recorded one. A conversation can be filed under a phone number on
	// one phone and under a hidden identity on the other, and without this it would
	// look like two different people and be duplicated rather than merged.
	lidOf   map[string]string
	phoneOf map[string]string

	// known caches the message identifiers already in a session, filled the first
	// time that session is asked about. A real store holds 161,000 messages and only
	// the handful of conversations being merged need looking at, so they are read
	// per conversation rather than all at once.
	known map[int64]map[string]struct{}
}

// Session is one conversation as the iPhone holds it.
type Session struct {
	// PK is Core Data's identifier for it.
	PK int64
	// Address is what the store files it under.
	Address string
	// Group reports whether it is a group rather than one person.
	Group bool
	// Messages is how many it holds now.
	Messages int
	// Removed marks a conversation the phone is no longer showing.
	Removed bool
}

// required names what this has to find before it will believe it has been handed an
// iPhone message store. Introspection rather than assumption: WhatsApp renames and
// removes columns between releases, and a clear refusal beats a confusing failure
// three steps later.
var required = map[string][]string{
	"ZWACHATSESSION": {"Z_PK", "ZCONTACTJID", "ZSESSIONTYPE", "ZMESSAGECOUNTER", "ZLASTMESSAGE", "ZREMOVED"},
	"ZWAMESSAGE":     {"Z_PK", "ZCHATSESSION", "ZSTANZAID", "ZMESSAGEDATE", "ZSORT", "ZISFROMME", "ZTEXT"},
	"Z_PRIMARYKEY":   {"Z_ENT", "Z_NAME", "Z_MAX"},
}

// OpenTarget reads an iPhone store as somewhere a history could be put.
//
// pairing is an optional second file, WhatsApp's own record of which hidden
// identities belong to which phone numbers. Without it a conversation filed under a
// hidden identity on one phone and a number on the other is merged only when the two
// spellings happen to match, which mostly they do not.
func OpenTarget(ctx context.Context, path, pairing string) (*Target, error) {
	// query_only alongside read-only: neither this code nor the driver's own
	// bookkeeping may write to a store somebody handed over.
	dsn := "file:" + url.PathEscape(path) + "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotAStore, err)
	}
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%w: %w", ErrNotAStore, err)
	}

	t := &Target{db: db, known: make(map[int64]map[string]struct{})}
	if err := t.check(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := t.readSessions(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := t.readPairing(ctx, pairing); err != nil {
		_ = db.Close()
		return nil, err
	}
	return t, nil
}

// Close releases the store.
func (t *Target) Close() error { return t.db.Close() }

// check refuses a file that is not the store this knows how to write into.
func (t *Target) check(ctx context.Context) error {
	for table, columns := range required {
		rows, err := t.db.QueryContext(ctx, "SELECT name FROM pragma_table_info(?)", table)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrNotAStore, err)
		}
		found := make(map[string]bool)
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				_ = rows.Close()
				return fmt.Errorf("%w: %w", ErrNotAStore, err)
			}
			found[name] = true
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return fmt.Errorf("%w: %w", ErrNotAStore, err)
		}
		if len(found) == 0 {
			return fmt.Errorf("%w: it has no %s table", ErrNotAStore, table)
		}
		for _, column := range columns {
			if !found[column] {
				return fmt.Errorf("%w: %s has no %s column", ErrUnfamiliarStore, table, column)
			}
		}
	}
	return nil
}

// readSessions loads every conversation the store holds.
func (t *Target) readSessions(ctx context.Context) error {
	rows, err := t.db.QueryContext(ctx, `
		SELECT s.Z_PK, s.ZCONTACTJID, s.ZSESSIONTYPE, s.ZREMOVED, count(m.Z_PK)
		FROM ZWACHATSESSION s
		LEFT JOIN ZWAMESSAGE m ON m.ZCHATSESSION = s.Z_PK
		WHERE s.ZCONTACTJID IS NOT NULL
		GROUP BY s.Z_PK`)
	if err != nil {
		return fmt.Errorf("reading the conversations already on the phone: %w", err)
	}
	defer func() { _ = rows.Close() }()

	t.sessions = make(map[string]Session, 512)
	for rows.Next() {
		var (
			s       Session
			kind    sql.NullInt64
			removed sql.NullInt64
		)
		if err := rows.Scan(&s.PK, &s.Address, &kind, &removed, &s.Messages); err != nil {
			return fmt.Errorf("reading the conversations already on the phone: %w", err)
		}
		s.Group, s.Removed = kind.Int64 == sessionGroup, removed.Int64 != 0
		t.sessions[s.Address] = s
	}
	return rows.Err()
}

// sessionGroup is the value ZSESSIONTYPE carries for a group.
const sessionGroup = 1

// readPairing loads WhatsApp's record of which hidden identities are which numbers.
//
// The file is optional and its shape has changed between releases, so a version this
// does not recognise yields no pairings rather than an error: the migration is worse
// without them and is not wrong without them.
func (t *Target) readPairing(ctx context.Context, path string) error {
	t.lidOf, t.phoneOf = map[string]string{}, map[string]string{}
	if path == "" {
		return nil
	}

	dsn := "file:" + url.PathEscape(path) + "?mode=ro&immutable=1&_pragma=query_only(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil
	}
	defer func() { _ = db.Close() }()

	rows, err := db.QueryContext(ctx, "SELECT ZLID, ZPHONENUMBER FROM ZWAPHONENUMBERLIDPAIR")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var lid, phone sql.NullString
		if err := rows.Scan(&lid, &phone); err != nil {
			return nil
		}
		if !lid.Valid || !phone.Valid {
			continue
		}
		digits := onlyDigits.ReplaceAllString(phone.String, "")
		if digits == "" {
			continue
		}
		t.lidOf[digits] = lid.String
		t.phoneOf[lid.String] = digits
	}
	return nil
}

var onlyDigits = regexp.MustCompile(`\D`)

// Session finds the conversation an address belongs to, however it is spelled.
//
// The address on the Android side and the address on the iPhone side are often the
// same string, and when they are not it is because one phone knows somebody by their
// number and the other by a hidden identity. Both spellings have to lead to the same
// conversation, or a merge becomes a duplicate.
func (t *Target) Session(address string) (Session, bool) {
	if s, ok := t.sessions[address]; ok {
		return s, true
	}

	user, server, found := strings.Cut(address, "@")
	if !found {
		return Session{}, false
	}
	switch server {
	case "s.whatsapp.net":
		if lid, ok := t.lidOf[user]; ok {
			s, ok := t.sessions[lid+"@lid"]
			return s, ok
		}
	case "lid":
		if phone, ok := t.phoneOf[user]; ok {
			s, ok := t.sessions[phone+"@s.whatsapp.net"]
			return s, ok
		}
	}
	return Session{}, false
}

// Sessions is how many conversations the phone holds.
func (t *Target) Sessions() int { return len(t.sessions) }

// Pairings is how many hidden identities this phone has a number for.
//
// Zero is the dangerous case, not an empty one: a conversation the iPhone files under
// a hidden identifier and the Android files under a number cannot then be recognised
// as the same person, and the migration adds them a second time. Nothing about the
// result looks wrong — the two entries have different addresses — so this has to be
// said before anybody agrees to it.
func (t *Target) Pairings() int { return len(t.lidOf) }

// Hidden is how many of the phone's own conversations are filed under a hidden
// identifier, which is what makes a missing pairing file matter or not.
func (t *Target) Hidden() int {
	var n int
	for address := range t.sessions {
		if strings.HasSuffix(address, "@lid") {
			n++
		}
	}
	return n
}

// Knows reports whether a conversation already holds a message.
//
// WhatsApp's own identifier is what is compared, because it is the same on both
// phones for the same message. That is what makes running a migration twice add
// nothing the second time, and it is the only reason this can be safely retried.
func (t *Target) Knows(ctx context.Context, session int64, id string) (bool, error) {
	if id == "" {
		// A message the source never recorded an identifier for cannot be matched
		// against anything. Treating it as new is the only honest answer; the
		// alternative is dropping a message because it could not be identified.
		return false, nil
	}

	ids, ok := t.known[session]
	if !ok {
		var err error
		if ids, err = t.identifiers(ctx, session); err != nil {
			return false, err
		}
		t.known[session] = ids
	}
	_, found := ids[id]
	return found, nil
}

// identifiers reads the message identifiers one conversation already holds.
func (t *Target) identifiers(ctx context.Context, session int64) (map[string]struct{}, error) {
	rows, err := t.db.QueryContext(ctx,
		"SELECT ZSTANZAID FROM ZWAMESSAGE WHERE ZCHATSESSION = :session AND ZSTANZAID IS NOT NULL",
		sql.Named("session", session))
	if err != nil {
		return nil, fmt.Errorf("reading what this conversation already holds: %w", err)
	}
	defer func() { _ = rows.Close() }()

	ids := make(map[string]struct{}, 1024)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("reading what this conversation already holds: %w", err)
		}
		ids[id] = struct{}{}
	}
	return ids, rows.Err()
}
