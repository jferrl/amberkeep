package migrate

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/guide"
)

// TestEveryFindingPointsAtSomethingThatExists is the cross-check worth having.
//
// A finding names the guided step that says what to do about it. A finding naming a
// step that is not there shows somebody a failure and no way to fix it, and nothing
// else would catch that: both halves compile perfectly.
func TestEveryFindingPointsAtSomethingThatExists(t *testing.T) {
	t.Parallel()

	// Every path through Preflight, so every finding it can emit is seen.
	cases := []func(t *testing.T) string{
		func(t *testing.T) string { t.Helper(); return "" },          // not a backup at all
		func(t *testing.T) string { t.Helper(); return t.TempDir() }, // an empty folder
	}

	seen := map[string]bool{}
	for _, where := range cases {
		ready, err := Preflight(context.Background(), where(t), "any", "any")
		if err != nil {
			continue
		}
		for _, f := range ready.Findings {
			seen[f.Step] = true
		}
	}
	// And the findings from a backup that is fine, which only a fixture can produce.
	for _, step := range []string{"encryption-off", "fresh-backup", "power-and-space", "safety-backup"} {
		seen[step] = true
	}

	for step := range seen {
		if step == "" {
			t.Error("a finding names no step, so nothing can say what to do about it")
			continue
		}
		if _, ok := guide.Find(step, guide.English); !ok {
			t.Errorf("a finding points at %q, and the guide has no such step", step)
		}
	}
}

func TestPreflightRefusesWhatCannotBeMigrated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		where func(t *testing.T) string
		says  string
	}{
		{
			name:  "a folder that is not a backup",
			where: func(t *testing.T) string { t.Helper(); return t.TempDir() },
			says:  "four files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ready, err := Preflight(context.Background(), tt.where(t), "any", "any")
			if err != nil {
				t.Fatalf("Preflight() failed: %v", err)
			}
			if ready.OK() {
				t.Fatal("it said this was ready to migrate")
			}
			blockers := ready.Blockers()
			if len(blockers) == 0 {
				t.Fatal("nothing was blocking, and nothing was ready either")
			}
			if !strings.Contains(blockers[0].Detail, tt.says) {
				t.Errorf("it said %q", blockers[0].Detail)
			}
		})
	}
}

// TestNothingCheckedIsNotReady covers the shape a green result takes when nothing ran.
func TestNothingCheckedIsNotReady(t *testing.T) {
	t.Parallel()

	var nothing Readiness
	if nothing.OK() {
		t.Error("a readiness that checked nothing called itself ready")
	}
}

// TestHowOldTheBackupIsSaid covers a sentence somebody acts on: anything said on the
// phone after the backup was taken is not in it and will be gone after the restore,
// so how old it is decides whether they go and take another.
func TestHowOldTheBackupIsSaid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		age  time.Duration
		zero bool
		says string
	}{
		{name: "minutes ago", age: 20 * time.Minute, says: "within the hour"},
		{name: "this morning", age: 5 * time.Hour, says: "5 hours ago"},
		{name: "exactly one hour", age: time.Hour, says: "1 hour ago"},
		{name: "yesterday", age: 30 * time.Hour, says: "1 day ago"},
		{name: "last week", age: 8 * 24 * time.Hour, says: "8 days ago"},
		{name: "a backup that does not say", zero: true, says: "does not say when"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			at := time.Now().Add(-tt.age)
			if tt.zero {
				at = time.Time{}
			}
			got := freshness(at, tt.age)
			if !strings.Contains(got, tt.says) {
				t.Errorf("freshness said %q, want something about %q", got, tt.says)
			}
			// Anything older than a day has to say why it matters, not just how old.
			if !tt.zero && tt.age >= staleAfter && !strings.Contains(got, "not in it") {
				t.Errorf("a stale backup does not say what that costs: %q", got)
			}
		})
	}
}

func TestDetailIsOnlyGivenWhenThereIsSomethingToSay(t *testing.T) {
	t.Parallel()

	if got := detailIf(true, "because"); got != "because" {
		t.Errorf("detailIf(true) = %q", got)
	}
	if got := detailIf(false, "because"); got != "" {
		t.Errorf("detailIf(false) = %q, want nothing", got)
	}
}

// TestPreflightOnABackupThatIsFine is the path every migration takes first.
func TestPreflightOnABackupThatIsFine(t *testing.T) {
	t.Parallel()

	const (
		domain = "AppDomainGroup-group.net.whatsapp.WhatsApp.shared"
		store  = "ChatStorage.sqlite"
	)

	t.Run("a recent backup holding the messages", func(t *testing.T) {
		t.Parallel()

		backup := backupHolding(t, domain, store, 2_453_504, time.Now().Add(-2*time.Hour))
		ready, err := Preflight(context.Background(), backup, domain, store)
		if err != nil {
			t.Fatalf("Preflight() failed: %v", err)
		}
		if !ready.OK() {
			t.Fatalf("it refused a backup that is fine: %v", ready.Blockers())
		}
		if ready.Needs == 0 {
			t.Error("it did not say how much room a copy would take")
		}

		// The one that cannot be checked still has to be raised, every time.
		var raised bool
		for _, f := range ready.Findings {
			if f.Step == "safety-backup" {
				raised = true
			}
		}
		if !raised {
			t.Error("it never mentioned the safety backup, which is the only way back")
		}
	})

	t.Run("a backup that does not hold the messages", func(t *testing.T) {
		t.Parallel()

		backup := backupHolding(t, domain, "something-else.sqlite", 10, time.Now())
		ready, err := Preflight(context.Background(), backup, domain, store)
		if err != nil {
			t.Fatalf("Preflight() failed: %v", err)
		}
		if ready.OK() {
			t.Error("it accepted a backup with no WhatsApp messages in it")
		}
	})

	t.Run("a backup old enough that something would be lost", func(t *testing.T) {
		t.Parallel()

		backup := backupHolding(t, domain, store, 10, time.Now().Add(-3*24*time.Hour))
		ready, err := Preflight(context.Background(), backup, domain, store)
		if err != nil {
			t.Fatalf("Preflight() failed: %v", err)
		}
		// Worth saying, not worth blocking: somebody who has said nothing since does
		// not need to take another backup.
		if !ready.OK() {
			t.Error("it blocked on an old backup rather than mentioning it")
		}
		var warned bool
		for _, f := range ready.Findings {
			if !f.Passed && strings.Contains(f.Detail, "not in it") {
				warned = true
			}
		}
		if !warned {
			t.Error("it did not say that anything said since is not in the backup")
		}
	})
}
