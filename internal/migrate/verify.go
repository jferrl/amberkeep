package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Checking the result against the original, before anybody's phone is involved.
//
// This is the safety net, and it is written before the thing it catches. A checker
// built after the writer can only ever be tested by running the writer and watching
// it pass, which proves the two agree and nothing about whether a fault would be
// noticed. Built first, every check here is tested by deliberately breaking a store
// and insisting the check fails.
//
// What it looks for is not arbitrary. The list was earned on a real migration onto a
// real phone, by the Python prototype this is ported from, and each entry is
// something that was either checked before that restore or found while getting it to
// work. The invariants matter more than they look: Core Data keeps its own
// bookkeeping, and a store that is merely valid SQLite can still make WhatsApp show
// an empty conversation list, or lose the thread of a chat, or crash on opening.

// Report is what checking found. It is meant to be shown to the person deciding, so
// every check has a name they could act on rather than a code.
type Report struct {
	Checks []Check `json:"checks"`

	// Added counts what the result holds beyond the original, per table, so a person
	// can see the shape of the change without reading a word anybody wrote.
	Added map[string]int `json:"added,omitempty"`
}

// Check is one thing that had to be true.
type Check struct {
	Name string `json:"name"`
	// Passed is the whole point. A check that could not be carried out at all is a
	// failure, not a pass: the alternative is a green report that checked nothing.
	Passed bool `json:"passed"`
	// Detail says what was found, when the name alone is not enough to act on.
	Detail string `json:"detail,omitempty"`
}

// OK reports whether every check passed. Nothing should be restored to a phone when
// this is false.
func (r Report) OK() bool {
	for _, c := range r.Checks {
		if !c.Passed {
			return false
		}
	}
	return len(r.Checks) > 0
}

// Failures are the checks that did not pass, which is what a person needs to see.
func (r Report) Failures() []Check {
	var out []Check
	for _, c := range r.Checks {
		if !c.Passed {
			out = append(out, c)
		}
	}
	return out
}

// Summary is the one line worth saying about a report.
func (r Report) Summary() string {
	failed := len(r.Failures())
	switch {
	case len(r.Checks) == 0:
		return "nothing was checked, which is not the same as nothing being wrong"
	case failed == 0:
		return fmt.Sprintf("all %d checks passed", len(r.Checks))
	default:
		return fmt.Sprintf("%s of %d did not pass; nothing should be restored from this",
			plural(failed, "check", "checks"), len(r.Checks))
	}
}

