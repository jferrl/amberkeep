package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The wizard, which is the part somebody uses before there is anything to read.
//
// Everything else in this package assumes an archive. These tests assume the
// opposite, because that is the situation the program exists for: a phone that has
// died, a backup nobody knows the shape of, and a person who needs to be walked
// from one to the other without a developer sitting next to them.

// helper is an importer the tests drive.
type helper struct {
	backups []Backup
	problem string
	// nowhere makes this a platform Apple ships nothing for, where there is no
	// folder to look in rather than no backup in the folder.
	nowhere bool

	// produces is what a finished piece of work hands back, and opens is the
	// archive reading that path is to give.
	produces string
	opens    func() Archive

	failWork error
	failOpen error

	// says is reported as the work runs, so a test can see what somebody watching
	// would have been told.
	says []Note

	// gate, when set, holds the work open until it is closed, which is how a test
	// looks at a server with something still running.
	gate chan struct{}

	// handing, when set, chooses which archive each call to Open returns.
	handing atomic.Int64

	mu    sync.Mutex
	asked []string
	saw   []string
}

func (h *helper) Backups() (backups []Backup, problem string) { return h.backups, h.problem }

func (h *helper) Locations() []string {
	if h.nowhere {
		return nil
	}
	return []string{"/Users/someone/Library/Application Support/MobileSync/Backup"}
}

func (h *helper) record(what string, values ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.asked = append(h.asked, what)
	h.saw = append(h.saw, values...)
}

// requests is what the importer was asked to do, in order.
func (h *helper) requests() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.asked...)
}

// arguments is every value the importer was handed, which is where a test looks
// for something that should never have travelled.
func (h *helper) arguments() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.saw...)
}

// work is one long operation: it says what it is doing, and then waits to be let
// go, so a test can look at the server while it is still running.
func (h *helper) work(say Progress, step Step) (string, error) {
	for _, said := range h.says {
		say(step, said)
	}
	if h.gate != nil {
		<-h.gate
	}
	if h.failWork != nil {
		return "", h.failWork
	}
	return h.produces, nil
}

func (h *helper) Extract(_ context.Context, backup, into string, say Progress) (string, error) {
	h.record("extract", backup, into)
	return h.work(say, StepExtracting)
}

func (h *helper) Decrypt(_ context.Context, file, key, into string, say Progress) (string, error) {
	h.record("decrypt", file, key, into)
	return h.work(say, StepDecrypting)
}

func (h *helper) Open(_ context.Context, path, contacts string, _ Progress) (Archive, error) {
	h.record("open", path, contacts)
	if h.failOpen != nil {
		return nil, h.failOpen
	}
	if h.opens != nil {
		return h.opens(), nil
	}
	return fixture(), nil
}

// wizard returns a server with nothing open and an importer behind it.
func wizard(t *testing.T, bring Importer) *Server {
	t.Helper()

	handler, err := New(context.Background(), nil, Options{
		Location: time.UTC, Workspace: "/tmp/amberkeep-test",
		Importer: bring,
		Advise: func(err error) string {
			return "source.unrecognised-archive"
		},
	})
	if err != nil {
		t.Fatalf("New() with no archive failed: %v", err)
	}
	t.Cleanup(func() { _ = handler.Close() })
	return handler
}

// post sends a wizard request and returns the status and the body.
func post(t *testing.T, handler http.Handler, path string, body any) (status int, answer map[string]any) {
	t.Helper()

	var payload string
	switch v := body.(type) {
	case string:
		payload = v
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("building the request: %v", err)
		}
		payload = string(raw)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload)))

	var decoded map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	return recorder.Code, decoded
}

// state is what the page polls.
func state(t *testing.T, handler http.Handler) map[string]any {
	t.Helper()
	return ask(t, handler, "/api/state")
}

// settles waits for the server to reach a stage, because the work runs in its own
// goroutine and a test that read the state once would be reading a race.
func settles(t *testing.T, handler http.Handler, want string) map[string]any {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		body := state(t, handler)
		if body["stage"] == want {
			return body
		}
		if time.Now().After(deadline) {
			t.Fatalf("the server stayed at %v; want %s (%v)", body["stage"], want, body["detail"])
		}
		time.Sleep(time.Millisecond)
	}
}

