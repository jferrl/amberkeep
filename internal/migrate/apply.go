package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/export"
	"github.com/jferrl/amberkeep/internal/model"
)

// Writing a plan into a copy of an iPhone's message store.
//
// Everything here is arranged around one idea: the plan is a promise, and this keeps
// it or refuses. It works out what to write by the same rules that worked out the
// plan, then compares what it wrote against what was promised, and throws the whole
// thing away if the two disagree. A store nobody predicted is not one to put on a
// phone, even when it looks fine.
//
// The original is never opened for writing. It is copied first, and only the copy is
// touched; if anything goes wrong the copy is deleted rather than left behind, since
// a half-written store that looks openable is worse than no store at all.

// coreDataEpoch is 1 January 2001, which is when Apple decided time began.
var coreDataEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// Result is what applying a plan actually did.
type Result struct {
	// Path is the store that was written. It is a copy; nothing else was touched.
	Path string `json:"path"`

	// Added, Created and Merged are what happened, counted as it happened rather
	// than copied from the plan. They are compared against the plan before this is
	// returned, so a difference is a refusal rather than a surprise in a report.
	Added   int `json:"added"`
	Created int `json:"created"`
	Merged  int `json:"merged"`

	// Sampled says how many of the store's own conventions were copied from it
	// rather than guessed at. A low number on a real store is worth knowing.
	Sampled int `json:"sampled"`

	// Took is how long it ran.
	Took time.Duration `json:"took"`
}

// Apply writes a plan into a copy of an iPhone store.
//
// original is left exactly as it is. into is the copy that gets written, and must
// not exist: this refuses to write over anything, because the thing most likely to
// be at that path is the result of the last attempt, which somebody may still need.
func Apply(ctx context.Context, from Source, original, into string, plan Plan) (Result, error) {
	started := time.Now()

	if plan.Empty() {
		return Result{}, ErrNothingToDo
	}
	// #nosec G703 -- the destination the caller named is the request.
	if _, err := os.Stat(into); err == nil {
		return Result{}, fmt.Errorf("%w: %s", ErrWouldOverwrite, into)
	}
	if err := copyStore(original, into); err != nil {
		return Result{}, err
	}

	result, err := write(ctx, from, into, plan)
	if err != nil {
		// A half-written store is worse than none: it opens, and it is wrong.
		// Clearing up what this function itself just wrote.
		for _, leftover := range []string{into, into + "-wal", into + "-shm"} {
			_ = os.Remove(leftover) // #nosec G703 -- this function's own output
		}
		return Result{}, err
	}

	result.Path, result.Took = into, time.Since(started)
	return result, nil
}

