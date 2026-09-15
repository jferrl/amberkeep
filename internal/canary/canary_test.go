package canary

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// written builds a database with the shape and rows a test needs, and returns where
// it is. Nothing here is a real message: every value is invented and belongs to
// nobody, which is also the rule for the fixtures everywhere else in this repository.
func written(t *testing.T, statements ...string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "msgstore.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	return path
}

// fromAndroid is the shape of the tables this build reads, reduced to what these tests
// need: enough that the database is recognised as an Android one.
func fromAndroid(t *testing.T, extra ...string) string {
	t.Helper()
	return written(t, append([]string{
		`CREATE TABLE message (_id INTEGER PRIMARY KEY, chat_row_id INTEGER, message_type INTEGER, text_data TEXT)`,
		`CREATE TABLE chat (_id INTEGER PRIMARY KEY, jid_row_id INTEGER)`,
		`CREATE TABLE jid (_id INTEGER PRIMARY KEY, user TEXT, server TEXT)`,
	}, extra...)...)
}

func TestItSaysWhichPhoneWroteADatabase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func(*testing.T) string
		want  string
	}{
		{
			name:  "an Android database",
			build: func(t *testing.T) string { return fromAndroid(t) },
			want:  "android",
		},
		{
			name: "an iPhone store",
			build: func(t *testing.T) string {
				return written(t,
					`CREATE TABLE ZWAMESSAGE (Z_PK INTEGER PRIMARY KEY, ZMESSAGETYPE INTEGER, ZTEXT TEXT)`,
					`CREATE TABLE ZWACHATSESSION (Z_PK INTEGER PRIMARY KEY)`)
			},
			want: "iphone",
		},
		{
			name: "the database WhatsApp keeps its contacts in, which is neither",
			build: func(t *testing.T) string {
				return written(t, `CREATE TABLE wa_contacts (jid TEXT, display_name TEXT)`)
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			report, err := Look(context.Background(), tt.build(t), "test")
			if tt.want == "" {
				if err == nil {
					t.Fatal("a database that is not WhatsApp's was accepted as one")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if report.Schema.Platform != tt.want {
				t.Errorf("platform = %q, want %q", report.Schema.Platform, tt.want)
			}
		})
	}
}

// TestItCountsTheTypesNothingHasAMeaningFor is the point of the whole thing: a new
// WhatsApp release is a number nobody has seen before, on however many messages.
func TestItCountsTheTypesNothingHasAMeaningFor(t *testing.T) {
	t.Parallel()

	path := fromAndroid(t,
		// 0 is text and 1 is an image, both of which this build knows. 201 is not a
		// code WhatsApp has ever used, which is exactly what a new one looks like.
		`INSERT INTO message (chat_row_id, message_type, text_data) VALUES
			(1, 0, 'a'), (1, 0, 'b'), (1, 1, 'c'), (1, 201, 'd'), (1, 201, 'e'), (1, NULL, 'f')`)

	report, err := Look(context.Background(), path, "test")
	if err != nil {
		t.Fatal(err)
	}

	if report.Counted != "message.message_type" {
		t.Errorf("it counted %q", report.Counted)
	}
	if report.Recognised != 3 {
		t.Errorf("it recognised %d messages, want 3", report.Recognised)
	}
	want := []Count{{Type: 201, Rows: 2}, {Type: -1, Rows: 1}}
	if len(report.Unknown) != len(want) {
		t.Fatalf("unknown = %v, want %v", report.Unknown, want)
	}
	for i, count := range want {
		if report.Unknown[i] != count {
			t.Errorf("unknown[%d] = %v, want %v", i, report.Unknown[i], count)
		}
	}
}

// TestNothingButNamesAndNumbersLeaves is the guarantee that makes this worth
// running: the report is meant to be pasted into a public issue by somebody whose
// database is their entire private correspondence.
//
// The database below is full of things that must not come out: words, a name, a
// number somebody could be called on, a file on a disk.
func TestNothingButNamesAndNumbersLeaves(t *testing.T) {
	t.Parallel()

	secrets := []string{
		"meet me at the usual place",
		"Ana Lopez",
		"34600111222",
		"/Users/someone/Pictures/holiday.jpg",
		"ana@example.com",
	}

	path := fromAndroid(t,
		`CREATE TABLE message_media (message_row_id INTEGER, file_path TEXT, media_caption TEXT)`,
		`INSERT INTO message (chat_row_id, message_type, text_data) VALUES
			(1, 0, 'meet me at the usual place'), (1, 201, 'ana@example.com')`,
		`INSERT INTO jid (user, server) VALUES ('34600111222', 's.whatsapp.net')`,
		`INSERT INTO chat (jid_row_id) VALUES (1)`,
		`INSERT INTO message_media (message_row_id, file_path, media_caption) VALUES
			(1, '/Users/someone/Pictures/holiday.jpg', 'Ana Lopez')`,
	)

	report, err := Look(context.Background(), path, "test")
	if err != nil {
		t.Fatal(err)
	}

	// Both forms, because both are handed to somebody: the page they read and the
	// JSON they send.
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := json.Marshal(report.Entry(About{WhatsApp: "2.26.35.75"}))
	if err != nil {
		t.Fatal(err)
	}

	for _, form := range []string{string(body), string(entry)} {
		for _, secret := range secrets {
			if strings.Contains(form, secret) {
				t.Errorf("the report carries %q", secret)
			}
		}
		// And nothing that looks like a number somebody could be called on, whatever
		// it happens to be in this fixture.
		if found := regexp.MustCompile(`\d{7,}`).FindString(form); found != "" {
			t.Errorf("the report carries a long number: %q", found)
		}
	}
}

// TestJidRowIsNotMistakenForAColumn guards the one place a value could get in by
// accident: the shape is read from the catalogue, so a row's contents can never
// reach it, and this proves the columns reported are the columns declared.
func TestItReportsTheColumnsThatAreThere(t *testing.T) {
	t.Parallel()

	report, err := Look(context.Background(), fromAndroid(t), "test")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"_id", "chat_row_id", "message_type", "text_data"}
	got := report.Schema.Tables["message"]
	if len(got) != len(want) {
		t.Fatalf("message has columns %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column %d is %q, want %q", i, got[i], want[i])
		}
	}
}