// Verify checks a migrated store against the original it was made from.
//
// Both are opened read-only. The plan says which conversations were merged into,
// because those are the only existing conversations allowed to have changed at all,
// and then only in the two columns that order a conversation and point at its end.
func Verify(ctx context.Context, original, result string, plan Plan) (Report, error) {
	var r Report

	// Before opening anything: a store with a write-ahead log beside it is a store
	// that has not been finished. Restoring one means the phone replays whatever is
	// in that log on first open, over a database it did not write.
	r.add("no write-ahead log left beside the result", noSidecars(result))
	// Checked before anything opens the file, because opening it is the thing most
	// likely to create one.
	r.add("the result is still a write-ahead-log database, as the phone expects", walMode(result))

	// immutable, not merely read-only. SQLite creates a -shm beside any write-ahead-log
	// database it opens, read-only or not, and this one is the file that is about to
	// be restored onto a phone: leaving two new files beside it is modifying the thing
	// being checked, and it makes the second run of this fail its own first check.
	// The claim immutable makes is true here — the file was finished and closed before
	// checking began.
	db, err := sql.Open("sqlite",
		"file:"+url.PathEscape(result)+"?mode=ro&immutable=1&_pragma=query_only(1)")
	if err != nil {
		return r, fmt.Errorf("opening the migrated store: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	// Bound, not interpolated. url.PathEscape leaves a single quote alone, so a store
	// whose path contained one would end the SQL string literal and whatever followed
	// would be statements — on a path that comes from whoever is running this, over a
	// file somebody was handed.
	if _, err := db.ExecContext(ctx, "ATTACH DATABASE ? AS original",
		"file:"+url.PathEscape(original)+"?mode=ro&immutable=1"); err != nil {
		return r, fmt.Errorf("opening the original to compare against: %w", err)
	}

	v := &verifier{ctx: ctx, db: db, r: &r, merged: mergedSessions(plan)}
	v.structural()
	v.nothingLost()
	v.bookkeeping()
	v.perSession()
	v.counts()
	return r, v.err
}

// mergedSessions is the conversations the plan said it would merge into. They are the
// only pre-existing conversations whose ordering is allowed to have moved.
func mergedSessions(plan Plan) map[int64]bool {
	out := make(map[int64]bool)
	for _, c := range plan.Conversations {
		if c.Merging() && c.Adding > 0 {
			out[c.Session] = true
		}
	}
	return out
}

// add records one check.
func (r *Report) add(name string, ok bool, detail ...string) {
	c := Check{Name: name, Passed: ok}
	if len(detail) > 0 {
		c.Detail = detail[0]
	}
	r.Checks = append(r.Checks, c)
}

// noSidecars reports whether the result stands alone.
func noSidecars(path string) bool {
	for _, side := range []string{"-wal", "-shm"} {
		// #nosec G703 -- looking beside the file this function was asked about is
		// the question, and the path is the caller's own argument.
		if _, err := os.Stat(path + side); err == nil {
			return false
		}
	}
	return true
}

// walMode reports whether the file still says it is a write-ahead-log database.
//
// Bytes 18 and 19 are the format versions SQLite writes and reads with; 2 means
// write-ahead logging. WhatsApp's own store is one, and handing the phone a store in
// the older journal mode is handing it something it did not write.
func walMode(path string) bool {
	// #nosec G304,G703 -- reading the file this function was asked about is the question,
	// and the path is the caller's own argument rather than anything a store contained.
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	header := make([]byte, 20)
	if _, err := f.Read(header); err != nil {
		return false
	}
	return header[18] == 2 && header[19] == 2
}

// verifier carries the state the checks share.
type verifier struct {
	ctx    context.Context
	db     *sql.DB
	r      *Report
	merged map[int64]bool
	err    error
}

// scalar runs a query that answers with one number.
//
// A query that fails is a failed check rather than a silent zero: "it could not be
// checked" and "it was checked and is fine" must never look the same.
func (v *verifier) scalar(query string, args ...any) (int64, bool) {
	var n int64
	if err := v.db.QueryRowContext(v.ctx, query, args...).Scan(&n); err != nil {
		v.err = errors.Join(v.err, err)
		return 0, false
	}
	return n, true
}

// text runs a query that answers with one string.
func (v *verifier) text(query string) (string, bool) {
	var s string
	if err := v.db.QueryRowContext(v.ctx, query).Scan(&s); err != nil {
		v.err = errors.Join(v.err, err)
		return "", false
	}
	return s, true
}

// structural is whether this is a database at all.
func (v *verifier) structural() {
	said, ok := v.text("PRAGMA main.integrity_check")
	v.r.add("the result is a sound database", ok && said == "ok", said)
}

// tables is what both sides are expected to hold. Checked for presence rather than
// assumed: a store from another WhatsApp release may not have all of them, and one
// that is absent from both sides is not a fault.
var tables = []string{
	"ZWACHATSESSION", "ZWAMESSAGE", "ZWAMEDIAITEM",
	"ZWAGROUPINFO", "ZWAGROUPMEMBER", "ZWAMESSAGEINFO", "ZWAPROFILEPUSHNAME",
}

// mutable names the columns a migration is allowed to change on a row that was
// already there. Everything else must come back byte for byte.
//
// A conversation that gains messages has to be reordered — that is what ZSORT is —
// and its end moves, which is what the session's counters and pointers say. Nothing
// else about an existing row may move, and that is the promise this whole feature
// rests on: the messages already on the phone are left exactly as they are.
var mutable = map[string][]string{
	"ZWAMESSAGE":     {"ZSORT", "ZLASTSESSION"},
	"ZWACHATSESSION": {"ZMESSAGECOUNTER", "ZLASTMESSAGE", "ZLASTMESSAGEDATE", "ZLASTMESSAGETEXT"},
}

// nothingLost is the promise: every row that was there before is still there, and is
// unchanged apart from the few columns a merge is allowed to move.
func (v *verifier) nothingLost() {
	v.r.Added = make(map[string]int, len(tables))

	for _, table := range tables {
		if !v.has(table) {
			continue
		}
		columns, ok := v.comparable(table)
		if !ok {
			v.r.add("rows already on the phone are unchanged in "+table, false,
				"the columns could not be read")
			continue
		}

		before, okBefore := v.scalar("SELECT count(*) FROM original." + table)
		after, okAfter := v.scalar("SELECT count(*) FROM main." + table)
		if okBefore && okAfter {
			v.r.Added[table] = int(after - before)
		}

		// Each row that was there is looked for by its own identifier, rather than
		// the two totals being compared. Comparing totals lets a deletion hide behind
		// an insertion, which is exactly what it did the first time this was tried:
		// a message was deleted, two were added, the count went up, and the check
		// said nothing was removed.
		gone, okGone := v.scalar(fmt.Sprintf(
			"SELECT count(*) FROM original.%[1]s o LEFT JOIN main.%[1]s m ON m.Z_PK = o.Z_PK WHERE m.Z_PK IS NULL",
			table))
		v.r.add("nothing was removed from "+table, okGone && gone == 0,
			fmt.Sprintf("%d of %d gone, %d rows now", gone, before, after))

		// Compared both ways round: a row that changed shows up as missing from one
		// side and extra on the other, and checking one direction would call a
		// changed row unchanged.
		highest, ok := v.scalar("SELECT coalesce(max(Z_PK), 0) FROM original." + table)
		if !ok {
			continue
		}
		lost, okLost := v.scalar(fmt.Sprintf(
			"SELECT count(*) FROM (SELECT %[1]s FROM original.%[2]s EXCEPT SELECT %[1]s FROM main.%[2]s WHERE Z_PK <= %[3]d)",
			columns, table, highest))
		changed, okChanged := v.scalar(fmt.Sprintf(
			"SELECT count(*) FROM (SELECT %[1]s FROM main.%[2]s WHERE Z_PK <= %[3]d EXCEPT SELECT %[1]s FROM original.%[2]s)",
			columns, table, highest))

		detail := ""
		if allowed := mutable[table]; len(allowed) > 0 {
			detail = "apart from " + strings.Join(allowed, " and ")
		}
		v.r.add("rows already on the phone are unchanged in "+table,
			okLost && okChanged && lost == 0 && changed == 0,
			strings.TrimSpace(fmt.Sprintf("%d missing, %d altered %s", lost, changed, detail)))
	}
}

// has reports whether both sides hold a table.
func (v *verifier) has(table string) bool {
	n, ok := v.scalar(
		"SELECT (SELECT count(*) FROM main.sqlite_master WHERE type='table' AND name=:t) * "+
			"(SELECT count(*) FROM original.sqlite_master WHERE type='table' AND name=:t)",
		sql.Named("t", table))
	return ok && n > 0
}

// hasColumn reports whether the result holds a column.
//
// Asked rather than assumed, for the same reason the readers ask: WhatsApp adds and
// removes columns between releases, and a check that cannot run on a store is a
// check that should say so rather than one that fails it.
func (v *verifier) hasColumn(table, column string) bool {
	n, ok := v.scalar("SELECT count(*) FROM pragma_table_info(:t) WHERE name = :c",
		sql.Named("t", table), sql.Named("c", column))
	return ok && n > 0
}

// comparable is the column list to compare a table on: everything except the few a
// merge is allowed to move. Read from the store rather than written down here,
// because WhatsApp adds and removes columns between releases.
func (v *verifier) comparable(table string) (string, bool) {
	rows, err := v.db.QueryContext(v.ctx, "SELECT name FROM pragma_table_info(:t)", sql.Named("t", table))
	if err != nil {
		v.err = errors.Join(v.err, err)
		return "", false
	}
	defer func() { _ = rows.Close() }()

	skip := make(map[string]bool, 4)
	for _, c := range mutable[table] {
		skip[c] = true
	}

	var keep []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			v.err = errors.Join(v.err, err)
			return "", false
		}
		if !skip[name] {
			keep = append(keep, name)
		}
	}
	if err := rows.Err(); err != nil || len(keep) == 0 {
		v.err = errors.Join(v.err, err)
		return "", false
	}
	return strings.Join(keep, ", "), true
}

