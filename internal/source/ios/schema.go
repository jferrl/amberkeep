package ios

import (
	"context"
	"database/sql"
	"fmt"
)

// The tables a WhatsApp iPhone store keeps. Core Data names them after the entity
// with a Z prefix, and the prefix is part of the name rather than decoration.
const (
	tableMessage  = "ZWAMESSAGE"
	tableSession  = "ZWACHATSESSION"
	tableMedia    = "ZWAMEDIAITEM"
	tableMember   = "ZWAGROUPMEMBER"
	tableGroup    = "ZWAGROUPINFO"
	tablePushName = "ZWAPROFILEPUSHNAME"
	tableDataItem = "ZWAMESSAGEDATAITEM"
)

// layout says which generation of the store we are looking at.
type layout uint8

const (
	layoutUnknown layout = iota
	// layoutCoreData is the only shape WhatsApp has shipped on iOS: a Core Data
	// store with Z-prefixed tables. It is named rather than assumed so that a
	// future one has somewhere to go.
	layoutCoreData
)

func (l layout) String() string {
	switch l {
	case layoutCoreData:
		return "core-data"
	case layoutUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// schema is what a particular store actually contains.
//
// WhatsApp adds and removes columns between releases, and Core Data renumbers its
// entities freely, so nothing here assumes a column exists. Readers ask the schema
// and adapt.
type schema struct {
	layout  layout
	columns map[string]map[string]struct{}
}

// introspect reads the shape of the database.
func introspect(ctx context.Context, db *sql.DB) (schema, error) {
	s := schema{columns: make(map[string]map[string]struct{})}

	// Virtual tables are skipped: describing one needs the SQLite module that
	// implements it, which a pure-Go build may not have, and none of them hold
	// conversation content of their own.
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
			// to refuse the whole archive.
			continue
		}
		s.columns[table] = cols
	}

	if s.hasColumn(tableMessage, "ZCHATSESSION") && s.has(tableSession) {
		s.layout = layoutCoreData
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

// columnOrNull returns the qualified column name when it exists and the SQL literal
// NULL when it does not, so a projection keeps a stable shape across versions.
func (s schema) columnOrNull(table, column string) string {
	if s.hasColumn(table, column) {
		return table + "." + column
	}
	return "NULL"
}

// columnAs is columnOrNull for a query that has given the table a shorter name.
// The alias is what the query says, the table is what the schema knows.
func (s schema) columnAs(table, alias, column string) string {
	if s.hasColumn(table, column) {
		return alias + "." + column
	}
	return "NULL"
}

// missing lists which of the named tables the database lacks, so a reader can say
// precisely what an unfamiliar file is missing rather than failing with a SQL error.
func (s schema) missing(tables ...string) []string {
	var absent []string
	for _, t := range tables {
		if !s.has(t) {
			absent = append(absent, t)
		}
	}
	return absent
}