// TestWhatIsNewIsWhatNoShapeHas covers the comparison, which is what turns a list of
// names into a report somebody can act on.
func TestWhatIsNewIsWhatNoShapeHas(t *testing.T) {
	t.Parallel()

	corpus := []Entry{
		{Platform: "android", WhatsApp: "1", Tables: map[string][]string{
			"message": {"_id", "text_data"},
			"chat":    {"_id"},
			"old":     {"_id"},
		}},
		{Platform: "android", WhatsApp: "2", Tables: map[string][]string{
			"message": {"_id", "text_data", "sort_id"},
			"chat":    {"_id"},
			"old":     {"_id"},
			"lately":  {"_id"},
		}},
	}

	report := Report{Schema: Schema{Platform: "android", Tables: map[string][]string{
		"message": {"_id", "text_data", "sort_id", "quoted_row_id"},
		"chat":    {"_id"},
		"lately":  {"_id"},
		"newer":   {"_id"},
	}}}
	report.compare(corpus)

	if len(report.NewTables) != 1 || report.NewTables[0] != "newer" {
		t.Errorf("new tables = %v, want [newer]", report.NewTables)
	}
	if got := report.NewColumns["message"]; len(got) != 1 || got[0] != "quoted_row_id" {
		t.Errorf("new columns of message = %v, want [quoted_row_id]", got)
	}
	// "old" is in every shape and not here, so it is gone. "lately" is in one shape
	// only, and a table one phone happened to carry is not a table this one is
	// missing.
	if len(report.GoneTables) != 1 || report.GoneTables[0] != "old" {
		t.Errorf("gone tables = %v, want [old]", report.GoneTables)
	}
	if len(report.Against) != 2 {
		t.Errorf("it compared against %v", report.Against)
	}
}

// TestTheCorpusIsShapesAndNothingElse covers what this repository ships. The entries
// come from real phones, so this is the test that would catch somebody contributing
// one with a name or a message in it.
func TestTheCorpusIsShapesAndNothingElse(t *testing.T) {
	t.Parallel()

	corpus := Corpus()
	if len(corpus) < 2 {
		t.Fatalf("the corpus has %d entries; it should carry the shapes this build was written against", len(corpus))
	}

	identifier := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	for _, entry := range corpus {
		if entry.Platform != "android" && entry.Platform != "iphone" {
			t.Errorf("%s: platform is %q", entry.Name(), entry.Platform)
		}
		if entry.WhatsApp == "" || entry.Seen == "" {
			t.Errorf("%s: an entry says nothing without the version and the day it was seen", entry.Name())
		}
		if len(entry.Tables) == 0 {
			t.Errorf("%s: no tables", entry.Name())
		}
		for table, columns := range entry.Tables {
			if !identifier.MatchString(table) {
				t.Errorf("%s: %q is not a table name", entry.Name(), table)
			}
			for _, column := range columns {
				if !identifier.MatchString(column) {
					t.Errorf("%s: %q is not a column name", entry.Name(), column)
				}
			}
		}
	}
}
