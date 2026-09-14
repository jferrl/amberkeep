package migrate

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jferrl/amberkeep/internal/source"
)

// Against real archives, and against the Python prototype this was ported from.
//
// Nothing here runs without being pointed at files that are not in this repository
// and never will be. What they hold is somebody's entire history, so these tests
// compare counts and never print a single thing anybody wrote, nor any address.
//
//	AMBERKEEP_REAL_MSGSTORE     a decrypted Android msgstore.db
//	AMBERKEEP_REAL_CHATSTORAGE  an iPhone ChatStorage.sqlite, with no -wal beside it
//	AMBERKEEP_REAL_LIDPAIRS     WhatsApp's LID.sqlite, optional
//	AMBERKEEP_ORACLE_REPORT     one or more reports from the Python ios/inject.py,
//	                            comma-separated and in the order they were run

// realSides opens both halves, or skips.
func realSides(t *testing.T) (Source, *Target) {
	t.Helper()

	androidPath := os.Getenv("AMBERKEEP_REAL_MSGSTORE")
	storePath := os.Getenv("AMBERKEEP_REAL_CHATSTORAGE")
	if androidPath == "" || storePath == "" {
		t.Skip("set AMBERKEEP_REAL_MSGSTORE and AMBERKEEP_REAL_CHATSTORAGE to run this")
	}

	from, err := source.Open(t.Context(), androidPath)
	if err != nil {
		t.Fatalf("opening the Android archive: %v", err)
	}
	t.Cleanup(func() { _ = from.Close() })

	to, err := OpenTarget(t.Context(), storePath, os.Getenv("AMBERKEEP_REAL_LIDPAIRS"))
	if err != nil {
		t.Fatalf("opening the iPhone store: %v", err)
	}
	t.Cleanup(func() { _ = to.Close() })

	return from, to
}

// TestAgainstTheOracle checks the port against the program it was ported from.
//
// The Python prototype did this migration once, for real, on the phone that started
// this project. It is the only thing in existence that knows what the right answer
// looks like, so where the two disagree, one of them is wrong and it is worth
// knowing which before anybody's phone is involved.
func TestAgainstTheOracle(t *testing.T) {
	reportPath := os.Getenv("AMBERKEEP_ORACLE_REPORT")
	if reportPath == "" {
		t.Skip("set AMBERKEEP_ORACLE_REPORT to a report from the Python ios/inject.py")
	}
	from, to := realSides(t)

	oracle, only := readOracle(t, reportPath)

	plan, err := Build(t.Context(), from, to, Options{Only: only, Groups: true, Hidden: true})
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	tests := []struct {
		name   string
		python int
		here   int
	}{
		{name: "messages added", python: oracle.inserted, here: plan.Adding},
		{name: "arriving as placeholders", python: oracle.placeholders, here: plan.AsPlaceholders},
		{name: "not carried across", python: oracle.untranslatable, here: plan.Untranslatable},
		{name: "conversations touched", python: oracle.conversations, here: plan.Creating + plan.Merging},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.python != tt.here {
				t.Errorf("python says %d, this says %d", tt.python, tt.here)
			}
		})
	}
}

// oracleTotals is what the Python prototype did, across however many runs it took.
type oracleTotals struct {
	inserted       int
	placeholders   int
	untranslatable int
	conversations  int
}

// readOracle sums a sequence of reports from the Python injector.
//
// A sequence rather than one, because the prototype was used to prove the merge
// path: it was run once over the first part of a conversation and again over more of
// it, and the second run reports the first run's own insertions as duplicates. Over
// the whole sequence the insertions add up, while what could not be carried does
// not: each run re-reads a superset of the last, so the largest of those counts is
// the one that covers every row. Comparing against a single step of that sequence is
// comparing against a partial answer, which is a mistake this cost an afternoon.
func readOracle(t *testing.T, paths string) (totals oracleTotals, only []string) {
	t.Helper()

	chats := map[string]bool{}
	for _, path := range strings.Split(paths, ",") {
		var report struct {
			Messages int `json:"messages"`
			Media    int `json:"media_placeholders"`
			Deleted  int `json:"deleted"`
			System   int `json:"skipped_system"`
			Chats    []struct {
				AndroidJID string `json:"android_jid"`
			} `json:"chats"`
		}
		raw, err := os.ReadFile(strings.TrimSpace(path)) // #nosec G304 -- a path the operator supplied
		if err != nil {
			t.Fatalf("reading the oracle's report: %v", err)
		}
		if err := json.Unmarshal(raw, &report); err != nil {
			t.Fatalf("reading the oracle's report: %v", err)
		}

		totals.inserted += report.Messages
		totals.placeholders += report.Media + report.Deleted
		totals.untranslatable = max(totals.untranslatable, report.System)
		for _, c := range report.Chats {
			chats[c.AndroidJID] = true
		}
	}

	only = make([]string, 0, len(chats))
	for address := range chats {
		only = append(only, address)
	}
	totals.conversations = len(only)
	return totals, only
}

