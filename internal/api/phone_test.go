package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Reading a phone, from a page.
//
// What matters here is what the page is told when there is no phone, no adb, and no
// permission — because those are the ordinary cases, and each one has a different
// thing for somebody to go and do. An empty list for all three would be a screen
// that says "no" and leaves them nowhere.

// plugged is a Reader the tests drive.
type plugged struct {
	phones  []Phone
	backups []PhoneBackup
	file    string
	media   PhoneMedia

	failPhones, failBackups, failFetch, failFiles error

	// What was asked of the phone, and the lock over it. Copying outlives the
	// request that started it, so a test reads this while the work is still writing
	// to it; without the lock the race detector is right and the reader is wrong.
	mu    sync.Mutex
	asked []string
}

// record notes one thing that was asked of the phone.
func (p *plugged) record(what string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asked = append(p.asked, what)
}

// requests is what has been asked so far.
func (p *plugged) requests() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.asked...)
}

func (p *plugged) Phones(context.Context) ([]Phone, error) {
	p.record("phones")
	return p.phones, p.failPhones
}

func (p *plugged) Backups(_ context.Context, serial string) ([]PhoneBackup, error) {
	p.record("backups:" + serial)
	return p.backups, p.failBackups
}

func (p *plugged) Fetch(_ context.Context, serial, remote string, say Progress) (string, error) {
	p.record("fetch:" + serial + ":" + remote)
	if p.failFetch != nil {
		return "", p.failFetch
	}
	say(StepFetching, Quoting("1 file pulled."))
	return p.file, nil
}

func (p *plugged) Files(_ context.Context, serial string) (PhoneMedia, error) {
	p.record("files:" + serial)
	return p.media, p.failFiles
}

func (p *plugged) FetchFiles(_ context.Context, serial, into string, say Progress) (string, error) {
	p.record("fetch-files:" + serial)
	if p.failFiles != nil {
		return "", p.failFiles
	}
	say(StepFetching, Saying("copyingPhotographs", "Copying the photographs off the phone."))
	return into, nil
}

// reading returns a server that can see phones.
func reading(t *testing.T, p *plugged) http.Handler {
	t.Helper()

	handler, err := New(context.Background(), nil, Options{
		Location: time.UTC, Workspace: t.TempDir(),
		Importer: &helper{}, Phones: p,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() { _ = handler.Close() })
	return handler
}

func TestWhatThePageIsToldAboutPhones(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		p    *plugged
		// trouble is whether the reply should carry a reason rather than a bare
		// empty list.
		trouble bool
		phones  int
	}{
		{
			name:   "one that has been trusted",
			p:      &plugged{phones: []Phone{{Serial: "R5CT30", Name: "SM_A566B", Ready: true}}},
			phones: 1,
		},
		{
			// Visible and useless. The page has to be able to say "unlock it and tap
			// allow" rather than "no phone found".
			name:   "one that has not had the prompt accepted",
			p:      &plugged{phones: []Phone{{Serial: "R5CT30", Name: "R5CT30", Trouble: "unauthorized"}}},
			phones: 1,
		},
		{
			// An ordinary computer, not an error. The screen says so and offers the
			// way in that does not need a cable.
			name:    "a computer with no Android tools on it",
			p:       &plugged{failPhones: errors.New("adb is not installed on this computer")},
			trouble: true,
		},
		{
			name:   "nothing plugged in",
			p:      &plugged{},
			phones: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := reading(t, tt.p)
			body := ask(t, handler, "/api/phones")

			list, ok := body["phones"].([]any)
			if !ok {
				t.Fatalf("phones is not a list: %v", body["phones"])
			}
			if len(list) != tt.phones {
				t.Errorf("got %d phones, want %d", len(list), tt.phones)
			}

			said, _ := body["trouble"].(string)
			if tt.trouble && said == "" {
				t.Error("it returned an empty list without saying why there is nothing in it")
			}
			if !tt.trouble && said != "" {
				t.Errorf("it reported trouble when there was none: %q", said)
			}
		})
	}
}

// TestAFetchNeedsTheKey covers the thing that makes a fetched file worth having.
// The backup on the phone is encrypted, and somebody who asked for their messages
// did not ask for a file they cannot open.
func TestAFetchNeedsTheKey(t *testing.T) {
	t.Parallel()

	missing := []struct {
		name string
		body map[string]string
	}{
		{"no phone", map[string]string{"path": "/sdcard/x/msgstore.db.crypt15", "key": "k"}},
		{"no backup", map[string]string{"serial": "R5CT30", "key": "k"}},
		{"no key", map[string]string{"serial": "R5CT30", "path": "/sdcard/x/msgstore.db.crypt15"}},
	}
	for _, tt := range missing {
		t.Run(tt.name+" is refused", func(t *testing.T) {
			t.Parallel()

			handler := reading(t, &plugged{})
			if status, _ := post(t, handler, "/api/phones/fetch", tt.body); status != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
			}
		})
	}
}