// copyStore makes the copy everything is written to.
//
// Copied through rather than read whole: a real store is 118 MB and there is no
// reason for it to be in memory at all.
func copyStore(original, into string) error {
	source, err := os.Open(original) // #nosec G304,G703 -- the store the caller named
	if err != nil {
		return fmt.Errorf("reading the iPhone store: %w", err)
	}
	defer func() { _ = source.Close() }()

	// Created for this user alone: it is about to hold every message they have.
	destination, err := os.OpenFile(into, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304,G703
	if err != nil {
		return fmt.Errorf("making a copy to write into: %w", err)
	}
	if _, err := io.Copy(destination, source); err != nil {
		_ = destination.Close()
		// #nosec G703 -- removing the half-made copy this function just made.
		_ = os.Remove(into)
		return fmt.Errorf("making a copy to write into: %w", err)
	}
	if err := destination.Close(); err != nil {
		// #nosec G703 -- removing the half-made copy this function just made.
		_ = os.Remove(into)
		return fmt.Errorf("making a copy to write into: %w", err)
	}
	return nil
}

// write does the work, in one transaction.
func write(ctx context.Context, from Source, into string, plan Plan) (Result, error) {
	db, err := sql.Open("sqlite", "file:"+url.PathEscape(into)+"?_pragma=busy_timeout(10000)")
	if err != nil {
		return Result{}, fmt.Errorf("opening the copy: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return Result{}, fmt.Errorf("opening the copy: %w", err)
	}

	w := &writer{
		db:     db,
		names:  from.Directory(),
		saying: export.Options{Names: from.Directory()},
		conv:   sample(ctx, db),
		known:  make(map[string]int64),
		stmts:  make(map[string]*sql.Stmt),
	}
	if err := w.readColumns(ctx); err != nil {
		return Result{}, err
	}
	if err := w.readEntities(ctx); err != nil {
		return Result{}, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, fmt.Errorf("starting to write: %w", err)
	}
	w.tx = tx
	defer func() { _ = tx.Rollback() }()

	result, err := w.everything(ctx, from, plan)
	if err != nil {
		return Result{}, err
	}

	// The plan was a promise. If what was written is not what it said, nothing here
	// is trustworthy, whatever it looks like.
	if err := kept(plan, result); err != nil {
		return Result{}, err
	}
	if err := w.saveEntities(ctx); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(); err != nil {
		return Result{}, fmt.Errorf("finishing the write: %w", err)
	}
	if err := finish(ctx, db, into); err != nil {
		return Result{}, err
	}

	result.Sampled = w.conv.sampled
	return result, nil
}

// kept refuses a result that is not what the plan promised.
func kept(plan Plan, result Result) error {
	switch {
	case result.Added != plan.Adding:
		return fmt.Errorf("%w: the plan said %d messages and %d were written",
			ErrBrokePromise, plan.Adding, result.Added)
	case result.Created != plan.Creating:
		return fmt.Errorf("%w: the plan said %d conversations would be created and %d were",
			ErrBrokePromise, plan.Creating, result.Created)
	case result.Merged != plan.Merging:
		return fmt.Errorf("%w: the plan said %d conversations would be merged into and %d were",
			ErrBrokePromise, plan.Merging, result.Merged)
	}
	return nil
}

// finish folds the write-ahead log back into the store and removes it.
//
// A store handed to a phone with a log beside it is a store the phone will finish
// writing itself, over a database it did not write. The log has to be gone, and the
// file has to still say it is a write-ahead-log database, which is what the phone
// expects.
func finish(ctx context.Context, db *sql.DB, path string) error {
	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("folding the write-ahead log back in: %w", err)
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("closing the store: %w", err)
	}
	for _, side := range []string{"-wal", "-shm"} {
		// #nosec G703 -- the store this function was asked to finish.
		if err := os.Remove(path + side); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("clearing the write-ahead log: %w", err)
		}
	}
	return nil
}

// writer holds what the writing needs.
type writer struct {
	db *sql.DB
	tx *sql.Tx

	names  *model.Directory
	saying export.Options
	conv   conventions

	// columns is what each table actually has, so a store from another WhatsApp
	// release is written with the columns it holds rather than the ones this build
	// was compiled expecting.
	columns map[string]map[string]bool

	// entities is Core Data's own record of the highest identifier handed out for
	// each kind of row, read at the start and written back at the end. entityOf is
	// the type code every row of that kind carries.
	entities map[string]int64
	entityOf map[string]int64

	// known is the group members already created, by address, so a person who wrote
	// a hundred messages in a group is recorded once.
	known map[string]int64

	stmts map[string]*sql.Stmt
}

// readColumns asks each table what it holds.
func (w *writer) readColumns(ctx context.Context) error {
	w.columns = make(map[string]map[string]bool, 4)
	for _, table := range []string{"ZWACHATSESSION", "ZWAMESSAGE", "ZWAGROUPMEMBER", "ZWAGROUPINFO"} {
		rows, err := w.db.QueryContext(ctx, "SELECT name FROM pragma_table_info(:t)", sql.Named("t", table))
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUnfamiliarStore, err)
		}
		held := make(map[string]bool, 64)
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				_ = rows.Close()
				return fmt.Errorf("%w: %w", ErrUnfamiliarStore, err)
			}
			held[name] = true
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return fmt.Errorf("%w: %w", ErrUnfamiliarStore, err)
		}
		w.columns[table] = held
	}
	return nil
}