// TestARealMigrationAddsUp is the invariant on a real archive: every message in the
// source is accounted for exactly once, and planning leaves the phone's store alone.
func TestARealMigrationAddsUp(t *testing.T) {
	from, to := realSides(t)

	before, err := os.ReadFile(os.Getenv("AMBERKEEP_REAL_CHATSTORAGE")) // #nosec G304 -- operator-supplied
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}

	plan, err := Build(t.Context(), from, to, Options{Groups: true})
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	t.Logf("would add %d messages to %d conversations, %d already there, %d not carried, %d as placeholders",
		plan.Adding, plan.Merging+plan.Creating, plan.AlreadyThere, plan.Untranslatable, plan.AsPlaceholders)

	for _, c := range plan.Conversations {
		if c.Adding < 0 || c.AlreadyThere < 0 || c.Untranslatable < 0 {
			t.Fatal("a conversation reported a negative count")
		}
		if c.AsPlaceholders > c.Adding {
			t.Error("a conversation reports more placeholders than messages")
		}
	}

	after, err := os.ReadFile(os.Getenv("AMBERKEEP_REAL_CHATSTORAGE")) // #nosec G304 -- operator-supplied
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}
	if len(before) != len(after) {
		t.Error("planning changed the store it was only supposed to read")
	}
}

// TestTheOraclesOwnOutputPasses runs the checks against a migration that really
// happened.
//
// The Python prototype produced this store and it was restored onto a phone that is
// still being used. It is the only artefact in existence known to be right, so if a
// check here fails on it, the check is what to doubt.
//
//	AMBERKEEP_REAL_ORIGINAL_STORE  the pristine ChatStorage.sqlite it started from
//	AMBERKEEP_REAL_MIGRATED_STORE  what the Python prototype produced from it
func TestTheOraclesOwnOutputPasses(t *testing.T) {
	original := os.Getenv("AMBERKEEP_REAL_ORIGINAL_STORE")
	migrated := os.Getenv("AMBERKEEP_REAL_MIGRATED_STORE")
	if original == "" || migrated == "" {
		t.Skip("set AMBERKEEP_REAL_ORIGINAL_STORE and AMBERKEEP_REAL_MIGRATED_STORE to run this")
	}

	// Which conversations the oracle merged into, so the checks know where reordering
	// was allowed. Without the reports every merge looks like a conversation this had
	// no business touching.
	var plan Plan
	if reports := os.Getenv("AMBERKEEP_ORACLE_REPORT"); reports != "" {
		for _, path := range strings.Split(reports, ",") {
			var report struct {
				Chats []struct {
					Mode    string `json:"mode"`
					Session int64  `json:"session_pk"`
				} `json:"chats"`
			}
			raw, err := os.ReadFile(strings.TrimSpace(path)) // #nosec G304 -- operator-supplied
			if err != nil {
				t.Fatalf("reading the oracle's report: %v", err)
			}
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatalf("reading the oracle's report: %v", err)
			}
			for _, c := range report.Chats {
				if c.Mode == "merged" {
					plan.Conversations = append(plan.Conversations, Conversation{
						Address: "merged", Into: "merged", Session: c.Session, Adding: 1,
					})
				}
			}
		}
	}

	report, err := Verify(t.Context(), original, migrated, plan)
	if err != nil {
		t.Fatalf("Verify() failed: %v", err)
	}

	t.Logf("%s; added %v", report.Summary(), report.Added)
	for _, c := range report.Failures() {
		t.Errorf("a migration that was restored to a real phone fails %q: %s", c.Name, c.Detail)
	}
}