func TestNothingIsOpenYet(t *testing.T) {
	t.Parallel()

	handler := wizard(t, &helper{})

	t.Run("the state says so, and where things will be written", func(t *testing.T) {
		body := state(t, handler)
		if body["stage"] != string(StageEmpty) {
			t.Errorf("stage = %v, want %s", body["stage"], StageEmpty)
		}
		if body["workspace"] != "/tmp/amberkeep-test" {
			t.Errorf("workspace = %v", body["workspace"])
		}
		if _, told := body["archive"]; told {
			t.Error("the state described an archive when none is open")
		}
	})

	// Everything that reads an archive has to say plainly that there is not one
	// yet, rather than failing in a way a page has to guess at.
	waiting := []string{
		"/api/archive",
		"/api/chats",
		"/api/chats/34600111222@s.whatsapp.net/messages",
		"/api/search?q=anything",
	}
	for _, path := range waiting {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, http.NoBody))
			if recorder.Code != http.StatusConflict {
				t.Errorf("GET %s returned %d, want %d", path, recorder.Code, http.StatusConflict)
			}
		})
	}
}

func TestListingTheBackupsOnThisComputer(t *testing.T) {
	t.Parallel()

	t.Run("what was found", func(t *testing.T) {
		t.Parallel()
		handler := wizard(t, &helper{backups: []Backup{{
			Path: "/backups/abc", DeviceName: "Ana's iPhone", ProductType: "iPhone14,2",
			IOSVersion: "26.0", LastBackup: "2026-09-12T20:00:00Z",
		}}})

		body := ask(t, handler, "/api/backups")
		found, ok := body["backups"].([]any)
		if !ok || len(found) != 1 {
			t.Fatalf("backups = %v", body["backups"])
		}
		one, _ := found[0].(map[string]any)
		if one["device_name"] != "Ana's iPhone" || one["path"] != "/backups/abc" {
			t.Errorf("the backup came back as %v", one)
		}
		if _, complained := body["problem"]; complained {
			t.Errorf("a successful listing reported a problem: %v", body["problem"])
		}
	})

	// An empty list and a folder this program has not been allowed to look in are
	// completely different situations, and only one of them is worth acting on.
	t.Run("a folder that could not be read is said out loud", func(t *testing.T) {
		t.Parallel()
		handler := wizard(t, &helper{problem: "Full Disk Access has not been granted"})

		body := ask(t, handler, "/api/backups")
		if body["problem"] != "Full Disk Access has not been granted" {
			t.Errorf("problem = %v", body["problem"])
		}
		if found, ok := body["backups"].([]any); !ok || len(found) != 0 {
			t.Errorf("backups = %v, want an empty list rather than null", body["backups"])
		}
	})

	t.Run("a server with no importer says so rather than pretending", func(t *testing.T) {
		t.Parallel()
		handler := serve(t, fixture())

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/backups", http.NoBody))
		if recorder.Code != http.StatusNotImplemented {
			t.Errorf("GET /api/backups returned %d, want %d", recorder.Code, http.StatusNotImplemented)
		}
	})
}

func TestGettingFromABackupToAnArchive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		request map[string]string
		wants   []string
	}{
		{
			name:    "an iPhone backup",
			path:    "/api/extract",
			request: map[string]string{"backup": "/backups/abc", "into": "/tmp/work"},
			wants:   []string{"extract", "open"},
		},
		{
			name: "an encrypted Android backup",
			path: "/api/decrypt",
			request: map[string]string{
				"file": "/downloads/msgstore.db.crypt15",
				"key":  strings.Repeat("a1", 32),
				"into": "/tmp/work",
			},
			wants: []string{"decrypt", "open"},
		},
		{
			// Nothing has to be taken out of anything here, so reading it is the
			// whole of the work.
			name:    "a database that is already readable",
			path:    "/api/open",
			request: map[string]string{"path": "/tmp/msgstore.db"},
			wants:   []string{"open"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bring := &helper{produces: "/tmp/work/msgstore.db"}
			handler := wizard(t, bring)

			status, body := post(t, handler, tt.path, tt.request)
			if status != http.StatusAccepted {
				t.Fatalf("POST %s returned %d: %v", tt.path, status, body)
			}
			if body["stage"] == nil {
				t.Error("the answer did not carry the state the page is about to poll")
			}

			ready := settles(t, handler, string(StageReady))
			archive, ok := ready["archive"].(map[string]any)
			if !ok {
				t.Fatalf("nothing described the archive that was opened: %v", ready)
			}
			if archive["conversations"] != float64(2) {
				t.Errorf("the archive came back as %v", archive)
			}

			// The work was asked for, and reading what it produced followed it.
			if got := bring.requests(); !slices.Equal(got, tt.wants) {
				t.Errorf("the importer was asked for %v, want %v", got, tt.wants)
			}

			// And the archive is now readable through the ordinary endpoints.
			if chats := ask(t, handler, "/api/chats"); chats["total"] != float64(2) {
				t.Errorf("the archive did not become readable: %v", chats)
			}
		})
	}
}

