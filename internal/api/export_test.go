package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Writing an archive out, from a page.
//
// The command has done this since the beginning; what is tested here is that the
// page can, that it says what it produced, and that it cannot be asked to do it
// twice at once into the same folder.

// writer is an Archive that can also write itself out.
type writer struct {
	*stub

	into   string
	failed error
	// gate, when set, holds the writing open so a test can look at it running.
	gate chan struct{}
}

func (w *writer) Export(_ context.Context, ask ExportRequest, say Progress) (Exported, error) {
	if w.gate != nil {
		<-w.gate
	}
	if w.failed != nil {
		return Exported{}, w.failed
	}
	say(StepWriting, "25 of 60 conversations written.")
	w.into = ask.Into

	// Something on disk, so a test can assert that the answer is not a fiction.
	if err := os.MkdirAll(ask.Into, 0o750); err != nil {
		return Exported{}, err
	}
	name := filepath.Join(ask.Into, "index.html")
	if err := os.WriteFile(name, []byte("<!doctype html>"), 0o600); err != nil {
		return Exported{}, err
	}
	return Exported{
		Into: ask.Into, Conversations: 2, Messages: 41, Bytes: 15, Formats: ask.Formats,
	}, nil
}

// exporting returns a server with an archive open that can write itself out.
func writing(t *testing.T, out *writer) http.Handler {
	t.Helper()

	handler, err := New(context.Background(), out, Options{
		Location: time.UTC, Workspace: t.TempDir(), Me: "You",
		Advise: func(err error) string { return "what to do about: " + err.Error() },
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() { _ = handler.Close() })
	return handler
}

// reachesExport waits for the export to reach a stage.
func reachesExport(t *testing.T, handler http.Handler, want string) map[string]any {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		body := ask(t, handler, "/api/export")
		if body["stage"] == want {
			return body
		}
		if time.Now().After(deadline) {
			t.Fatalf("the export stayed at %v; want %s (%v)", body["stage"], want, body["detail"])
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAnArchiveCanBeWrittenOutFromAPage(t *testing.T) {
	t.Parallel()

	out := &writer{stub: &stub{}}
	handler := writing(t, out)

	t.Run("it starts having written nothing", func(t *testing.T) {
		if got := ask(t, handler, "/api/export")["stage"]; got != string(ExportIdle) {
			t.Errorf("stage = %v, want %s", got, ExportIdle)
		}
	})

	t.Run("and says what it produced", func(t *testing.T) {
		into := filepath.Join(t.TempDir(), "archive")
		status, answer := post(t, handler, "/api/export", map[string]any{
			"into": into, "formats": []string{"html", "text"}, "groups": true,
		})
		if status != http.StatusAccepted {
			t.Fatalf("the request was not accepted: %d", status)
		}
		// The reply is the export's own state, not some other job's.
		if answer["stage"] != string(ExportWriting) {
			t.Errorf("it answered with stage %v, want %s", answer["stage"], ExportWriting)
		}

		state := reachesExport(t, handler, string(ExportDone))
		result, ok := state["result"].(map[string]any)
		if !ok {
			t.Fatalf("it did not say what it produced: %v", state["result"])
		}
		if result["into"] != into {
			t.Errorf("into = %v, want %s", result["into"], into)
		}
		if result["messages"] != float64(41) {
			t.Errorf("messages = %v, want 41", result["messages"])
		}

		// And the folder it named actually holds something.
		if _, err := os.Stat(filepath.Join(into, "index.html")); err != nil {
			t.Errorf("it reported a folder with nothing in it: %v", err)
		}
	})

	t.Run("and can be asked again", func(t *testing.T) {
		status, _ := post(t, handler, "/api/export/forget", map[string]string{})
		if status != http.StatusOK {
			t.Fatalf("forgetting returned %d", status)
		}
		if got := ask(t, handler, "/api/export")["stage"]; got != string(ExportIdle) {
			t.Errorf("stage = %v, want %s", got, ExportIdle)
		}
	})
}

func TestAnExportNeedsAFormatAndAnArchive(t *testing.T) {
	t.Parallel()

	t.Run("no formats is refused rather than guessed at", func(t *testing.T) {
		t.Parallel()

		handler := writing(t, &writer{stub: &stub{}})
		if status, _ := post(t, handler, "/api/export",
			map[string]any{"formats": []string{}}); status != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
		}
	})

	t.Run("no archive open is a conflict, not a crash", func(t *testing.T) {
		t.Parallel()

		handler, err := New(context.Background(), nil, Options{
			Location: time.UTC, Workspace: t.TempDir(), Importer: &helper{},
		})
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		t.Cleanup(func() { _ = handler.Close() })

		if status, _ := post(t, handler, "/api/export",
			map[string]any{"formats": []string{"html"}}); status != http.StatusConflict {
			t.Errorf("status = %d, want %d", status, http.StatusConflict)
		}
	})
}

// TestOnlyOneExportAtATime covers two writes into one folder, which would race over
// the same filenames and leave a reader with half of each.
func TestOnlyOneExportAtATime(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	handler := writing(t, &writer{stub: &stub{}, gate: gate})

	// The folder first, so that the cleanup below is registered after the one that
	// removes it and therefore runs before it. Cleanups run last-registered-first,
	// and the work here deliberately outlives the request that started it: released
	// any later, the export would be writing its files into a directory the test
	// framework was in the middle of deleting. On Unix that is invisible; Windows
	// reports the directory it could not empty and fails the run.
	into := t.TempDir()
	t.Cleanup(func() {
		close(gate)
		reachesExport(t, handler, string(ExportDone))
	})

	first := map[string]any{"into": into, "formats": []string{"html"}}
	if status, _ := post(t, handler, "/api/export", first); status != http.StatusAccepted {
		t.Fatal("the first request was not accepted")
	}
	reachesExport(t, handler, string(ExportWriting))

	if status, _ := post(t, handler, "/api/export", first); status != http.StatusConflict {
		t.Errorf("a second export was accepted while one was running")
	}
}

func TestAnExportThatFailsSaysWhatToDo(t *testing.T) {
	t.Parallel()

	handler := writing(t, &writer{stub: &stub{}, failed: errors.New("that folder is not writable")})
	if status, _ := post(t, handler, "/api/export",
		map[string]any{"into": t.TempDir(), "formats": []string{"html"}}); status != http.StatusAccepted {
		t.Fatal("the request was not accepted")
	}

	state := reachesExport(t, handler, string(ExportFailed))
	if detail, _ := state["detail"].(string); detail == "" {
		t.Error("it did not say what went wrong")
	}
	if guidance, _ := state["guidance"].(string); !strings.HasPrefix(guidance, "what to do about: ") {
		t.Errorf("guidance = %q", guidance)
	}
}
