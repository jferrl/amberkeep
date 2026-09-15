package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/fixture"

	"github.com/jferrl/amberkeep/internal/api"
)

// The wizard, end to end: a person with a backup and no idea what is in it, and
// the page that gets them from one to the other.
//
// The server's own tests prove the state machine with an Importer that pretends.
// These prove the part that does not pretend: a real iPhone backup folder, a real
// message store, the real search index, over the real HTTP API.

// wizardOver returns a server with nothing open and the real Importer behind it.
//
// It lets go at the end of the test, which is not housekeeping: the archive it opens
// is a file in the test's own temporary directory, and on Windows a file that is
// still open cannot be deleted, so the cleanup fails and takes the test with it. The
// first CI run on Windows found that, on a machine nobody develops on.
func wizardOver(t *testing.T, workspace string) *api.Server {
	t.Helper()

	handler, err := api.New(t.Context(), nil, api.Options{
		Location:  time.UTC,
		Workspace: workspace,
		Importer:  Importer{Me: "You"},
		Advise:    AdviseOn,
	})
	if err != nil {
		t.Fatalf("starting the wizard: %v", err)
	}
	t.Cleanup(func() { _ = handler.Close() })
	return handler
}

// asks sends one wizard request and returns the status.
func asks(t *testing.T, handler http.Handler, path string, body map[string]string) int {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw))))
	return recorder.Code
}

// reads a GET and decodes it.
func reads(t *testing.T, handler http.Handler, path string) map[string]any {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, http.NoBody))

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET %s returned %d, which was not JSON: %s", path, recorder.Code, recorder.Body.String())
	}
	return body
}

// arrives waits for the import to finish, and says what went wrong when it did not.
func arrives(t *testing.T, handler http.Handler) map[string]any {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)
	for {
		body := reads(t, handler, "/api/state")
		switch body["stage"] {
		case "ready":
			return body
		case "failed":
			t.Fatalf("the import failed: %v\n%v", body["detail"], body["guidance"])
		}
		if time.Now().After(deadline) {
			t.Fatalf("the import never finished: %v", body)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTheWizardBringsAnIPhoneBackupIn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	backup := fixture.BackupWithAStore(t, dir)
	workspace := filepath.Join(dir, "workspace")

	handler := wizardOver(t, workspace)
	if status := asks(t, handler, "/api/extract", map[string]string{
		"backup": backup, "into": workspace,
	}); status != http.StatusAccepted {
		t.Fatalf("POST /api/extract returned %d", status)
	}

	ready := arrives(t, handler)
	archive, ok := ready["archive"].(map[string]any)
	if !ok {
		t.Fatalf("nothing described what had been brought in: %v", ready)
	}
	if archive["layout"] != "core-data" {
		t.Errorf("layout = %v, want the iPhone one", archive["layout"])
	}

	t.Run("the conversations are readable", func(t *testing.T) {
		chats := reads(t, handler, "/api/chats")
		if chats["total"] == float64(0) {
			t.Errorf("the archive came in with nothing in it: %v", chats)
		}
	})

	// The store holds only a path to a picture; without this step an iPhone archive
	// shows a decade of the words "image omitted".
	t.Run("the pictures came with it", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(fixture.ThumbInStore))); err != nil {
			t.Errorf("the picture did not land beside the store: %v", err)
		}
	})

	t.Run("and the store landed where somebody can find it", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(workspace, ChatStorage)); err != nil {
			t.Errorf("the message store is not in the workspace: %v", err)
		}
	})
}

