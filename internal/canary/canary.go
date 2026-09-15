// Package canary reports what a WhatsApp database holds that this build has never
// seen.
//
// The readers in this program never assume a column exists: they ask the database
// what it has and adapt, which is what lets one build open a database written by a
// version of WhatsApp nobody has looked at yet. That design keeps an archive
// readable, and it is silent. A database carrying a new table full of a new kind of
// message opens perfectly and says nothing at all about the part that was not read.
//
// This is the thing that says it. It compares a database against the shapes this
// build was developed against, and reports what is new, what is missing, and which
// message type codes appear that nothing here has a meaning for.
//
// # What it will not report
//
// Names, numbers, words, dates, filenames: nothing a person wrote or is called, and
// nothing that identifies anybody. The report is table names, column names, integer
// type codes and counts, and that is enforced by what it reads rather than by being
// careful — every query here selects a name from the catalogue or counts rows, and
// none of them selects a value. The point is that somebody whose database is their
// entire private correspondence can paste the whole report into a public issue
// without reading it first.
package canary

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sort"

	_ "modernc.org/sqlite" // the driver this program reads every database with

	"github.com/jferrl/amberkeep/internal/source/android"
	"github.com/jferrl/amberkeep/internal/source/ios"
)

// Schema is the shape of one database and nothing else: what its tables are called
// and what their columns are called.
type Schema struct {
	// Platform is "android" or "iphone".
	Platform string `json:"platform"`
	// Tables maps a table name to its column names, both in the order SQLite gives
	// them, which is the order they were declared in.
	Tables map[string][]string `json:"tables"`
}

// Count is how many rows carry one message type code.
type Count struct {
	// Type is the code as the database stores it. A row whose type is NULL is
	// counted under -1, which is not a code WhatsApp uses.
	Type int `json:"type"`
	Rows int `json:"rows"`
}

// Report is what the canary found.
type Report struct {
	// Build is the version of Amberkeep that looked.
	Build string `json:"build"`
	// Schema is the database's own shape, which is also what a corpus entry is made
	// of.
	Schema Schema `json:"schema"`
	// Against names the corpus entries this was compared with. Empty means there
	// were none for this platform, and then everything below is unknown because
	// nothing was known rather than because anything changed.
	Against []string `json:"against"`

	// NewTables are tables no corpus entry has. These are the interesting ones: a
	// table that appeared is usually a feature that appeared.
	NewTables []string `json:"new_tables,omitempty"`
	// NewColumns are columns no corpus entry has, in tables that they do have.
	NewColumns map[string][]string `json:"new_columns,omitempty"`
	// GoneTables are tables every corpus entry has and this database does not.
	GoneTables []string `json:"gone_tables,omitempty"`
	// GoneColumns are the same for columns.
	GoneColumns map[string][]string `json:"gone_columns,omitempty"`

	// Counted says where the message types were counted from, or is empty when
	// there was no such table to count.
	Counted string `json:"counted,omitempty"`
	// Unknown are the type codes this build has no meaning for, with how many rows
	// carry each. Sorted by how many, because a code on one message is a curiosity
	// and a code on forty thousand is a release note.
	Unknown []Count `json:"unknown,omitempty"`
	// Recognised is how many rows carry a code this build does know.
	Recognised int `json:"recognised"`
}

// place is where a build keeps the number saying what a message is.
//
// The table and column names are here rather than asked of the readers because a
// reader does not have a single such place to point at — it builds a query around
// whichever layout it found — and two names are cheaper to keep right in one spot
// than an accessor is to thread through. What the meanings are is a different
// question, and that is asked of the package that owns them.
type place struct{ table, column string }

// vocabulary is what one platform's build knows.
type vocabulary struct {
	// counts is where to count message types, in the order to try: the modern
	// layout first, then the one WhatsApp used before 2021.
	counts []place
	// known is every code this build has a meaning for.
	known map[int]bool
}

