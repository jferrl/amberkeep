package migrate

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jferrl/amberkeep/internal/model"
)

// applied runs a plan and returns where it landed, insisting it worked.
func applied(t *testing.T, from Source, p *phone, opts Options) (Result, Plan) {
	t.Helper()

	plan, err := Build(context.Background(), from, p.open(t, ""), opts)
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	into := filepath.Join(t.TempDir(), "ChatStorage.migrated.sqlite")
	result, err := Apply(context.Background(), from, p.path, into, plan)
	if err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}
	return result, plan
}

// checked insists the result passes every check, which is the whole point of having
// written them first.
func checked(t *testing.T, original string, result Result, plan Plan) Report {
	t.Helper()

	report, err := Verify(context.Background(), original, result.Path, plan)
	if err != nil {
		t.Fatalf("Verify() failed: %v", err)
	}
	for _, c := range report.Failures() {
		t.Errorf("what was written fails %q: %s", c.Name, c.Detail)
	}
	return report
}

// TestWhatIsWrittenPassesEveryCheck is the test the whole order of work was for.
//
// The checks were written first, against stores damaged by hand, so this is not two
// pieces of code agreeing with each other: it is a writer meeting a standard that
// already existed.
func TestWhatIsWrittenPassesEveryCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		phone func(t *testing.T) *phone
		from  *android
		opts  Options
	}{
		{
			name:  "a conversation the phone has never seen",
			phone: buildPhone,
			from:  history(said(1, "K1", 0, "one"), said(2, "K2", 1, "two")),
		},
		{
			name: "a conversation they both have",
			phone: func(t *testing.T) *phone {
				t.Helper()
				p := buildPhone(t)
				p.holds(t, ana.String(), 0, "K1")
				return p
			},
			from: history(said(1, "K1", 0, "one"), said(2, "K2", 1, "two"), said(3, "K3", 2, "three")),
		},
		{
			name:  "a message of every kind that can be carried",
			phone: buildPhone,
			from: history(
				said(1, "K1", 0, "words"),
				sent(2, "K2", 1, model.KindImage),
				sent(3, "K3", 2, model.KindDeleted),
				sent(4, "K4", 3, model.KindVoice),
			),
		},
		{
			name:  "a group",
			phone: buildPhone,
			from: history(said(1, "K1", 0, "one")).
				with(model.Chat{ID: 2, JID: group, Kind: model.ChatGroup, Name: "Vermut"},
					said(10, "G1", 0, "one"), said(11, "G2", 1, "two")),
			opts: Options{Groups: true},
		},
		{
			name:  "several conversations at once",
			phone: buildPhone,
			from: history(said(1, "K1", 0, "one")).
				with(model.Chat{ID: 5, JID: luis, Kind: model.ChatDirect, Name: "Luis"},
					said(50, "L1", 0, "hello"), said(51, "L2", 5, "again")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := tt.phone(t)
			p.wal(t)
			before, err := os.ReadFile(p.path)
			if err != nil {
				t.Fatalf("reading the store: %v", err)
			}

			result, plan := applied(t, tt.from, p, tt.opts)
			report := checked(t, p.path, result, plan)

			if !report.OK() {
				t.Fatalf("summary: %s", report.Summary())
			}
			if result.Added != plan.Adding {
				t.Errorf("wrote %d messages, planned %d", result.Added, plan.Adding)
			}

			// The original is the phone's own store. Nothing may have happened to it.
			after, err := os.ReadFile(p.path)
			if err != nil {
				t.Fatalf("reading the store: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Error("the original store was changed")
			}
			for _, side := range []string{"-wal", "-shm"} {
				if _, err := os.Stat(result.Path + side); err == nil {
					t.Errorf("a %s was left beside the result", side)
				}
			}
		})
	}
}

