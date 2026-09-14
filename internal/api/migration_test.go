package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/migrate"
)

// Moving a history onto a phone, from a page.
//
// What is tested here is mostly what does not happen: that checking does not begin
// planning, that a plan does not begin writing, and that nothing is written without
// a word typed out. This is the one part of the program that ends with somebody
// restoring a backup onto a phone they depend on, and a page that carries them along
// is a page they can reach the end of without reading anything.

// mover is a Migrator the tests drive.
type mover struct {
	ready migrate.Readiness
	plan  migrate.Plan
	done  Migrated

	failCheck, failPlan, failCarry error

	// gate, when set, holds the work open so a test can look at a stage.
	gate chan struct{}

	mu     sync.Mutex
	asked  []string
	agreed MigrationRequest
}

func (m *mover) record(what string, ask MigrationRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.asked = append(m.asked, what)
	m.agreed = ask
}

func (m *mover) requests() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.asked...)
}

func (m *mover) Check(_ context.Context, backup string) (migrate.Readiness, error) {
	m.record("check", MigrationRequest{Backup: backup})
	return m.ready, m.failCheck
}

func (m *mover) Plan(_ context.Context, ask MigrationRequest, _ Progress) (migrate.Plan, error) {
	m.record("plan", ask)
	if m.gate != nil {
		<-m.gate
	}
	return m.plan, m.failPlan
}

func (m *mover) Carry(_ context.Context, ask MigrationRequest, _ migrate.Plan, _ Progress) (Migrated, error) {
	m.record("carry", ask)
	return m.done, m.failCarry
}

// migrating returns a server with a migrator behind it.
func migrating(t *testing.T, move Migrator) *Server {
	t.Helper()

	handler, err := New(context.Background(), nil, Options{
		Location: time.UTC, Workspace: "/tmp/amberkeep-test",
		Importer: &helper{}, Migrator: move,
		Advise: func(err error) string { return "what to do about: " + err.Error() },
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() { _ = handler.Close() })
	return handler
}

// reaches waits for the migration to reach a stage.
func reaches(t *testing.T, handler http.Handler, want string) map[string]any {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		body := ask(t, handler, "/api/migration")
		if body["stage"] == want {
			return body
		}
		if time.Now().After(deadline) {
			t.Fatalf("the migration stayed at %v; want %s (%v)", body["stage"], want, body["detail"])
		}
		time.Sleep(time.Millisecond)
	}
}

// TestNothingMovesOnItsOwn is the whole design in one test.
func TestNothingMovesOnItsOwn(t *testing.T) {
	t.Parallel()

	move := &mover{
		ready: migrate.Readiness{Findings: []migrate.Finding{
			{Step: "encryption-off", Title: "The backup is not encrypted", Passed: true, Blocking: true},
		}},
		plan: migrate.Plan{Adding: 12, Creating: 2, Conversations: []migrate.Conversation{{Adding: 12}}},
		done: Migrated{Backup: "/tmp/patched", Added: 12, Created: 2, Checks: 31, Files: 27352},
	}
	handler := migrating(t, move)

	t.Run("it starts having done nothing", func(t *testing.T) {
		if got := ask(t, handler, "/api/migration")["stage"]; got != string(MigrationIdle) {
			t.Errorf("stage = %v, want %s", got, MigrationIdle)
		}
	})

	t.Run("checking stops at the checks", func(t *testing.T) {
		if status, _ := post(t, handler, "/api/migration/check",
			map[string]string{"backup": "/backups/abc"}); status != http.StatusAccepted {
			t.Fatal("the request was not accepted")
		}
		state := reaches(t, handler, string(MigrationChecked))
		if state["checks"] == nil {
			t.Error("it did not say what the checks found")
		}
		if state["plan"] != nil {
			t.Error("it worked out a plan without being asked")
		}
	})

	t.Run("planning stops at the plan", func(t *testing.T) {
		if status, _ := post(t, handler, "/api/migration/plan", map[string]string{
			"backup": "/backups/abc", "android": "/tmp/msgstore.db",
		}); status != http.StatusAccepted {
			t.Fatal("the request was not accepted")
		}
		state := reaches(t, handler, string(MigrationPlanned))
		if state["plan"] == nil {
			t.Fatal("it did not say what would move")
		}
		if state["result"] != nil {
			t.Error("it wrote something without being asked")
		}
	})

	t.Run("and writing needs the word", func(t *testing.T) {
		if status, _ := post(t, handler, "/api/migration/carry-out",
			map[string]string{"confirm": "yes"}); status != http.StatusBadRequest {
			t.Errorf("it accepted %q as agreement", "yes")
		}
		if got := ask(t, handler, "/api/migration")["stage"]; got != string(MigrationPlanned) {
			t.Errorf("stage = %v; nothing should have moved", got)
		}

		if status, _ := post(t, handler, "/api/migration/carry-out",
			map[string]string{"confirm": theWord}); status != http.StatusAccepted {
			t.Fatal("it refused the word")
		}
		state := reaches(t, handler, string(MigrationDone))

		result, ok := state["result"].(map[string]any)
		if !ok || result["backup"] != "/tmp/patched" {
			t.Fatalf("it did not say what it produced: %v", state["result"])
		}
		if result["checks"] != float64(31) {
			t.Errorf("it did not say how much was checked: %v", result["checks"])
		}
	})

	t.Run("in that order and no other", func(t *testing.T) {
		want := []string{"check", "plan", "carry"}
		got := move.requests()
		if len(got) != len(want) {
			t.Fatalf("the migrator was asked for %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("the migrator was asked for %v, want %v", got, want)
				break
			}
		}
	})
}