// TestTheKeyIsNeverRepeatedBack is the privacy property.
//
// The 64-digit key is the whole of somebody's backup. It reaches this program once,
// in one request, and must not come back out: not in the state the page polls, not
// in an error, and not in the address of anything.
func TestTheKeyIsNeverRepeatedBack(t *testing.T) {
	t.Parallel()

	const key = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	bring := &helper{failWork: errors.New("the backup could not be decrypted with this key")}
	handler := wizard(t, bring)

	status, accepted := post(t, handler, "/api/decrypt", map[string]string{
		"file": "/downloads/msgstore.db.crypt15", "key": key, "into": "/tmp/work",
	})
	if status != http.StatusAccepted {
		t.Fatalf("POST /api/decrypt returned %d: %v", status, accepted)
	}

	failed := settles(t, handler, string(StageFailed))
	for _, body := range []map[string]any{accepted, failed} {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("re-reading the answer: %v", err)
		}
		if strings.Contains(string(raw), key) {
			t.Fatalf("the key came back in %s", raw)
		}
	}

	// It did reach the importer, which is the only thing that needs it.
	if !slicesContain(bring.arguments(), key) {
		t.Error("the key never reached the thing that was supposed to use it")
	}
}

func TestAFailureSaysWhatToDoAboutIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		request map[string]string
		give    *helper
	}{
		{
			name:    "the work itself failed",
			path:    "/api/extract",
			request: map[string]string{"backup": "/backups/abc"},
			give:    &helper{failWork: errors.New("no such backup")},
		},
		{
			name:    "what it produced could not be read",
			path:    "/api/open",
			request: map[string]string{"path": "/tmp/x"},
			give:    &helper{failOpen: errors.New("not a database")},
		},
		{
			name:    "the archive could not be listed",
			path:    "/api/open",
			request: map[string]string{"path": "/tmp/x"},
			give: &helper{opens: func() Archive {
				broken := fixture()
				broken.failWith = errors.New("this file is damaged")
				return broken
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := wizard(t, tt.give)
			if status, body := post(t, handler, tt.path, tt.request); status != http.StatusAccepted {
				t.Fatalf("POST %s returned %d: %v", tt.path, status, body)
			}

			failed := settles(t, handler, string(StageFailed))
			detail, _ := failed["detail"].(string)
			if detail == "" {
				t.Fatal("a failure said nothing about what went wrong")
			}
			// Nobody should be left at a dead end with a sentence they cannot act on.
			if guidance, _ := failed["guidance"].(string); guidance != "source.unrecognised-archive" {
				t.Errorf("guidance = %q", guidance)
			}
		})
	}
}

// TestAFailureLetsGoOfWhatItOpened covers the case that leaks a file handle: the
// archive opened, listing it failed, and nothing above will ever see it again.
func TestAFailureLetsGoOfWhatItOpened(t *testing.T) {
	t.Parallel()

	broken := fixture()
	broken.failWith = errors.New("this file is damaged")

	handler := wizard(t, &helper{opens: func() Archive { return broken }})
	if status, body := post(t, handler, "/api/open", map[string]string{"path": "/tmp/x"}); status != http.StatusAccepted {
		t.Fatalf("POST /api/open returned %d: %v", status, body)
	}
	settles(t, handler, string(StageFailed))

	if !broken.closed.Load() {
		t.Error("an archive that could not be listed was left open")
	}
}

