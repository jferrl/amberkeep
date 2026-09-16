package api

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// filed is an archive with files beside it, for testing what the endpoint does with
// what a reader hands back.
type filed struct {
	Archive
	root string
}

func (f filed) OpenMedia(ref string) (io.ReadSeekCloser, string, error) {
	// The real readers refuse anything that climbs out; this one only has to be
	// able to hand back what it was given.
	if strings.Contains(ref, "..") {
		return nil, "", errors.New("no")
	}
	file, err := os.Open(filepath.Join(f.root, filepath.FromSlash(ref)))
	if err != nil {
		return nil, "", err
	}
	switch filepath.Ext(ref) {
	case ".jpg":
		return file, "image/jpeg", nil
	case ".svg":
		return file, "image/svg+xml", nil
	case ".html":
		return file, "text/html", nil
	case ".mp4":
		return file, "video/mp4", nil
	default:
		return file, "application/octet-stream", nil
	}
}

// withFiles returns a server holding an archive that has the named files with it.
func withFiles(t *testing.T, files map[string]string) http.Handler {
	t.Helper()

	root := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return serve(t, filed{Archive: fixture(), root: root})
}

// TestAPictureIsServedAsAPicture covers the ordinary case: it has to arrive as an
// image or it cannot be shown as one.
func TestAPictureIsServedAsAPicture(t *testing.T) {
	t.Parallel()

	handler := withFiles(t, map[string]string{"holiday.jpg": "not a real picture"})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/media?ref=holiday.jpg", http.NoBody))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("content type = %q", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("the browser is allowed to guess: %q", got)
	}
	if recorder.Body.String() != "not a real picture" {
		t.Errorf("body = %q", recorder.Body.String())
	}
}

// TestNothingElseIsServedAsItself is the one that matters.
//
// These bytes came off somebody's phone and are served from the address the launch
// secret is scoped to. A file served as text/html or as an SVG is script running with
// everything this page can reach, and the archive is full of things other people
// sent.
func TestNothingElseIsServedAsItself(t *testing.T) {
	t.Parallel()

	handler := withFiles(t, map[string]string{
		"page.html":   "<script>alert(1)</script>",
		"drawing.svg": `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		"notes.txt":   "words",
	})

	for _, name := range []string{"page.html", "drawing.svg", "notes.txt"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder,
				httptest.NewRequest(http.MethodGet, "/api/media?ref="+name, http.NoBody))

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d", recorder.Code)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/octet-stream" {
				t.Errorf("content type = %q, want it served as bytes to save", got)
			}
			if got := recorder.Header().Get("Content-Disposition"); got != "attachment" {
				t.Errorf("disposition = %q, want an attachment", got)
			}
		})
	}
}

// TestAVideoCanBeSeeked covers what a person dragging the middle of a recording
// actually sends, and what would otherwise mean sending the whole file to play ten
// seconds of it.
func TestAVideoCanBeSeeked(t *testing.T) {
	t.Parallel()

	handler := withFiles(t, map[string]string{"clip.mp4": "0123456789"})

	request := httptest.NewRequest(http.MethodGet, "/api/media?ref=clip.mp4", http.NoBody)
	request.Header.Set("Range", "bytes=4-6")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want a partial answer", recorder.Code)
	}
	if recorder.Body.String() != "456" {
		t.Errorf("body = %q", recorder.Body.String())
	}
}

// TestAskingForSomethingThatIsNotThere covers the two kinds of no, which are
// deliberately the same answer: a request that tried to climb out of the folder
// learns nothing from the reply.
func TestAskingForSomethingThatIsNotThere(t *testing.T) {
	t.Parallel()

	handler := withFiles(t, map[string]string{"holiday.jpg": "x"})

	for _, ref := range []string{"gone.jpg", "../../../etc/passwd"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/media?ref="+ref, http.NoBody))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("asking for %q = %d, want 404", ref, recorder.Code)
		}
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/media", http.NoBody))
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("asking for nothing = %d", recorder.Code)
	}
}

// TestAnArchiveWithNoFilesSaysSo covers every archive anybody has ever opened
// without bringing a folder, which is nearly all of them.
func TestAnArchiveWithNoFilesSaysSo(t *testing.T) {
	t.Parallel()

	handler := serve(t, fixture())

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/media?ref=anything.jpg", http.NoBody))
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d", recorder.Code)
	}
}
