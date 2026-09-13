package android

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestPrepareChangesNoData is the promise that makes this command acceptable at
// all: it writes to a file somebody named, and what it writes must be derived data
// and nothing else. Every row of every table is compared before and after.
func TestPrepareChangesNoData(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := buildFixture(t)

	before := contentsOf(t, path)
	if _, err := Prepare(ctx, path); err != nil {
		t.Fatalf("Prepare() failed: %v", err)
	}
	after := contentsOf(t, path)

	if len(before) != len(after) {
		t.Fatalf("preparing changed the set of tables: %d became %d", len(before), len(after))
	}
	for table, rows := range before {
		if after[table] != rows {
			t.Errorf("the contents of table %s changed", table)
		}
	}
}

// TestPrepareMakesTheArchiveQuickToRead checks that the indexes are actually built
// and that the reader can then tell.
func TestPrepareMakesTheArchiveQuickToRead(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := buildFixture(t)

	t.Run("an untouched database says it is not prepared", func(t *testing.T) {
		r, err := Open(ctx, path)
		if err != nil {
			t.Fatalf("Open() failed: %v", err)
		}
		defer func() { _ = r.Close() }()

		if r.Indexed() {
			t.Error("a database with no indexes claims to be prepared")
		}
	})

	prepared, err := Prepare(ctx, path)
	if err != nil {
		t.Fatalf("Prepare() failed: %v", err)
	}

	t.Run("it built something", func(t *testing.T) {
		if prepared.NothingToDo() {
			t.Fatal("Prepare() built no indexes at all")
		}
		if prepared.Took <= 0 {
			t.Error("Prepare() reported no time taken")
		}
	})

	t.Run("the conversation index is the one that matters", func(t *testing.T) {
		if !hasIndexNamed(t, path, indexPrefix+"message") {
			t.Error("the index every conversation is read through was not built")
		}
	})

	t.Run("the reader can tell afterwards", func(t *testing.T) {
		r, err := Open(ctx, path)
		if err != nil {
			t.Fatalf("Open() failed: %v", err)
		}
		defer func() { _ = r.Close() }()

		if !r.Indexed() {
			t.Error("a prepared database does not say so")
		}
	})

	t.Run("running it again does nothing", func(t *testing.T) {
		again, err := Prepare(ctx, path)
		if err != nil {
			t.Fatalf("Prepare() failed the second time: %v", err)
		}
		if !again.NothingToDo() {
			t.Errorf("the second run built %v", again.Created)
		}
		if again.AlreadyThere != prepared.AlreadyThere+len(prepared.Created) {
			t.Errorf("the second run found %d indexes, want %d",
				again.AlreadyThere, prepared.AlreadyThere+len(prepared.Created))
		}
	})

	t.Run("reading it still gives the same archive", func(t *testing.T) {
		r, err := Open(ctx, path)
		if err != nil {
			t.Fatalf("Open() failed: %v", err)
		}
		defer func() { _ = r.Close() }()

		chats, err := r.Chats(ctx)
		if err != nil {
			t.Fatalf("Chats() failed: %v", err)
		}
		var messages int
		for _, chat := range chats {
			for _, err := range r.Messages(ctx, chat) {
				if err != nil {
					t.Fatalf("Messages() failed: %v", err)
				}
				messages++
			}
		}
		if messages == 0 {
			t.Error("a prepared database yielded no messages")
		}
	})
}

// TestEveryIndexNamesATableThatIsThere guards the written list of tables against
// drifting away from the database. A name that is wrong builds no index and slows
// nothing down visibly, which is exactly the kind of mistake that survives.
func TestEveryIndexNamesATableThatIsThere(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := buildFixture(t)

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("opening the fixture: %v", err)
	}
	defer func() { _ = db.Close() }()

	s, err := introspect(ctx, db)
	if err != nil {
		t.Fatalf("introspect() failed: %v", err)
	}

	for _, want := range indexesFor(s) {
		t.Run(want.name, func(t *testing.T) {
			if !s.has(want.table) {
				t.Errorf("an index was planned for %q, which this database does not have", want.table)
			}
		})
	}

	t.Run("the list names no table twice", func(t *testing.T) {
		seen := make(map[string]bool, len(read))
		for _, table := range read {
			if seen[table] {
				t.Errorf("%q is listed twice", table)
			}
			seen[table] = true
		}
	})
}

// TestPrepareRefusesWhatItCannotRead checks the failure path.
func TestPrepareRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()

	t.Run("a file that is not a database", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "msgstore.db")
		if err := os.WriteFile(path, []byte("not a database"), 0o600); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
		if _, err := Prepare(context.Background(), path); err == nil {
			t.Error("Prepare() accepted something that is not a database")
		}
	})

	t.Run("a database with none of the tables", func(t *testing.T) {
		t.Parallel()

		path := buildBare(t, `CREATE TABLE notes (id INTEGER PRIMARY KEY);`)
		prepared, err := Prepare(context.Background(), path)
		if err != nil {
			t.Fatalf("Prepare() failed: %v", err)
		}
		if !prepared.NothingToDo() {
			t.Errorf("Prepare() built %v on a database with nothing to index", prepared.Created)
		}
	})
}

// contentsOf hashes every value of every row of every table, so that a change
// anywhere shows up as a difference.
//
// The rows are read in row-id order on purpose. Without it the comparison fails for
// a reason that is not a change at all: once an index exists the query planner
// reads through it, and the same rows come back in a different order.
func contentsOf(t *testing.T, path string) map[string]string {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	defer func() { _ = db.Close() }()

	out := make(map[string]string, 32)
	for _, name := range tableNames(t, db) {
		// The name comes from the database, never from a caller.
		rows, err := db.Query(`SELECT * FROM "` + name + `" ORDER BY rowid`)
		if err != nil {
			// A table without row ids is summarised by its count alone, which still
			// catches anything being added or removed.
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM "` + name + `"`).Scan(&count); err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}
			out[name] = "count:" + strconv.Itoa(count)
			continue
		}

		columns, err := rows.Columns()
		if err != nil {
			t.Fatalf("reading the columns of %s: %v", name, err)
		}

		sum := fnv.New64a()
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		for rows.Next() {
			if err := rows.Scan(pointers...); err != nil {
				t.Fatalf("reading a row of %s: %v", name, err)
			}
			for _, v := range values {
				fmt.Fprintf(sum, "%v\x00", v)
			}
			_, _ = sum.Write([]byte("\x01"))
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		_ = rows.Close()
		out[name] = strconv.FormatUint(sum.Sum64(), 16)
	}
	return out
}

// tableNames lists the tables a database has, in a stable order.
func tableNames(t *testing.T, db *sql.DB) []string {
	t.Helper()

	rows, err := db.Query(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("reading a table name: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	return names
}

// hasIndexNamed reports whether the database carries an index by that name.
func hasIndexNamed(t *testing.T, path, name string) bool {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var found int
	row := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name)
	if err := row.Scan(&found); err != nil {
		t.Fatalf("looking for the index: %v", err)
	}
	return found > 0
}
