package contacts

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"

	_ "modernc.org/sqlite" // registers the pure-Go SQLite driver

	"github.com/jferrl/amberkeep/internal/model"
)

// Book is an address book gathered from whatever sources the user has.
//
// Names are keyed by phone number, because that is the only thing every source
// agrees on: a vCard has no idea what a WhatsApp address is, and a message
// database has no idea what somebody is saved as.
//
// The zero value is not usable; call New.
type Book struct {
	defaultCountry string
	byPhone        map[string]string
}

// New returns an empty address book.
//
// defaultCountry is the dialling code, without a plus, to assume for numbers
// written without one, such as "34" for Spain. Most address books are full of
// them, because people save their neighbours the way they dial them.
func New(defaultCountry string) *Book {
	return &Book{
		defaultCountry: strings.TrimPrefix(strings.TrimSpace(defaultCountry), "+"),
		byPhone:        make(map[string]string),
	}
}

// Len reports how many numbers the book can name.
func (b *Book) Len() int { return len(b.byPhone) }

// add records a name, keeping the first one seen. Address books contain the same
// number several times, and the first entry is as good a choice as any; what
// matters is that a later, emptier entry never erases a real name.
func (b *Book) add(rawPhone, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	phone, ok := NormalizePhone(rawPhone, b.defaultCountry)
	if !ok {
		return false
	}
	if _, seen := b.byPhone[phone]; seen {
		return false
	}
	b.byPhone[phone] = name
	return true
}

// ReadVCard loads an address book exported from a phone or from a contacts
// service, and reports how many numbers it named.
func (b *Book) ReadVCard(r io.Reader) (int, error) {
	cards, err := readVCards(r)
	if err != nil {
		return 0, err
	}
	var added int
	for _, c := range cards {
		name := c.name()
		for _, phone := range c.phones {
			if b.add(phone, name) {
				added++
			}
		}
	}
	return added, nil
}

// ReadVCardFile is ReadVCard for a path the user chose.
func (b *Book) ReadVCardFile(path string) (int, error) {
	// The path comes from the person running the tool, pointing at their own
	// address book on their own machine. There is nothing to confine it to.
	f, err := os.Open(path) //nolint:gosec // a user-chosen path is the whole point
	if err != nil {
		return 0, fmt.Errorf("opening the address book: %w", err)
	}
	defer func() { _ = f.Close() }()
	return b.ReadVCard(f)
}

// providerRow matches one line of a listing of the phone's address book, as
// produced by asking Android for its contacts:
//
//	Row: 0 display_name=Ana Lopez, data1=+34 600 11 22 33, data4=+34600112233
//
// The name can contain commas, so the fields are anchored on their keys rather
// than split on the separator.
var providerRow = regexp.MustCompile(
	`^Row: \d+ display_name=(?P<name>.*), data1=(?P<number>.*?)(?:, data4=(?P<normalised>.*))?$`)

// ReadAndroidContactsDump loads a listing of the phone's own address book.
//
// This is the most reliable source of names there is, because it is what the
// person actually saved, and it is available without asking them to export
// anything: the listing can be produced with a read-only query over a cable.
func (b *Book) ReadAndroidContactsDump(r io.Reader) (int, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxVCardBytes)

	var added int
	for scanner.Scan() {
		match := providerRow.FindStringSubmatch(strings.TrimRight(scanner.Text(), "\r"))
		if match == nil {
			continue
		}
		name := strings.TrimSpace(match[providerRow.SubexpIndex("name")])
		number := strings.TrimSpace(match[providerRow.SubexpIndex("number")])
		// Android also stores a normalised form of the number, which already carries
		// its country code and is therefore the better one to match on.
		if normalised := strings.TrimSpace(match[providerRow.SubexpIndex("normalised")]); isUsable(normalised) {
			number = normalised
		}
		if !isUsable(name) || !isUsable(number) {
			continue
		}
		if b.add(number, name) {
			added++
		}
	}
	if err := scanner.Err(); err != nil {
		return added, fmt.Errorf("reading the address book listing: %w", err)
	}
	return added, nil
}

