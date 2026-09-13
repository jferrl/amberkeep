package android

import (
	"context"
	"database/sql"
	"fmt"
)

// layout says which generation of WhatsApp's Android database we are looking at.
type layout uint8

const (
	layoutUnknown layout = iota
	// layoutModern is the schema introduced in 2021: separate message, chat and jid
	// tables joined by row identifiers.
	layoutModern
	// layoutLegacy is the schema before it: one flat messages table keyed by the
	// remote address.
	layoutLegacy
)

func (l layout) String() string {
	switch l {
	case layoutModern:
		return "modern"
	case layoutLegacy:
		return "legacy"
	case layoutUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// schema is what a particular database actually contains.
//
// WhatsApp adds, renames and removes columns every few months, so nothing in this
// package assumes a column exists. Readers ask the schema and adapt, which is what
// lets one build open a database written by a version nobody has seen yet.
type schema struct {
	layout  layout
	columns map[string]map[string]struct{}
}

// introspect reads the shape of the database.
func introspect(ctx context.Context, db *sql.DB) (schema, error) {
	s := schema{columns: make(map[string]map[string]struct{})}

	// Virtual tables are skipped. A real database carries several full-text search
	// indexes, and those depend on SQLite modules a pure-Go build does not include,
	// so merely asking what columns they have fails. They hold no conversation
	// content of their own, only a search index over content stored elsewhere.
	rows, err := db.QueryContext(ctx,
		`SELECT name FROM sqlite_master
		 WHERE type = 'table'
		   AND name NOT LIKE 'sqlite_%'
		   AND COALESCE(sql, '') NOT LIKE 'CREATE VIRTUAL%'`)
	if err != nil {
		return schema{}, fmt.Errorf("listing tables: %w", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return schema{}, fmt.Errorf("reading a table name: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return schema{}, fmt.Errorf("listing tables: %w", err)
	}
	if err := rows.Close(); err != nil {
		return schema{}, fmt.Errorf("listing tables: %w", err)
	}

	for _, table := range tables {
		cols, err := columnsOf(ctx, db, table)
		if err != nil {
			// One table we cannot describe is one table we cannot use, not a reason
			// to refuse the whole archive. Real databases accumulate tables that
			// depend on features a given SQLite build lacks, and none of them hold
			// conversations.
			continue
		}
		s.columns[table] = cols
	}

	switch {
	case s.hasColumn("message", "chat_row_id") && s.has("chat") && s.has("jid"):
		s.layout = layoutModern
	case s.hasColumn("messages", "key_remote_jid"):
		s.layout = layoutLegacy
	}
	return s, nil
}

// columnsOf reads one table's column names.
func columnsOf(ctx context.Context, db *sql.DB, table string) (map[string]struct{}, error) {
	// The table name comes from sqlite_master, not from a caller, so interpolating
	// it is safe; PRAGMA does not accept a bound parameter here.
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return nil, fmt.Errorf("reading the columns of %s: %w", table, err)
	}
	defer rows.Close()

	cols := make(map[string]struct{})
	for rows.Next() {
		var (
			cid        int
			name       string
			declType   sql.NullString
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &declType, &notNull, &defaultVal, &primaryKey); err != nil {
			return nil, fmt.Errorf("reading a column of %s: %w", table, err)
		}
		cols[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the columns of %s: %w", table, err)
	}
	return cols, nil
}

// has reports whether the database contains a table.
func (s schema) has(table string) bool {
	_, ok := s.columns[table]
	return ok
}

// hasColumn reports whether a table exists and contains a column.
func (s schema) hasColumn(table, column string) bool {
	cols, ok := s.columns[table]
	if !ok {
		return false
	}
	_, ok = cols[column]
	return ok
}

// pick returns the first of several candidate column names that this database
// actually has, or an empty string when it has none of them. It is how a query
// survives a column being renamed between WhatsApp versions.
func (s schema) pick(table string, candidates ...string) string {
	for _, c := range candidates {
		if s.hasColumn(table, c) {
			return c
		}
	}
	return ""
}

// columnOrNull returns the column name when it exists and the SQL literal NULL
// when it does not, so a projection can keep a stable shape across versions.
func (s schema) columnOrNull(table, column string) string {
	if s.hasColumn(table, column) {
		return fmt.Sprintf("%s.%s", table, column)
	}
	return "NULL"
}

// missing lists which of the named tables the database lacks. A reader uses it to
// tell the user precisely what an unfamiliar database is missing rather than
// failing with a SQL error.
func (s schema) missing(tables ...string) []string {
	var absent []string
	for _, t := range tables {
		if !s.has(t) {
			absent = append(absent, t)
		}
	}
	return absent
}