func TestTheWizardOpensSomethingAlreadyReadable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	db := filepath.Join(dir, "msgstore.db")
	fixture.TinyArchive(t, db)

	book := filepath.Join(dir, "contacts.vcf")
	if err := os.WriteFile(book,
		[]byte("BEGIN:VCARD\nVERSION:3.0\nFN:Ana Lopez\nTEL:+34600111222\nEND:VCARD\n"), 0o600); err != nil {
		t.Fatalf("writing the address book: %v", err)
	}

	handler := wizardOver(t, dir)
	if status := asks(t, handler, "/api/open", map[string]string{
		"path": db, "contacts": book,
	}); status != http.StatusAccepted {
		t.Fatalf("POST /api/open returned %d", status)
	}
	arrives(t, handler)

	t.Run("the address book was applied", func(t *testing.T) {
		chats := reads(t, handler, "/api/chats")
		if !strings.Contains(strings.ToLower(mustJSON(t, chats)), "ana lopez") {
			t.Errorf("the conversation is still named after a phone number: %v", chats)
		}
	})

	// The wizard builds the index on the way in, so somebody who came through it
	// can search without being told to run a command.
	t.Run("and it can be searched", func(t *testing.T) {
		if summary := reads(t, handler, "/api/archive"); summary["searchable"] != true {
			t.Fatalf("the archive came in without a search index: %v", summary)
		}
		hits := reads(t, handler, "/api/search?q=hello")
		if hits["total"] == float64(0) {
			t.Errorf("searching found nothing: %v", hits)
		}
	})

	t.Run("and closing it goes back to the beginning", func(t *testing.T) {
		if status := asks(t, handler, "/api/close", map[string]string{}); status != http.StatusOK {
			t.Fatalf("POST /api/close returned %d", status)
		}
		if state := reads(t, handler, "/api/state"); state["stage"] != "empty" {
			t.Errorf("stage = %v, want empty", state["stage"])
		}
	})
}

func TestTheWizardSaysWhatToDoWhenItCannotOpenSomething(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	notAnArchive := filepath.Join(dir, "holiday.jpg")
	if err := os.WriteFile(notAnArchive, fixture.OnePixelJPEG, 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	handler := wizardOver(t, dir)
	if status := asks(t, handler, "/api/open", map[string]string{"path": notAnArchive}); status != http.StatusAccepted {
		t.Fatalf("POST /api/open returned %d", status)
	}

	// Nobody should be left at a dead end with a sentence they cannot act on.
	failed := refuses(t, handler, "")
	if guidance, _ := failed["guidance"].(string); guidance == "" {
		t.Errorf("the failure said nothing about what to do next: %v", failed)
	}
}

func TestDecryptingThroughTheWizard(t *testing.T) {
	t.Parallel()

	const key = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	dir := t.TempDir()
	backup := filepath.Join(dir, "msgstore.db.crypt15")
	if err := os.WriteFile(backup, []byte("not really a backup"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	tests := []struct {
		name    string
		request map[string]string
		prepare func(t *testing.T, workspace string)
		says    string
	}{
		{
			name:    "a key that is not one",
			request: map[string]string{"file": backup, "key": "hunter2"},
			says:    "neither a readable file nor a valid key",
		},
		{
			name:    "a backup that is not there",
			request: map[string]string{"file": filepath.Join(dir, "absent.crypt15"), "key": key},
			says:    "reading the backup",
		},
		{
			// Whatever is already there might be the archive from last time, and
			// this is the one command that would otherwise write over it.
			name:    "somewhere that already holds one",
			request: map[string]string{"file": backup, "key": key},
			prepare: func(t *testing.T, workspace string) {
				t.Helper()
				if err := os.MkdirAll(workspace, 0o700); err != nil {
					t.Fatalf("preparing the workspace: %v", err)
				}
				if err := os.WriteFile(filepath.Join(workspace, "msgstore.db"), []byte("mine"), 0o600); err != nil {
					t.Fatalf("writing the fixture: %v", err)
				}
			},
			says: "already a file",
		},
		{
			name:    "a file that is not a backup at all",
			request: map[string]string{"file": backup, "key": key},
			says:    "backup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			workspace := filepath.Join(t.TempDir(), "workspace")
			if tt.prepare != nil {
				tt.prepare(t, workspace)
			}

			handler := wizardOver(t, workspace)
			request := map[string]string{"into": workspace}
			for k, v := range tt.request {
				request[k] = v
			}
			if status := asks(t, handler, "/api/decrypt", request); status != http.StatusAccepted {
				t.Fatalf("POST /api/decrypt returned %d", status)
			}

			// And the key is never repeated back, whatever went wrong.
			failed := refuses(t, handler, tt.says)
			if strings.Contains(mustJSON(t, failed), key) {
				t.Error("the key came back in the state the page polls")
			}
		})
	}

	t.Run("the file that was already there is untouched", func(t *testing.T) {
		t.Parallel()

		workspace := filepath.Join(t.TempDir(), "workspace")
		if err := os.MkdirAll(workspace, 0o700); err != nil {
			t.Fatalf("preparing the workspace: %v", err)
		}
		mine := filepath.Join(workspace, "msgstore.db")
		if err := os.WriteFile(mine, []byte("mine"), 0o600); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}

		handler := wizardOver(t, workspace)
		asks(t, handler, "/api/decrypt", map[string]string{"file": backup, "key": key, "into": workspace})
		refuses(t, handler, "already a file")

		after, err := os.ReadFile(mine)
		if err != nil || string(after) != "mine" {
			t.Errorf("a file that was already there came back as %q (%v)", after, err)
		}
	})
}

// TestAddressBookIsRecognisedByItsName covers the one thing the wizard asks about
// contacts. Somebody has one file; which kind it is can be read off the name, and
// asking them to classify it would be asking them to know something the program
// can work out.
func TestAddressBookIsRecognisedByItsName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		given        string
		book, whatsA string
	}{
		{name: "nothing at all", given: ""},
		{name: "an exported address book", given: "/tmp/contacts.vcf", book: "/tmp/contacts.vcf"},
		{name: "WhatsApp's own contacts", given: "/tmp/wa.db", whatsA: "/tmp/wa.db"},
		{name: "shouted", given: "/tmp/WA.DB", whatsA: "/tmp/WA.DB"},
		{name: "something with no extension", given: "/tmp/contacts", book: "/tmp/contacts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			book, whatsApp := addressBook(tt.given)
			if book != tt.book || whatsApp != tt.whatsA {
				t.Errorf("addressBook(%q) = (%q, %q), want (%q, %q)",
					tt.given, book, whatsApp, tt.book, tt.whatsA)
			}
		})
	}
}

