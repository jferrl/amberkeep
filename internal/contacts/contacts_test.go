package contacts

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jferrl/amberkeep/internal/model"
)

func TestNormalizePhone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		country string
		want    string
		ok      bool
	}{
		{
			name: "an international number as people write it",
			raw:  "+34 600 11 22 33", country: "34", want: "34600112233", ok: true,
		},
		{
			name: "the older way of writing an international number",
			raw:  "0034600112233", country: "34", want: "34600112233", ok: true,
		},
		{
			name: "a number written for people in the same country",
			raw:  "600 11 22 33", country: "34", want: "34600112233", ok: true,
		},
		{
			name: "punctuation people use to make numbers readable",
			raw:  "+34 (600) 11-22.33", country: "34", want: "34600112233", ok: true,
		},
		{
			name: "a number that already carries its country code without a plus",
			raw:  "34600112233", country: "34", want: "34600112233", ok: true,
		},
		{
			name: "a trunk digit is dropped when a country code is added",
			raw:  "07700 900123", country: "44", want: "447700900123", ok: true,
		},
		{
			name: "a number written as a link, the way version four does",
			raw:  "tel:+34-600-112-233", country: "34", want: "34600112233", ok: true,
		},
		{
			name: "an extension belongs to a switchboard, not to a person",
			raw:  "+34600112233;ext=42", country: "34", want: "34600112233", ok: true,
		},
		{
			name: "no country to assume leaves a national number as it stands",
			raw:  "600112233", country: "", want: "600112233", ok: true,
		},
		{name: "a short code is not a person", raw: "112", country: "34", ok: false},
		{name: "nothing at all", raw: "", country: "34", ok: false},
		{name: "no digits at all", raw: "not a number", country: "34", ok: false},
		{name: "only punctuation", raw: "+()- ", country: "34", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := NormalizePhone(tt.raw, tt.country)
			if ok != tt.ok {
				t.Fatalf("NormalizePhone(%q) usable = %v, want %v", tt.raw, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("NormalizePhone(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

// TestReadVCardHandlesRealExports covers the parts of the format that quietly
// lose people. Every case here appears in address books exported by real phones,
// and getting any of them wrong drops names without any sign that it happened.
func TestReadVCardHandlesRealExports(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		card  string
		phone string
		want  string
	}{
		{
			name:  "version three, the common case",
			card:  "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Ana Lopez\r\nTEL;TYPE=CELL:+34600112233\r\nEND:VCARD\r\n",
			phone: "34600112233", want: "Ana Lopez",
		},
		{
			// Version 2.1 encodes anything outside plain ASCII, which in a Spanish
			// address book is most of it. Failing to decode leaves mojibake.
			name: "version two with an accented name",
			card: "BEGIN:VCARD\r\nVERSION:2.1\r\n" +
				"FN;CHARSET=UTF-8;ENCODING=QUOTED-PRINTABLE:Jos=C3=A9 Mu=C3=B1oz\r\n" +
				"TEL;CELL:+34600112233\r\nEND:VCARD\r\n",
			phone: "34600112233", want: "José Muñoz",
		},
		{
			// Unfolding removes the break and exactly one following space, so a
			// well-formed fold keeps the word break on the line before it.
			name: "a long line folded onto the next",
			card: "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Maria del Carmen \r\n Fernandez\r\n" +
				"TEL:+34600112233\r\nEND:VCARD\r\n",
			phone: "34600112233", want: "Maria del Carmen Fernandez",
		},
		{
			name: "a quoted-printable value continued across lines",
			card: "BEGIN:VCARD\r\nVERSION:2.1\r\n" +
				"FN;ENCODING=QUOTED-PRINTABLE:Jos=\r\n=C3=A9\r\n" +
				"TEL:+34600112233\r\nEND:VCARD\r\n",
			phone: "34600112233", want: "José",
		},
		{
			name: "a property carrying a group prefix",
			card: "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Ana Lopez\r\nitem1.TEL;TYPE=CELL:+34600112233\r\n" +
				"item1.X-ABLabel:mobile\r\nEND:VCARD\r\n",
			phone: "34600112233", want: "Ana Lopez",
		},
		{
			name:  "version four writes the number as a link",
			card:  "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ana Lopez\r\nTEL;VALUE=uri;TYPE=\"cell\":tel:+34-600-112-233\r\nEND:VCARD\r\n",
			phone: "34600112233", want: "Ana Lopez",
		},
		{
			name:  "no display name, so one is assembled from the parts",
			card:  "BEGIN:VCARD\r\nVERSION:3.0\r\nN:Lopez;Ana;;;\r\nTEL:+34600112233\r\nEND:VCARD\r\n",
			phone: "34600112233", want: "Ana Lopez",
		},
		{
			name:  "a name containing an escaped separator",
			card:  "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Lopez\\, Ana\r\nTEL:+34600112233\r\nEND:VCARD\r\n",
			phone: "34600112233", want: "Lopez, Ana",
		},
		{
			name:  "a display name is preferred over the assembled parts",
			card:  "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Anita\r\nN:Lopez;Ana;;;\r\nTEL:+34600112233\r\nEND:VCARD\r\n",
			phone: "34600112233", want: "Anita",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := New("34")
			added, err := b.ReadVCard(strings.NewReader(tt.card))
			if err != nil {
				t.Fatalf("ReadVCard() failed: %v", err)
			}
			if added != 1 {
				t.Fatalf("ReadVCard() named %d numbers, want 1", added)
			}
			got, ok := b.Name(tt.phone)
			if !ok {
				t.Fatalf("the number %s was not named", tt.phone)
			}
			if got != tt.want {
				t.Errorf("name = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadVCardEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("one person with several numbers is named at each", func(t *testing.T) {
		t.Parallel()

		b := New("34")
		added, err := b.ReadVCard(strings.NewReader(
			"BEGIN:VCARD\nVERSION:3.0\nFN:Ana Lopez\nTEL;TYPE=CELL:+34600112233\n" +
				"TEL;TYPE=WORK:+34911223344\nEND:VCARD\n"))
		if err != nil {
			t.Fatalf("ReadVCard() failed: %v", err)
		}
		if added != 2 {
			t.Fatalf("named %d numbers, want 2", added)
		}
		for _, phone := range []string{"34600112233", "34911223344"} {
			if _, ok := b.Name(phone); !ok {
				t.Errorf("%s was not named", phone)
			}
		}
	})

	t.Run("several cards in one file", func(t *testing.T) {
		t.Parallel()

		b := New("34")
		added, err := b.ReadVCard(strings.NewReader(
			"BEGIN:VCARD\nFN:Ana\nTEL:+34600112233\nEND:VCARD\n" +
				"BEGIN:VCARD\nFN:Beatriz\nTEL:+34600445566\nEND:VCARD\n"))
		if err != nil {
			t.Fatalf("ReadVCard() failed: %v", err)
		}
		if added != 2 || b.Len() != 2 {
			t.Errorf("named %d of %d, want 2 of 2", added, b.Len())
		}
	})

	t.Run("a card with no number names nobody", func(t *testing.T) {
		t.Parallel()

		b := New("34")
		added, err := b.ReadVCard(strings.NewReader("BEGIN:VCARD\nFN:Nobody\nEND:VCARD\n"))
		if err != nil {
			t.Fatalf("ReadVCard() failed: %v", err)
		}
		if added != 0 {
			t.Errorf("named %d numbers for a card with none", added)
		}
	})

	t.Run("a card with no name names nobody", func(t *testing.T) {
		t.Parallel()

		b := New("34")
		added, err := b.ReadVCard(strings.NewReader("BEGIN:VCARD\nTEL:+34600112233\nEND:VCARD\n"))
		if err != nil {
			t.Fatalf("ReadVCard() failed: %v", err)
		}
		if added != 0 {
			t.Errorf("named %d numbers without a name", added)
		}
	})

	t.Run("the first name for a number wins", func(t *testing.T) {
		t.Parallel()

		b := New("34")
		if _, err := b.ReadVCard(strings.NewReader(
			"BEGIN:VCARD\nFN:Ana\nTEL:+34600112233\nEND:VCARD\n" +
				"BEGIN:VCARD\nFN:Ana old number\nTEL:+34600112233\nEND:VCARD\n")); err != nil {
			t.Fatalf("ReadVCard() failed: %v", err)
		}
		if name, _ := b.Name("34600112233"); name != "Ana" {
			t.Errorf("name = %q, want the first one seen", name)
		}
	})

	t.Run("rubbish is tolerated rather than fatal", func(t *testing.T) {
		t.Parallel()

		b := New("34")
		if _, err := b.ReadVCard(strings.NewReader(
			"not a card\nBEGIN:VCARD\nFN:Ana\nTEL:+34600112233\nnonsense without a colon\nEND:VCARD\n")); err != nil {
			t.Fatalf("ReadVCard() failed: %v", err)
		}
		if _, ok := b.Name("34600112233"); !ok {
			t.Error("a usable card beside rubbish was lost")
		}
	})

	t.Run("an empty file is not an error", func(t *testing.T) {
		t.Parallel()

		b := New("34")
		added, err := b.ReadVCard(strings.NewReader(""))
		if err != nil || added != 0 {
			t.Errorf("ReadVCard() on nothing = %d, %v", added, err)
		}
	})
}

func TestReadVCardFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "contacts.vcf")
	if err := os.WriteFile(path,
		[]byte("BEGIN:VCARD\nFN:Ana Lopez\nTEL:+34600112233\nEND:VCARD\n"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	b := New("34")
	added, err := b.ReadVCardFile(path)
	if err != nil {
		t.Fatalf("ReadVCardFile() failed: %v", err)
	}
	if added != 1 {
		t.Errorf("named %d numbers, want 1", added)
	}

	t.Run("a missing file is reported", func(t *testing.T) {
		t.Parallel()
		if _, err := New("34").ReadVCardFile(filepath.Join(t.TempDir(), "absent.vcf")); err == nil {
			t.Error("ReadVCardFile() on a missing file did not report it")
		}
	})
}

// TestReadAndroidContactsDump covers the listing of the phone's own address book,
// which is the most trustworthy source of names because it is what the person
// actually saved.
func TestReadAndroidContactsDump(t *testing.T) {
	t.Parallel()

	dump := strings.Join([]string{
		`Row: 0 display_name=Ana Lopez, data1=+34 600 11 22 33, data4=+34600112233`,
		// A name containing a comma must not be split on it.
		`Row: 1 display_name=Lopez, Beatriz, data1=600 44 55 66, data4=+34600445566`,
		// No normalised form, so the written number has to do.
		`Row: 2 display_name=Carlos, data1=+34600778899, data4=NULL`,
		// Rows with nothing usable.
		`Row: 3 display_name=NULL, data1=+34600000000, data4=NULL`,
		`Row: 4 display_name=Dana, data1=NULL, data4=NULL`,
		`unrelated output from the tool`,
	}, "\n")

	b := New("34")
	added, err := b.ReadAndroidContactsDump(strings.NewReader(dump))
	if err != nil {
		t.Fatalf("ReadAndroidContactsDump() failed: %v", err)
	}
	if added != 3 {
		t.Errorf("named %d numbers, want 3", added)
	}

	tests := map[string]string{
		"34600112233": "Ana Lopez",
		"34600445566": "Lopez, Beatriz",
		"34600778899": "Carlos",
	}
	for phone, want := range tests {
		got, ok := b.Name(phone)
		if !ok {
			t.Errorf("%s was not named", phone)
			continue
		}
		if got != want {
			t.Errorf("name for %s = %q, want %q", phone, got, want)
		}
	}
	if _, ok := b.Name("34600000000"); ok {
		t.Error("a row with no name produced one anyway")
	}
}

// TestApplyToNamesTheArchive is the point of the whole package: an archive that
// listed phone numbers now lists people.
func TestApplyToNamesTheArchive(t *testing.T) {
	t.Parallel()

	ana := model.ParseJID("34600112233@s.whatsapp.net")
	hidden := model.ParseJID("99887766@lid")

	d := model.NewDirectory()
	d.Add(model.Contact{JID: ana})
	d.Add(model.Contact{JID: hidden, PushName: "anita"})
	d.Alias(hidden, ana)

	if got := d.NameOf(ana); got != "~anita" {
		t.Fatalf("before the address book, NameOf() = %q", got)
	}

	b := New("34")
	if _, err := b.ReadVCard(strings.NewReader(
		"BEGIN:VCARD\nFN:Ana Lopez\nTEL:+34600112233\nEND:VCARD\n")); err != nil {
		t.Fatalf("ReadVCard() failed: %v", err)
	}

	named := b.ApplyTo(d)
	if named != 1 {
		t.Errorf("ApplyTo() named %d people, want 1", named)
	}

	t.Run("a saved name outranks the one somebody chose for themselves", func(t *testing.T) {
		if got := d.NameOf(ana); got != "Ana Lopez" {
			t.Errorf("NameOf() = %q, want the saved name", got)
		}
	})

	t.Run("the same person behind a hidden identifier is named too", func(t *testing.T) {
		if got := d.NameOf(hidden); got != "Ana Lopez" {
			t.Errorf("NameOf(hidden) = %q, want the saved name", got)
		}
	})

	t.Run("applying to nothing is harmless", func(t *testing.T) {
		if got := b.ApplyTo(nil); got != 0 {
			t.Errorf("ApplyTo(nil) = %d, want 0", got)
		}
		if got := New("34").ApplyTo(model.NewDirectory()); got != 0 {
			t.Errorf("an empty book named %d people", got)
		}
	})
}

// TestReadWhatsAppContacts covers WhatsApp's own contacts database. It is worth
// trying and worth not relying on: the copy inside a backup usually has an empty
// table, which is the whole reason the other sources exist.
func TestReadWhatsAppContacts(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, schema, data string) string {
		t.Helper()

		path := filepath.Join(t.TempDir(), "wa.db")
		db, err := sql.Open("sqlite", "file:"+path)
		if err != nil {
			t.Fatalf("creating the database: %v", err)
		}
		defer db.Close()
		if schema != "" {
			if _, err := db.Exec(schema); err != nil {
				t.Fatalf("creating the schema: %v", err)
			}
		}
		if data != "" {
			if _, err := db.Exec(data); err != nil {
				t.Fatalf("populating the database: %v", err)
			}
		}
		return path
	}

	const waSchema = `CREATE TABLE wa_contacts (
		_id INTEGER PRIMARY KEY, jid TEXT, number TEXT, display_name TEXT,
		wa_name TEXT, given_name TEXT, family_name TEXT)`

	tests := []struct {
		name   string
		schema string
		data   string
		want   int
		check  func(t *testing.T, b *Book)
	}{
		{
			name:   "names saved in the address book",
			schema: waSchema,
			data: `INSERT INTO wa_contacts (jid, number, display_name) VALUES
				('34600112233@s.whatsapp.net', '+34 600 11 22 33', 'Ana Lopez')`,
			want: 1,
			check: func(t *testing.T, b *Book) {
				if got, _ := b.Name("34600112233"); got != "Ana Lopez" {
					t.Errorf("name = %q", got)
				}
			},
		},
		{
			name:   "a name assembled from its parts when there is no display name",
			schema: waSchema,
			data: `INSERT INTO wa_contacts (jid, number, given_name, family_name) VALUES
				('34600112233@s.whatsapp.net', '+34600112233', 'Ana', 'Lopez')`,
			want: 1,
			check: func(t *testing.T, b *Book) {
				if got, _ := b.Name("34600112233"); got != "Ana Lopez" {
					t.Errorf("name = %q", got)
				}
			},
		},
		{
			name:   "a self-chosen name when nothing was saved",
			schema: waSchema,
			data: `INSERT INTO wa_contacts (jid, number, wa_name) VALUES
				('34600112233@s.whatsapp.net', '+34600112233', 'anita')`,
			want: 1,
		},
		{
			name:   "the number is taken from the address when the column is empty",
			schema: waSchema,
			data: `INSERT INTO wa_contacts (jid, display_name) VALUES
				('34600112233@s.whatsapp.net', 'Ana Lopez')`,
			want: 1,
			check: func(t *testing.T, b *Book) {
				if _, ok := b.Name("34600112233"); !ok {
					t.Error("the number was not taken from the address")
				}
			},
		},
		{
			name:   "rows with nobody to name are skipped",
			schema: waSchema,
			data: `INSERT INTO wa_contacts (jid, number) VALUES
				('34600112233@s.whatsapp.net', '+34600112233')`,
			want: 0,
		},
		{
			// This is what a backup actually contains, and it must not look like a
			// failure: it is a reason to try another source.
			name:   "the empty table found inside a backup",
			schema: waSchema,
			want:   0,
		},
		{
			name:   "a database holding something else entirely",
			schema: `CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT)`,
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := New("34")
			added, err := b.ReadWhatsAppContacts(context.Background(), build(t, tt.schema, tt.data))
			if err != nil {
				t.Fatalf("ReadWhatsAppContacts() failed: %v", err)
			}
			if added != tt.want {
				t.Errorf("named %d numbers, want %d", added, tt.want)
			}
			if tt.check != nil {
				tt.check(t, b)
			}
		})
	}

	t.Run("a missing file is reported", func(t *testing.T) {
		t.Parallel()

		_, err := New("34").ReadWhatsAppContacts(context.Background(), filepath.Join(t.TempDir(), "absent.db"))
		if err == nil {
			t.Error("ReadWhatsAppContacts() on a missing file did not report it")
		}
	})
}

// TestSourcesCombine checks the order that matters: a name from the phone's own
// address book must survive whatever the other sources say.
func TestSourcesCombine(t *testing.T) {
	t.Parallel()

	b := New("34")
	if _, err := b.ReadAndroidContactsDump(strings.NewReader(
		`Row: 0 display_name=Ana from my phone, data1=+34600112233, data4=+34600112233`)); err != nil {
		t.Fatalf("ReadAndroidContactsDump() failed: %v", err)
	}
	if _, err := b.ReadVCard(strings.NewReader(
		"BEGIN:VCARD\nFN:Ana from an old export\nTEL:+34600112233\nEND:VCARD\n")); err != nil {
		t.Fatalf("ReadVCard() failed: %v", err)
	}

	if got, _ := b.Name("34600112233"); got != "Ana from my phone" {
		t.Errorf("name = %q, want the one read first", got)
	}
	if b.Len() != 1 {
		t.Errorf("Len() = %d, want one person counted once", b.Len())
	}
}