// readEntities loads Core Data's identifier bookkeeping.
func (w *writer) readEntities(ctx context.Context) error {
	rows, err := w.db.QueryContext(ctx, "SELECT Z_NAME, Z_ENT, Z_MAX FROM Z_PRIMARYKEY WHERE Z_NAME IS NOT NULL")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnfamiliarStore, err)
	}
	defer func() { _ = rows.Close() }()

	w.entities = make(map[string]int64, 32)
	w.entityOf = make(map[string]int64, 32)
	for rows.Next() {
		var (
			name         string
			ent, highest sql.NullInt64
		)
		if err := rows.Scan(&name, &ent, &highest); err != nil {
			return fmt.Errorf("%w: %w", ErrUnfamiliarStore, err)
		}
		w.entities[name] = highest.Int64
		w.entityOf[name] = ent.Int64
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: %w", ErrUnfamiliarStore, err)
	}
	for _, needed := range []string{"WAChatSession", "WAMessage"} {
		if _, ok := w.entities[needed]; !ok {
			return fmt.Errorf("%w: it has no %s entity", ErrUnfamiliarStore, needed)
		}
	}

	// Core Data's counter is trusted only as far as the rows agree with it. A store
	// whose counter has fallen behind its own contents would have this hand out an
	// identifier that is already in use, and the row written with it replaces one
	// that was there — silently, and in a file about to go onto a phone. Starting
	// from whichever is higher costs one query per kind of row and makes that
	// impossible rather than unlikely.
	for name := range w.entities {
		table := "Z" + strings.ToUpper(name)
		var highest sql.NullInt64
		if err := w.db.QueryRowContext(ctx,
			"SELECT max(Z_PK) FROM "+table).Scan(&highest); err != nil { // #nosec G202
			continue // a table this store does not have is not a problem
		}
		if highest.Valid && highest.Int64 > w.entities[name] {
			w.entities[name] = highest.Int64
		}
	}
	return nil
}

// next hands out the next identifier for a kind of row.
//
// Through Core Data's own counter rather than letting SQLite choose, because the
// phone will hand out the next one from exactly this number. A row written with an
// identifier above it is a row the phone will later write over.
func (w *writer) next(entity string) int64 {
	w.entities[entity]++
	return w.entities[entity]
}

// saveEntities writes the bookkeeping back.
func (w *writer) saveEntities(ctx context.Context) error {
	for name, max := range w.entities {
		if _, err := w.tx.ExecContext(ctx,
			"UPDATE Z_PRIMARYKEY SET Z_MAX = :max WHERE Z_NAME = :name",
			sql.Named("max", max), sql.Named("name", name)); err != nil {
			return fmt.Errorf("writing Core Data's bookkeeping back: %w", err)
		}
	}
	return nil
}

// toCoreData turns an instant into what Apple's stores hold: seconds since 2001.
func toCoreData(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	return t.Sub(coreDataEpoch).Seconds()
}

// insert writes one row, using only the columns this store actually has.
func (w *writer) insert(ctx context.Context, table string, values map[string]any) error {
	held := w.columns[table]
	names := make([]string, 0, len(values))
	for name := range values {
		if held[name] {
			names = append(names, name)
		}
	}
	// Sorted so the statement is the same every time and can be prepared once. A
	// map's order is deliberately not stable, and a new statement per row would cost
	// more than everything else here put together.
	sort.Strings(names)

	key := table + ":" + strings.Join(names, ",")
	stmt, ok := w.stmts[key]
	if !ok {
		marks := strings.TrimSuffix(strings.Repeat("?, ", len(names)), ", ")
		// #nosec G201,G202 -- the table and column names are this package's own
		// literals, filtered against what the store itself reported holding. Every
		// value is bound.
		query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			table, strings.Join(names, ", "), marks)

		var err error
		if stmt, err = w.tx.PrepareContext(ctx, query); err != nil {
			return fmt.Errorf("preparing to write to %s: %w", table, err)
		}
		w.stmts[key] = stmt
	}

	args := make([]any, 0, len(names))
	for _, name := range names {
		args = append(args, values[name])
	}
	if _, err := stmt.ExecContext(ctx, args...); err != nil {
		return fmt.Errorf("writing to %s: %w", table, err)
	}
	return nil
}
