package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

	failPhones, failBackups, failFetch error

	asked []string
}

func (p *plugged) Phones(context.Context) ([]Phone, error) {
	p.asked = append(p.asked, "phones")
	return p.phones, p.failPhones
}

func (p *plugged) Backups(_ context.Context, serial string) ([]PhoneBackup, error) {
	p.asked = append(p.asked, "backups:"+serial)
	return p.backups, p.failBackups
}

func (p *plugged) Fetch(_ context.Context, serial, remote string, say Progress) (string, error) {
	p.asked = append(p.asked, "fetch:"+serial+":"+remote)
	if p.failFetch != nil {
		return "", p.failFetch
	}
	say(StepFetching, Quoting("1 file pulled."))
	return p.file, nil
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