// TestTheMessagesArriveWhereTheyBelong covers the part somebody would notice: a
// message from another phone lands among the ones already there, in date order, not
// bolted on at the end.
func TestTheMessagesArriveWhereTheyBelong(t *testing.T) {
	t.Parallel()

	p := buildPhone(t)
	session := p.holds(t, ana.String(), 0, "OLD")
	p.wal(t)

	// The Android side holds one message before the phone's and one after.
	from := history(said(1, "EARLY", -10, "before"), said(2, "LATE", 10, "after"))
	result, plan := applied(t, from, p, Options{})
	checked(t, p.path, result, plan)

	db, err := sql.Open("sqlite", "file:"+result.Path+"?mode=ro&immutable=1")
	if err != nil {
		t.Fatalf("opening the result: %v", err)
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query(
		"SELECT ZSTANZAID FROM ZWAMESSAGE WHERE ZCHATSESSION = ? ORDER BY ZSORT", session)
	if err != nil {
		t.Fatalf("reading the conversation: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var order []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("reading the conversation: %v", err)
		}
		order = append(order, id)
	}

	want := []string{"EARLY", "OLD", "LATE"}
	if len(order) != len(want) {
		t.Fatalf("the conversation holds %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("the conversation reads %v, want %v", order, want)
			break
		}
	}
}

// TestItRefusesRatherThanWriteSomethingNobodyAgreedTo is the safeguard against the
// one thing a person cannot check for themselves.
//
// A plan is what somebody read and said yes to. If the archive changes between
// reading it and running it, what gets written is something nobody agreed to — and
// it would look perfectly fine. So the writer counts what it wrote and throws the
// whole thing away rather than hand back a store that does not match.
func TestItRefusesRatherThanWriteSomethingNobodyAgreedTo(t *testing.T) {
	t.Parallel()

	p := buildPhone(t)
	p.wal(t)

	planned := history(said(1, "K1", 0, "one"))
	plan, err := Build(context.Background(), planned, p.open(t, ""), Options{})
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// The archive gains a message after the plan was agreed to.
	changed := history(said(1, "K1", 0, "one"), said(2, "K2", 1, "two"))

	into := filepath.Join(t.TempDir(), "ChatStorage.migrated.sqlite")
	_, err = Apply(context.Background(), changed, p.path, into, plan)
	if !errors.Is(err, ErrBrokePromise) {
		t.Fatalf("it wrote a store nobody agreed to, and returned %v", err)
	}
	if _, err := os.Stat(into); err == nil {
		t.Error("it left the store it refused to stand behind on disk")
	}
}

func TestWhatItWillNotDo(t *testing.T) {
	t.Parallel()

	t.Run("write over something that is already there", func(t *testing.T) {
		t.Parallel()

		p := buildPhone(t)
		p.wal(t)
		from := history(said(1, "K1", 0, "one"))

		plan, err := Build(context.Background(), from, p.open(t, ""), Options{})
		if err != nil {
			t.Fatalf("Build() failed: %v", err)
		}

		into := filepath.Join(t.TempDir(), "taken.sqlite")
		if err := os.WriteFile(into, []byte("last week's migration"), 0o600); err != nil {
			t.Fatalf("preparing the fixture: %v", err)
		}

		_, err = Apply(context.Background(), from, p.path, into, plan)
		if !errors.Is(err, ErrWouldOverwrite) {
			t.Fatalf("Apply() returned %v, want a refusal", err)
		}
		kept, err := os.ReadFile(into)
		if err != nil || string(kept) != "last week's migration" {
			t.Error("it wrote over what was already there")
		}
	})

	t.Run("produce a copy when there is nothing to add", func(t *testing.T) {
		t.Parallel()

		p := buildPhone(t)
		p.holds(t, ana.String(), 0, "K1")
		p.wal(t)
		from := history(said(1, "K1", 0, "one"))

		plan, err := Build(context.Background(), from, p.open(t, ""), Options{})
		if err != nil {
			t.Fatalf("Build() failed: %v", err)
		}

		into := filepath.Join(t.TempDir(), "pointless.sqlite")
		if _, err := Apply(context.Background(), from, p.path, into, plan); !errors.Is(err, ErrNothingToDo) {
			t.Fatalf("Apply() returned %v, want a refusal", err)
		}
		if _, err := os.Stat(into); err == nil {
			t.Error("it produced an identical copy for no reason")
		}
	})
}

// TestRunningItTwiceOverItsOwnResult is the property a person relies on when they are
// unsure: doing it again changes nothing.
func TestRunningItTwiceOverItsOwnResult(t *testing.T) {
	t.Parallel()

	p := buildPhone(t)
	p.wal(t)
	from := history(said(1, "K1", 0, "one"), said(2, "K2", 1, "two"))

	first, plan := applied(t, from, p, Options{})
	checked(t, p.path, first, plan)

	// Plan again, this time against what was written.
	second, err := OpenTarget(context.Background(), first.Path, "")
	if err != nil {
		t.Fatalf("opening the result: %v", err)
	}
	defer func() { _ = second.Close() }()

	again, err := Build(context.Background(), from, second, Options{})
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	if !again.Empty() {
		t.Errorf("a second run would add %d messages, and should add none", again.Adding)
	}
}

// TestItUsesTheStoresOwnConventions covers version tolerance: the values WhatsApp
// fills its own rows with are copied from the store rather than written down here,
// because a message written with the wrong ones is one the phone may show as still
// sending, years after it was sent.
func TestItUsesTheStoresOwnConventions(t *testing.T) {
	t.Parallel()

	p := buildPhone(t)
	session := p.holds(t, luis.String(), 0, "OLD")
	p.wal(t)

	// A store whose own rows use values nothing in this package would have guessed.
	damage(t, p.path, `UPDATE ZWAMESSAGE SET ZMESSAGETYPE = 0, ZFLAGS = 4242, ZMESSAGESTATUS = 9`)
	_ = session

	from := history(said(1, "K1", 0, "one"))
	result, plan := applied(t, from, p, Options{})
	checked(t, p.path, result, plan)

	if result.Sampled == 0 {
		t.Fatal("it copied none of the store's own conventions")
	}

	db, err := sql.Open("sqlite", "file:"+result.Path+"?mode=ro&immutable=1")
	if err != nil {
		t.Fatalf("opening the result: %v", err)
	}
	defer func() { _ = db.Close() }()

	var flags, status int64
	if err := db.QueryRow(
		"SELECT ZFLAGS, ZMESSAGESTATUS FROM ZWAMESSAGE WHERE ZSTANZAID = 'K1'").Scan(&flags, &status); err != nil {
		t.Fatalf("reading what was written: %v", err)
	}
	if flags != 4242 || status != 9 {
		t.Errorf("it wrote flags %d status %d, want the store's own 4242 and 9", flags, status)
	}
}