func vocabularies() map[string]vocabulary {
	return map[string]vocabulary{
		"android": {
			counts: []place{{"message", "message_type"}, {"messages", "media_wa_type"}},
			known:  set(android.Known()),
		},
		"iphone": {
			counts: []place{{"ZWAMESSAGE", "ZMESSAGETYPE"}},
			known:  set(ios.Known()),
		},
	}
}

func set(codes []int) map[int]bool {
	out := make(map[int]bool, len(codes))
	for _, c := range codes {
		out[c] = true
	}
	return out
}

// Look reads a database's shape and says what is new about it.
//
// The database is opened read-only and query_only, like everything else that reads
// somebody's archive here, and nothing is written anywhere.
func Look(ctx context.Context, path, build string) (Report, error) {
	dsn := "file:" + url.PathEscape(path) + "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return Report{}, fmt.Errorf("opening the database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return Report{}, fmt.Errorf("opening the database: %w", err)
	}

	schema, err := shapeOf(ctx, db)
	if err != nil {
		return Report{}, err
	}

	report := Report{Build: build, Schema: schema}
	if schema.Platform == "" {
		return report, fmt.Errorf(
			"this is a database, but not one of WhatsApp's: it has neither its Android tables nor its iPhone ones")
	}

	known := vocabularies()[schema.Platform]
	if err := report.count(ctx, db, known); err != nil {
		return report, err
	}
	report.compare(corpusFor(schema.Platform))
	return report, nil
}

// shapeOf reads the tables and their columns, and decides which phone wrote them.
func shapeOf(ctx context.Context, db *sql.DB) (Schema, error) {
	// Virtual tables are skipped, as they are everywhere else in this program: a
	// real database carries WhatsApp's own full-text indexes, and asking those what
	// columns they have needs SQLite modules a pure-Go build does not have. They
	// hold no shape of their own worth reporting.
	rows, err := db.QueryContext(ctx,
		`SELECT name FROM sqlite_master
		 WHERE type = 'table'
		   AND name NOT LIKE 'sqlite_%'
		   AND COALESCE(sql, '') NOT LIKE 'CREATE VIRTUAL%'
		 ORDER BY name`)
	if err != nil {
		return Schema{}, fmt.Errorf("listing the tables: %w", err)
	}

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return Schema{}, fmt.Errorf("reading a table name: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return Schema{}, fmt.Errorf("listing the tables: %w", err)
	}
	if err := rows.Close(); err != nil {
		return Schema{}, fmt.Errorf("listing the tables: %w", err)
	}

	schema := Schema{Tables: make(map[string][]string, len(tables))}
	for _, table := range tables {
		columns, err := columnsOf(ctx, db, table)
		if err != nil {
			// A table this build cannot describe is not a reason to describe none of
			// them. Real databases accumulate tables that depend on features a given
			// SQLite build lacks.
			continue
		}
		schema.Tables[table] = columns
	}

	switch {
	case schema.Tables["ZWAMESSAGE"] != nil, schema.Tables["ZWACHATSESSION"] != nil:
		schema.Platform = "iphone"
	case schema.Tables["message"] != nil, schema.Tables["messages"] != nil:
		schema.Platform = "android"
	}
	return schema, nil
}