// isUsable rejects the empty and null markers such a listing uses.
func isUsable(field string) bool {
	return field != "" && field != "NULL"
}

// ReadWhatsAppContacts loads names from WhatsApp's own contacts database, the
// file it keeps beside the message database.
//
// It is worth trying and worth not relying on: the copy inside a backup often has
// an empty contacts table, which is exactly why the other sources exist.
func (b *Book) ReadWhatsAppContacts(ctx context.Context, path string) (int, error) {
	// Immutable mode treats an absent file as an empty database, which would turn
	// a mistyped path into a silent "no contacts found". Check for it first so the
	// user is told what actually happened.
	if _, err := os.Stat(path); err != nil {
		return 0, fmt.Errorf("opening WhatsApp's contacts: %w", err)
	}

	// The file is usually left in write-ahead mode without its companion files, so
	// it is opened as immutable: that reads the contents as they stand and writes
	// nothing, which is what we want for somebody's only copy.
	dsn := "file:" + url.PathEscape(path) + "?immutable=1&_pragma=query_only(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return 0, fmt.Errorf("opening WhatsApp's contacts: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return 0, fmt.Errorf("opening WhatsApp's contacts: %w", err)
	}

	var exists int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'wa_contacts'`).
		Scan(&exists); err != nil || exists == 0 {
		return 0, nil
	}

	rows, err := db.QueryContext(ctx, `
		SELECT jid, number, display_name, wa_name, given_name, family_name
		FROM wa_contacts`)
	if err != nil {
		// An older or trimmed database may not have these columns. That is a reason
		// to move on to another source, not to fail.
		return 0, nil //nolint:nilerr // an unusable contacts table is not an error
	}
	defer rows.Close()

	var added int
	for rows.Next() {
		var jid, number, display, waName, given, family sql.NullString
		if err := rows.Scan(&jid, &number, &display, &waName, &given, &family); err != nil {
			return added, fmt.Errorf("reading WhatsApp's contacts: %w", err)
		}

		name := firstNonEmpty(display.String, joinName(given.String, family.String), waName.String)
		phone := firstNonEmpty(number.String, phoneFromJID(jid.String))
		if b.add(phone, name) {
			added++
		}
	}
	if err := rows.Err(); err != nil {
		return added, fmt.Errorf("reading WhatsApp's contacts: %w", err)
	}
	return added, nil
}

// ApplyTo writes what the book knows into a directory, and reports how many
// people it named.
//
// Only people the archive already refers to are named, because a directory is a
// description of one archive, not a copy of somebody's whole address book.
func (b *Book) ApplyTo(d *model.Directory) int {
	if d == nil || len(b.byPhone) == 0 {
		return 0
	}
	var named int
	for phone, name := range b.byPhone {
		jid := model.ParseJID(phone + "@" + string(model.ServerUser))
		before := d.Lookup(jid)
		d.Add(model.Contact{JID: jid, Name: name, Phone: phone})
		// Count the ones this actually taught the archive something about, which is
		// the number worth showing the user.
		if before.Name == "" {
			named++
		}
	}
	return named
}

// Name returns what the book calls a number, if anything.
func (b *Book) Name(rawPhone string) (string, bool) {
	phone, ok := NormalizePhone(rawPhone, b.defaultCountry)
	if !ok {
		return "", false
	}
	name, found := b.byPhone[phone]
	return name, found
}

// firstNonEmpty returns the first value with anything in it.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// joinName assembles a name from its parts.
func joinName(given, family string) string {
	return strings.TrimSpace(strings.TrimSpace(given) + " " + strings.TrimSpace(family))
}

// phoneFromJID pulls the number out of a WhatsApp address.
func phoneFromJID(jid string) string {
	j := model.ParseJID(jid)
	if phone, ok := j.Phone(); ok {
		return phone
	}
	return ""
}