// TestAServerThatCannotSeePhones covers a build without the Android tools: the
// endpoints say so rather than the page offering something that cannot work.
func TestAServerThatCannotSeePhones(t *testing.T) {
	t.Parallel()

	handler, err := New(context.Background(), nil, Options{
		Location: time.UTC, Workspace: t.TempDir(), Importer: &helper{},
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() { _ = handler.Close() })

	request := httptest.NewRequest(http.MethodGet, "/api/phones", http.NoBody)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusNotImplemented)
	}
}

// TestWhatThePhoneSaysAboutItsPhotographs: several gigabytes is not a thing to start
// without saying so, and a phone with no such folder is an ordinary phone rather
// than a failure.
func TestWhatThePhoneSaysAboutItsPhotographs(t *testing.T) {
	t.Parallel()

	t.Run("the size and the kinds, before anything is copied", func(t *testing.T) {
		t.Parallel()

		p := &plugged{media: PhoneMedia{
			Path:  "/sdcard/Android/media/com.whatsapp/WhatsApp/Media",
			Bytes: 5_700_000_000,
			Kinds: []string{"WhatsApp Images", "WhatsApp Voice Notes"},
		}}
		body := ask(t, reading(t, p), "/api/phones/R5CT30/media")

		if body["bytes"] != float64(5_700_000_000) {
			t.Errorf("it said the folder is %v", body["bytes"])
		}
		kinds, _ := body["kinds"].([]any)
		if len(kinds) != 2 {
			t.Errorf("it named %d kinds of file: %v", len(kinds), body["kinds"])
		}
		// Nothing was copied to answer the question.
		for _, one := range p.requests() {
			if strings.HasPrefix(one, "fetch") {
				t.Errorf("asking what is there copied something: %q", one)
			}
		}
	})

	t.Run("a phone with no such folder, which is not a failure", func(t *testing.T) {
		t.Parallel()

		body := ask(t, reading(t, &plugged{failFiles: errors.New("no WhatsApp media folder")}),
			"/api/phones/R5CT30/media")

		kinds, _ := body["kinds"].([]any)
		if len(kinds) != 0 {
			t.Errorf("it named kinds of file on a phone that has none: %v", body["kinds"])
		}
		// The sentence travels with the emptiness: "no photographs" and "this build
		// does not know where your phone keeps them" read very differently.
		if said, _ := body["path"].(string); said == "" {
			t.Error("it said nothing at all about why there is nothing")
		}
	})
}

// TestFetchingThePhotographsAlongWithTheMessages is the whole point of asking: the
// files land beside the database that was just decrypted, so the archive that opens
// next shows the photographs without anybody moving anything.
func TestFetchingThePhotographsAlongWithTheMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		media bool
		wants bool
	}{
		{name: "asked for", media: true, wants: true},
		{name: "not asked for", media: false, wants: false},
	}

	for _, tt := range tests {
		t.Run("the photographs are "+tt.name, func(t *testing.T) {
			t.Parallel()

			p := &plugged{file: filepath.Join(t.TempDir(), "msgstore.db.crypt15")}
			handler := reading(t, p)

			status, _ := post(t, handler, "/api/phones/fetch", map[string]any{
				"serial": "R5CT30",
				"path":   "/sdcard/Android/media/com.whatsapp/WhatsApp/Databases/msgstore.db.crypt15",
				"key":    strings.Repeat("a", 64),
				"media":  tt.media,
			})
			if status != http.StatusAccepted {
				t.Fatalf("status = %d, want %d", status, http.StatusAccepted)
			}
			waitFor(t, handler, func() bool {
				for _, one := range p.requests() {
					if one == "fetch-files:R5CT30" {
						return true
					}
				}
				return !tt.wants && done(t, handler)
			})

			var copied bool
			asked := p.requests()
			for _, one := range asked {
				if one == "fetch-files:R5CT30" {
					copied = true
				}
			}
			if copied != tt.wants {
				t.Errorf("the photographs were copied = %v, want %v (it did: %v)", copied, tt.wants, asked)
			}
		})
	}
}

// waitFor spins until something has happened or the test gives up, which is how a
// request that starts work in the background has to be checked.
func waitFor(t *testing.T, _ http.Handler, until func() bool) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for !until() {
		if time.Now().After(deadline) {
			t.Fatal("it never got there")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// done reports whether the wizard has stopped working, either way.
func done(t *testing.T, handler http.Handler) bool {
	t.Helper()

	stage, _ := ask(t, handler, "/api/state")["stage"].(string)
	return stage != "working"
}