func TestOnlyOneThingAtATime(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	bring := &helper{gate: gate, produces: "/tmp/work/msgstore.db"}
	handler := wizard(t, bring)

	if status, body := post(t, handler, "/api/decrypt", map[string]string{
		"file": "/a.crypt15", "key": "k", "into": "/tmp/work",
	}); status != http.StatusAccepted {
		t.Fatalf("the first request returned %d: %v", status, body)
	}

	// Two decryptions writing to the same file would ruin both, so the second is
	// turned away rather than queued.
	working := settles(t, handler, string(StageWorking))
	if working["step"] != string(StepDecrypting) {
		t.Errorf("step = %v, want %s", working["step"], StepDecrypting)
	}

	status, _ := post(t, handler, "/api/decrypt", map[string]string{
		"file": "/b.crypt15", "key": "k", "into": "/tmp/work",
	})
	if status != http.StatusConflict {
		t.Errorf("a second request returned %d, want %d", status, http.StatusConflict)
	}

	close(gate)
	settles(t, handler, string(StageReady))
	if got := bring.requests(); len(got) != 2 {
		t.Errorf("the importer was asked for %v; the refused request should not have run", got)
	}
}

func TestProgressIsReportedWhileItRuns(t *testing.T) {
	t.Parallel()

	said := Noted("indexedSoFar", "Indexed {conversations} conversations so far.",
		"conversations", "400").Counting("conversations", 400)

	gate := make(chan struct{})
	bring := &helper{gate: gate, produces: "/tmp/msgstore.db", says: []Note{said}}
	handler := wizard(t, bring)

	if status, _ := post(t, handler, "/api/extract", map[string]string{"backup": "/backups/abc"}); status != http.StatusAccepted {
		t.Fatal("the request was not accepted")
	}

	// The sentence the importer said has to reach the state the page polls, because
	// a person watching a bar that does not move assumes the program has hung. The
	// work is still held open here, so this is what somebody would be looking at.
	deadline := time.Now().Add(5 * time.Second)
	var body map[string]any
	for {
		body = state(t, handler)
		if body["detail"] == said.Text {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("nothing said what was happening: %v", body)
		}
		time.Sleep(time.Millisecond)
	}

	// And the sentence's name goes with it, along with what filled it and the number
	// behind it. Without those the page can only show the English, which is what it
	// used to do to a reader who had been reading Spanish up to that point.
	if body["note"] != said.Name {
		t.Errorf("the state named the sentence %v, want %q", body["note"], said.Name)
	}
	values, ok := body["values"].(map[string]any)
	if !ok || values["conversations"] != "400" {
		t.Errorf("what filled the sentence did not arrive: %v", body["values"])
	}
	counts, ok := body["counts"].([]any)
	if !ok || len(counts) != 1 {
		t.Fatalf("the numbers did not arrive: %v", body["counts"])
	}
	if first, ok := counts[0].(map[string]any); !ok || first["of"] != "conversations" || first["n"] != 400.0 {
		t.Errorf("the number arrived as %v", counts[0])
	}

	close(gate)
	settles(t, handler, string(StageReady))
}

func TestTryingASecondBackupLetsGoOfTheFirst(t *testing.T) {
	t.Parallel()

	first, second := fixture(), fixture()
	bring := &helper{produces: "/tmp/msgstore.db"}
	bring.opens = func() Archive {
		if bring.handing.Add(1) == 1 {
			return first
		}
		return second
	}
	handler := wizard(t, bring)

	for _, path := range []string{"/tmp/one.db", "/tmp/two.db"} {
		if status, body := post(t, handler, "/api/open", map[string]string{"path": path}); status != http.StatusAccepted {
			t.Fatalf("opening %s returned %d: %v", path, status, body)
		}
		settles(t, handler, string(StageReady))
	}

	if !first.closed.Load() {
		t.Error("opening a second archive left the first one open")
	}
	if second.closed.Load() {
		t.Error("the archive that is being read was closed")
	}
}

func TestClosingGoesBackToTheBeginning(t *testing.T) {
	t.Parallel()

	open := fixture()
	handler := wizard(t, &helper{produces: "/tmp/msgstore.db", opens: func() Archive { return open }})

	if status, _ := post(t, handler, "/api/open", map[string]string{"path": "/tmp/one.db"}); status != http.StatusAccepted {
		t.Fatal("the request was not accepted")
	}
	settles(t, handler, string(StageReady))

	status, body := post(t, handler, "/api/close", map[string]string{})
	if status != http.StatusOK {
		t.Fatalf("POST /api/close returned %d: %v", status, body)
	}
	if body["stage"] != string(StageEmpty) {
		t.Errorf("stage = %v, want %s", body["stage"], StageEmpty)
	}
	if !open.closed.Load() {
		t.Error("closing left the archive open")
	}
}