// TestListingBackupsNeverReturnsNothingAtAll covers the shape the page relies on:
// a list it can iterate, whatever this computer turns out to have on it.
func TestListingBackupsNeverReturnsNothingAtAll(t *testing.T) {
	t.Parallel()

	found, problem := Importer{}.Backups()
	if found == nil {
		t.Error("the list came back as nothing rather than as an empty list")
	}
	for _, backup := range found {
		if backup.Path == "" {
			t.Errorf("a backup came back with nowhere to find it: %+v", backup)
		}
		if backup.LastBackup != "" {
			if _, err := time.Parse(time.RFC3339, backup.LastBackup); err != nil {
				t.Errorf("last_backup = %q, which the page cannot read as a date", backup.LastBackup)
			}
		}
	}
	// A problem is a sentence for somebody to act on, not a stack trace.
	if strings.Contains(problem, "goroutine") {
		t.Errorf("problem = %q", problem)
	}
}

// TestAnArchiveLetsGoOfItsIndex covers the handle that would otherwise be left
// behind every time somebody tried a second backup.
func TestAnArchiveLetsGoOfItsIndex(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	db := filepath.Join(dir, "msgstore.db")
	fixture.TinyArchive(t, db)

	opened, err := Importer{Me: "You"}.Open(context.Background(), db, "", func(api.Step, api.Note) {})
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	searchable, ok := opened.(api.Searchable)
	if !ok || searchable.Index() == nil {
		t.Fatal("the archive came back without the index that was built over it")
	}
	if err := opened.Close(); err != nil {
		t.Errorf("closing the archive: %v", err)
	}
	// Closing twice is what a second close would be, and it must not panic.
	_ = opened.Close()
}

// refuses waits for the import to fail, and checks the sentence it failed with.
func refuses(t *testing.T, handler http.Handler, says string) map[string]any {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for {
		body := reads(t, handler, "/api/state")
		if body["stage"] == "failed" {
			detail, _ := body["detail"].(string)
			switch {
			case detail == "":
				t.Fatalf("the failure said nothing about what went wrong: %v", body)
			case !strings.Contains(detail, says):
				t.Errorf("the failure said %q, want something about %q", detail, says)
			}
			return body
		}
		if time.Now().After(deadline) {
			t.Fatalf("it never failed: %v", body)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("re-reading the answer: %v", err)
	}
	return string(raw)
}