// bookkeeping is Core Data's own housekeeping, which WhatsApp trusts absolutely.
//
// Z_PRIMARYKEY holds the highest identifier handed out for each kind of row. A store
// whose Z_MAX is below a row that exists will hand the same identifier out again on
// the phone, and the second row silently replaces the first.
func (v *verifier) bookkeeping() {
	rows, err := v.db.QueryContext(v.ctx,
		"SELECT Z_NAME, Z_MAX FROM main.Z_PRIMARYKEY WHERE Z_NAME IS NOT NULL AND Z_NAME LIKE 'WA%'")
	if err != nil {
		v.err = errors.Join(v.err, err)
		v.r.add("Core Data's own bookkeeping is consistent", false, "it could not be read")
		return
	}
	defer func() { _ = rows.Close() }()

	type entity struct {
		name string
		max  int64
	}
	var entities []entity
	for rows.Next() {
		var e entity
		if err := rows.Scan(&e.name, &e.max); err != nil {
			v.err = errors.Join(v.err, err)
			return
		}
		entities = append(entities, e)
	}
	if err := rows.Err(); err != nil {
		v.err = errors.Join(v.err, err)
		return
	}

	var behind []string
	for _, e := range entities {
		table := "Z" + strings.ToUpper(e.name)
		if !v.has(table) {
			continue
		}
		highest, ok := v.scalar(fmt.Sprintf("SELECT coalesce(max(Z_PK), 0) FROM main.%s", table))
		if ok && highest > e.max {
			behind = append(behind, fmt.Sprintf("%s says %d but holds %d", e.name, e.max, highest))
		}
	}
	v.r.add("Core Data will not hand out an identifier that is already in use",
		len(behind) == 0, strings.Join(behind, "; "))

	for _, table := range []string{"Z_METADATA", "Z_MODELCACHE"} {
		if !v.has(table) {
			continue
		}
		changed, ok := v.scalar(fmt.Sprintf(
			"SELECT count(*) FROM (SELECT * FROM main.%[1]s EXCEPT SELECT * FROM original.%[1]s)", table))
		v.r.add("the store still describes itself the same way ("+table+")", ok && changed == 0)
	}
}