func TestARequestThatCannotBeActedOn(t *testing.T) {
	t.Parallel()

	handler := wizard(t, &helper{})

	tests := []struct {
		name string
		path string
		body any
		want int
	}{
		{name: "opening nothing in particular", path: "/api/open", body: map[string]string{}, want: http.StatusBadRequest},
		{name: "extracting from nowhere", path: "/api/extract", body: map[string]string{}, want: http.StatusBadRequest},
		{
			name: "decrypting without a key",
			path: "/api/decrypt",
			body: map[string]string{"file": "/a.crypt15"},
			want: http.StatusBadRequest,
		},
		{
			name: "decrypting nothing",
			path: "/api/decrypt",
			body: map[string]string{"key": strings.Repeat("a1", 32)},
			want: http.StatusBadRequest,
		},
		{name: "something that is not a request at all", path: "/api/open", body: "{not json", want: http.StatusBadRequest},
		{
			name: "something enormous under a request's name",
			path: "/api/open",
			body: fmt.Sprintf(`{"path": %q}`, strings.Repeat("x", 128<<10)),
			want: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if status, _ := post(t, handler, tt.path, tt.body); status != tt.want {
				t.Errorf("POST %s returned %d, want %d", tt.path, status, tt.want)
			}
		})
	}
}

// TestWhereThingsAreWrittenIsRemembered covers the one setting somebody chooses
// before anything is written, and has to be able to find again afterwards.
func TestWhereThingsAreWrittenIsRemembered(t *testing.T) {
	t.Parallel()

	bring := &helper{produces: "/elsewhere/msgstore.db"}
	handler := wizard(t, bring)

	if status, _ := post(t, handler, "/api/extract", map[string]string{
		"backup": "/backups/abc", "into": "/elsewhere",
	}); status != http.StatusAccepted {
		t.Fatal("the request was not accepted")
	}
	settles(t, handler, string(StageReady))

	if got := state(t, handler)["workspace"]; got != "/elsewhere" {
		t.Errorf("workspace = %v, want /elsewhere", got)
	}
	if !slicesContain(bring.arguments(), "/elsewhere") {
		t.Errorf("the importer was told to write to %v", bring.arguments())
	}
}

// TestTheWorkspaceHasAHomeWorthFinding covers the default: somebody who has
// forgotten everything about this should still be able to find their archive.
func TestTheWorkspaceHasAHomeWorthFinding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		home string
		want string
	}{
		{name: "inside the home directory", home: "/Users/ana", want: "/Users/ana/Amberkeep"},
		{name: "when there is no home to speak of", home: "", want: "amberkeep"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Written with slashes because that is how a path reads, and converted
			// because that is not how Windows spells one.
			home, want := filepath.FromSlash(tt.home), filepath.FromSlash(tt.want)
			if got := defaultWorkspace(home); got != want {
				t.Errorf("defaultWorkspace(%q) = %q, want %q", home, got, want)
			}
		})
	}
}

func slicesContain(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// TestTheBackupsReplySaysWhereItLooked covers the difference between having made no
// backup and being on a computer that cannot make one.
//
// Both are an empty list. Apple ships no Finder, iTunes or Apple Devices for Linux,
// so a page told only that the list was empty goes on to explain how to make a
// backup in Finder, on a machine that has never had one.
func TestTheBackupsReplySaysWhereItLooked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		stub  *helper
		empty bool
	}{
		{
			name:  "a computer Apple's software runs on says where it looked",
			stub:  &helper{},
			empty: false,
		},
		{
			name:  "one it does not says it looked nowhere",
			stub:  &helper{nowhere: true},
			empty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := wizard(t, tt.stub)
			body := ask(t, handler, "/api/backups")

			looked, ok := body["looked"].([]any)
			if !ok {
				t.Fatalf("the reply did not say where it looked: %v", body["looked"])
			}
			if got := len(looked) == 0; got != tt.empty {
				t.Errorf("looked has %d entries; want empty = %v", len(looked), tt.empty)
			}
		})
	}
}