// columnsOf reads one table's column names, in the order they were declared.
func columnsOf(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	// The name comes from the catalogue rather than from a caller, so putting it in
	// the statement is safe; PRAGMA takes no bound parameter here.
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return nil, fmt.Errorf("reading the columns of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var (
			cid        int
			name       string
			declared   sql.NullString
			notNull    int
			byDefault  sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &declared, &notNull, &byDefault, &primaryKey); err != nil {
			return nil, fmt.Errorf("reading a column of %s: %w", table, err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the columns of %s: %w", table, err)
	}
	return columns, nil
}

// count groups the messages by the number that says what they are.
//
// One query, on a column WhatsApp indexes, rather than reading a million rows
// through the model: this has to be quick enough that somebody runs it.
func (r *Report) count(ctx context.Context, db *sql.DB, known vocabulary) error {
	for _, where := range known.counts {
		columns, ok := r.Schema.Tables[where.table]
		if !ok || !contains(columns, where.column) {
			continue
		}

		rows, err := db.QueryContext(ctx, fmt.Sprintf(
			"SELECT %q, count(*) FROM %q GROUP BY 1", where.column, where.table))
		if err != nil {
			return fmt.Errorf("counting the message types: %w", err)
		}

		for rows.Next() {
			var (
				code sql.NullInt64
				n    int
			)
			if err := rows.Scan(&code, &n); err != nil {
				_ = rows.Close()
				return fmt.Errorf("counting the message types: %w", err)
			}
			// A NULL type is a real row in a real database — there was one in the
			// archive this program was built from — and it is not a code, so it is
			// reported as one nothing knows rather than as code zero, which means
			// plain text and would be a lie.
			kind := -1
			if code.Valid {
				kind = int(code.Int64)
			}
			if known.known[kind] {
				r.Recognised += n
				continue
			}
			r.Unknown = append(r.Unknown, Count{Type: kind, Rows: n})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("counting the message types: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("counting the message types: %w", err)
		}

		sort.Slice(r.Unknown, func(i, j int) bool {
			if r.Unknown[i].Rows != r.Unknown[j].Rows {
				return r.Unknown[i].Rows > r.Unknown[j].Rows
			}
			return r.Unknown[i].Type < r.Unknown[j].Type
		})
		r.Counted = where.table + "." + where.column
		return nil
	}
	return nil
}

// compare works out what this database has that the corpus does not, and the other
// way round.
//
// Against the union of every entry rather than each in turn: a column that one
// device has is a column this build has seen, whoever sent it. What is missing is
// judged the other way — against what every entry has — so that a table one phone
// happened to carry does not read as missing from all the others.
func (r *Report) compare(corpus []Entry) {
	if len(corpus) == 0 {
		return
	}

	seenTables := map[string]int{}
	seenColumns := map[string]map[string]int{}
	for _, entry := range corpus {
		r.Against = append(r.Against, entry.Name())
		for table, columns := range entry.Tables {
			seenTables[table]++
			if seenColumns[table] == nil {
				seenColumns[table] = map[string]int{}
			}
			for _, column := range columns {
				seenColumns[table][column]++
			}
		}
	}
	sort.Strings(r.Against)

	for table, columns := range r.Schema.Tables {
		if seenTables[table] == 0 {
			r.NewTables = append(r.NewTables, table)
			continue
		}
		for _, column := range columns {
			if seenColumns[table][column] == 0 {
				if r.NewColumns == nil {
					r.NewColumns = map[string][]string{}
				}
				r.NewColumns[table] = append(r.NewColumns[table], column)
			}
		}
	}
	sort.Strings(r.NewTables)

	everywhere := len(corpus)
	for table, count := range seenTables {
		if count < everywhere {
			continue
		}
		columns, here := r.Schema.Tables[table]
		if !here {
			r.GoneTables = append(r.GoneTables, table)
			continue
		}
		for column, seen := range seenColumns[table] {
			if seen == everywhere && !contains(columns, column) {
				if r.GoneColumns == nil {
					r.GoneColumns = map[string][]string{}
				}
				r.GoneColumns[table] = append(r.GoneColumns[table], column)
			}
		}
	}
	sort.Strings(r.GoneTables)
	for _, columns := range r.NewColumns {
		sort.Strings(columns)
	}
	for _, columns := range r.GoneColumns {
		sort.Strings(columns)
	}
}

func contains(haystack []string, needle string) bool {
	for _, straw := range haystack {
		if straw == needle {
			return true
		}
	}
	return false
}