// perSession is what has to be true of every conversation that gained messages.
func (v *verifier) perSession() {
	highestMessage, ok := v.scalar("SELECT coalesce(max(Z_PK), 0) FROM original.ZWAMESSAGE")
	if !ok {
		return
	}
	highestSession, ok := v.scalar("SELECT coalesce(max(Z_PK), 0) FROM original.ZWACHATSESSION")
	if !ok {
		return
	}

	// Ordering may only have moved inside conversations that were merged into.
	// Anywhere else it is a conversation this had no business touching.
	//
	// The merged conversations go into the query as literals rather than as bound
	// parameters. They are int64s this package put there itself, read from a plan it
	// built; nothing a user typed or a file contained reaches this string.
	where := ""
	if len(v.merged) > 0 {
		ids := make([]string, 0, len(v.merged))
		for session := range v.merged {
			ids = append(ids, strconv.FormatInt(session, 10))
		}
		where = " AND m.ZCHATSESSION NOT IN (" + strings.Join(ids, ",") + ")"
	}
	moved, ok := v.scalar(
		"SELECT count(*) FROM main.ZWAMESSAGE m JOIN original.ZWAMESSAGE o ON o.Z_PK = m.Z_PK "+ // #nosec G202
			"WHERE m.Z_PK <= :highest AND m.ZSORT IS NOT o.ZSORT"+where,
		sql.Named("highest", highestMessage))
	v.r.add("conversations this did not touch are ordered exactly as they were",
		ok && moved == 0, fmt.Sprintf("%d moved", moved))

	for _, session := range v.sessionsToCheck(highestSession) {
		v.oneSession(session, highestMessage)
	}
}