func TestItWillNotWriteWithoutAPlan(t *testing.T) {
	t.Parallel()

	handler := migrating(t, &mover{})
	if status, _ := post(t, handler, "/api/migration/carry-out",
		map[string]string{"confirm": theWord}); status != http.StatusConflict {
		t.Errorf("it agreed to carry out a plan that does not exist: %d", status)
	}
}

func TestAMigrationThatFailsSaysWhatToDo(t *testing.T) {
	t.Parallel()

	handler := migrating(t, &mover{failCheck: errors.New("this backup is encrypted")})
	if status, _ := post(t, handler, "/api/migration/check",
		map[string]string{"backup": "/backups/abc"}); status != http.StatusAccepted {
		t.Fatal("the request was not accepted")
	}

	state := reaches(t, handler, string(MigrationFailed))
	if detail, _ := state["detail"].(string); detail == "" {
		t.Error("it did not say what went wrong")
	}
	if guidance, _ := state["guidance"].(string); !strings.HasPrefix(guidance, "what to do about: ") {
		t.Errorf("guidance = %q", guidance)
	}
}

func TestOnlyOneMigrationAtATime(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	handler := migrating(t, &mover{gate: gate, plan: migrate.Plan{Adding: 1}})
	t.Cleanup(func() { close(gate) })

	first := map[string]string{"backup": "/backups/abc", "android": "/tmp/msgstore.db"}
	if status, _ := post(t, handler, "/api/migration/plan", first); status != http.StatusAccepted {
		t.Fatal("the first request was not accepted")
	}
	reaches(t, handler, string(MigrationPlanning))

	if status, _ := post(t, handler, "/api/migration/plan", first); status != http.StatusConflict {
		t.Errorf("a second request returned %d, want %d", status, http.StatusConflict)
	}
}

// TestTheGuideComesFromTheProgram covers the words living in one place. A copy in the
// page would drift from this one the first time anybody corrected a sentence.
func TestTheGuideComesFromTheProgram(t *testing.T) {
	t.Parallel()

	body := ask(t, migrating(t, &mover{}), "/api/migration/guide")
	stages, ok := body["stages"].([]any)
	if !ok || len(stages) != 4 {
		t.Fatalf("the guide came back as %v", body["stages"])
	}

	var steps, critical int
	for _, s := range stages {
		stage, _ := s.(map[string]any)
		if stage["heading"] == "" {
			t.Error("a stage has no heading")
		}
		within, _ := stage["steps"].([]any)
		steps += len(within)
		for _, one := range within {
			step, _ := one.(map[string]any)
			if step["id"] == "" || step["title"] == "" || step["body"] == "" {
				t.Errorf("a step is missing something: %v", step)
			}
			if step["critical"] == true {
				critical++
			}
		}
	}
	if steps < 12 {
		t.Errorf("the guide carries %d steps, which is fewer than it should", steps)
	}
	if critical != 3 {
		t.Errorf("%d steps are marked as losing something if skipped, want 3", critical)
	}
}

func TestAServerWithoutAMigrator(t *testing.T) {
	t.Parallel()

	// An archive-only build leaves the whole thing off rather than offering it and
	// then failing.
	handler := serve(t, fixture())
	for _, path := range []string{"/api/migration/check", "/api/migration/plan", "/api/migration/carry-out"} {
		t.Run(path, func(t *testing.T) {
			if status, _ := post(t, handler, path, map[string]string{}); status != http.StatusNotImplemented {
				t.Errorf("POST %s returned %d, want %d", path, status, http.StatusNotImplemented)
			}
		})
	}
}