// sessionsToCheck is every conversation that was merged into or newly created.
func (v *verifier) sessionsToCheck(highestSession int64) []int64 {
	seen := make(map[int64]bool, len(v.merged))
	out := make([]int64, 0, len(v.merged))
	for session := range v.merged {
		seen[session] = true
		out = append(out, session)
	}

	rows, err := v.db.QueryContext(v.ctx,
		"SELECT Z_PK FROM main.ZWACHATSESSION WHERE Z_PK > :highest", sql.Named("highest", highestSession))
	if err != nil {
		v.err = errors.Join(v.err, err)
		return out
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var pk int64
		if err := rows.Scan(&pk); err != nil {
			v.err = errors.Join(v.err, err)
			return out
		}
		if !seen[pk] {
			out = append(out, pk)
		}
	}
	if err := rows.Err(); err != nil {
		v.err = errors.Join(v.err, err)
	}
	return out
}

// oneSession checks the invariants WhatsApp relies on within one conversation.
func (v *verifier) oneSession(session, highestMessage int64) {
	name := fmt.Sprintf("conversation %d", session)

	empty, ok := v.scalar("SELECT count(*) FROM main.ZWAMESSAGE WHERE ZCHATSESSION = :s",
		sql.Named("s", session))
	if !ok || empty == 0 {
		v.r.add(name+" holds messages", false, "it is empty")
		return
	}

	// ZSORT is what orders a conversation on the phone. It has to be 1..N with no
	// gaps and in date order, or the conversation opens jumbled or truncated.
	gaps, ok := v.scalar(
		"SELECT count(*) FROM (SELECT ZSORT, row_number() OVER (ORDER BY ZMESSAGEDATE, Z_PK) AS n "+
			"FROM main.ZWAMESSAGE WHERE ZCHATSESSION = :s) WHERE ZSORT IS NOT n",
		sql.Named("s", session))
	v.r.add(name+" is numbered 1 to N in date order", ok && gaps == 0,
		fmt.Sprintf("%d out of place", gaps))

	// The session points at its own last message, and that message points back.
	pointers, ok := v.scalar(
		"SELECT count(*) FROM main.ZWACHATSESSION s WHERE s.Z_PK = :s AND s.ZLASTMESSAGE IS NOT "+
			"(SELECT Z_PK FROM main.ZWAMESSAGE WHERE ZCHATSESSION = :s ORDER BY ZSORT DESC LIMIT 1)",
		sql.Named("s", session))
	v.r.add(name+" points at its own last message", ok && pointers == 0)

	// The counter must not fall below the highest position, or the next message the
	// phone writes lands on top of one that is already there.
	//
	// Not strictly above, which is what this demanded at first. The prototype's own
	// validator wanted counter >= n+1, but the store it produced — the one that was
	// restored onto a phone that is still in use — has the counter exactly equal to
	// the highest position, and that phone is fine. Direct evidence that equality is
	// safe beats an inference about what the field means, and a check that blocks
	// work already known to be good is worse than no check: it teaches people to
	// ignore the report.
	counter, okCounter := v.scalar(
		"SELECT coalesce(ZMESSAGECOUNTER, 0) FROM main.ZWACHATSESSION WHERE Z_PK = :s",
		sql.Named("s", session))
	highest, okHighest := v.scalar(
		"SELECT coalesce(max(ZSORT), 0) FROM main.ZWAMESSAGE WHERE ZCHATSESSION = :s",
		sql.Named("s", session))
	v.r.add(name+" will not write a new message on top of an old one",
		okCounter && okHighest && counter >= highest,
		fmt.Sprintf("counter %d, highest position %d", counter, highest))

	// An added message says who it came from or who it went to, never both and never
	// neither. Native group-event rows legitimately carry both, so only added rows
	// are held to this.
	wrong, ok := v.scalar(
		"SELECT count(*) FROM main.ZWAMESSAGE WHERE ZCHATSESSION = :s AND Z_PK > :highest AND ("+
			"(ZISFROMME = 1 AND NOT (ZTOJID IS NOT NULL AND ZFROMJID IS NULL)) OR "+
			"(ZISFROMME = 0 AND NOT (ZFROMJID IS NOT NULL AND ZTOJID IS NULL)))",
		sql.Named("s", session), sql.Named("highest", highestMessage))
	v.r.add(name+" says who each added message was from or to", ok && wrong == 0,
		fmt.Sprintf("%d neither or both", wrong))
}

// counts is what has to be true of the store as a whole once messages were added.
func (v *verifier) counts() {
	highestMessage, ok := v.scalar("SELECT coalesce(max(Z_PK), 0) FROM original.ZWAMESSAGE")
	if !ok {
		return
	}

	// An orphan can pre-exist in a real store, where a conversation was deleted and
	// its messages were not. Only new ones are this migration's doing.
	orphans, ok := v.scalar(
		"SELECT count(*) FROM main.ZWAMESSAGE m LEFT JOIN main.ZWACHATSESSION s ON s.Z_PK = m.ZCHATSESSION "+
			"WHERE s.Z_PK IS NULL AND m.Z_PK > :highest", sql.Named("highest", highestMessage))
	v.r.add("no message was added without a conversation to belong to", ok && orphans == 0,
		fmt.Sprintf("%d orphaned", orphans))

	if v.has("ZWAGROUPMEMBER") && v.hasColumn("ZWAMESSAGE", "ZGROUPMEMBER") {
		loose, ok := v.scalar(
			"SELECT count(*) FROM main.ZWAGROUPMEMBER g LEFT JOIN main.ZWACHATSESSION s ON s.Z_PK = g.ZCHATSESSION " +
				"WHERE s.Z_PK IS NULL")
		v.r.add("every group member belongs to a conversation", ok && loose == 0)

		crossed, ok := v.scalar(
			"SELECT count(*) FROM main.ZWAMESSAGE m JOIN main.ZWAGROUPMEMBER g ON g.Z_PK = m.ZGROUPMEMBER "+
				"WHERE g.ZCHATSESSION IS NOT m.ZCHATSESSION AND m.Z_PK > :highest",
			sql.Named("highest", highestMessage))
		v.r.add("no added message credits somebody from another conversation", ok && crossed == 0)
	}

	// The same message twice in the same conversation is the failure this whole
	// feature is judged on.
	//
	// Within a conversation, not across the store. WhatsApp's identifier for a
	// message is unique to the conversation it is in, not globally: the same one
	// appears in somebody's own chat and in a group, legitimately and often. On a
	// real archive that is 32,664 identifiers shared between exactly two
	// conversations each, and none repeated inside one. The prototype's validator
	// asked the global question and got away with it because the migration it was
	// run against touched a single conversation.
	before, okBefore := v.scalar(
		"SELECT count(*) FROM (SELECT ZCHATSESSION, ZSTANZAID FROM original.ZWAMESSAGE " +
			"WHERE ZSTANZAID IS NOT NULL GROUP BY 1, 2 HAVING count(*) > 1)")
	after, okAfter := v.scalar(
		"SELECT count(*) FROM (SELECT ZCHATSESSION, ZSTANZAID FROM main.ZWAMESSAGE " +
			"WHERE ZSTANZAID IS NOT NULL GROUP BY 1, 2 HAVING count(*) > 1)")
	v.r.add("no message appears twice in a conversation that did not already",
		okBefore && okAfter && after <= before,
		fmt.Sprintf("%d duplicated before, %d after", before, after))

	blank, ok := v.scalar(
		"SELECT count(*) FROM main.ZWAMESSAGE WHERE Z_PK > :highest AND (ZTEXT IS NULL OR ZTEXT = '')",
		sql.Named("highest", highestMessage))
	v.r.add("every added message has something to show", ok && blank == 0,
		fmt.Sprintf("%d blank", blank))

	twice, ok := v.scalar(
		"SELECT count(*) FROM (SELECT ZCONTACTJID FROM main.ZWACHATSESSION " +
			"WHERE ZREMOVED = 0 AND ZSESSIONTYPE IN (0, 1) AND ZCONTACTJID IS NOT NULL " +
			"GROUP BY 1 HAVING count(*) > 1)")
	v.r.add("nobody appears in the conversation list twice", ok && twice == 0,
		fmt.Sprintf("%d duplicated", twice))
}
