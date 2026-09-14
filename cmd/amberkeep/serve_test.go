package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/fixture"

	"github.com/jferrl/amberkeep/internal/app"
)

// TestServeOpensTheArchiveOnThisMachineOnly runs the viewer the way somebody does
// and then checks the two things that matter: it answers, and it answers only to
// whoever holds the secret it printed.
//
// It is not parallel, because it takes over the program's own output to find the
// address the server chose.
func TestServeOpensTheArchiveOnThisMachineOnly(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "msgstore.db")
	fixture.TinyArchive(t, db)

	viewer, stop := startViewer(t, db)
	defer stop()

	// A cookie jar, because that is what a browser has: the secret arrives once in
	// the address and is kept for the rest of the session.
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("making a cookie jar: %v", err)
	}
	client := &http.Client{Timeout: 5 * time.Second, Jar: jar}

	t.Run("the page opens with the secret in the address", func(t *testing.T) {
		body, status := fetch(t, client, viewer.opening())
		if status != http.StatusOK {
			t.Fatalf("the viewer returned %d", status)
		}
		if !strings.Contains(body, "<title>Amberkeep</title>") {
			t.Errorf("what came back is not the viewer:\n%.200s", body)
		}
	})

	t.Run("without the secret it says no", func(t *testing.T) {
		// A fresh client, with none of the first one's cookies: this is another
		// program on the same machine that has found the port.
		stranger := &http.Client{Timeout: 5 * time.Second}
		if _, status := fetch(t, stranger, viewer.plain("/api/archive")); status != http.StatusForbidden {
			t.Errorf("a request with no secret returned %d, want it refused", status)
		}
	})

	t.Run("with the secret it serves the archive", func(t *testing.T) {
		body, status := fetch(t, client, viewer.at("/api/archive"))
		if status != http.StatusOK {
			t.Fatalf("the archive endpoint returned %d", status)
		}
		for _, want := range []string{`"conversations":1`, `"messages":2`, `"searchable":false`} {
			if !strings.Contains(body, want) {
				t.Errorf("the summary is missing %s:\n%s", want, body)
			}
		}

		list, status := fetch(t, client, viewer.at("/api/chats"))
		if status != http.StatusOK {
			t.Fatalf("the conversation list returned %d", status)
		}
		if !strings.Contains(list, "34600111222") {
			t.Errorf("the conversation is missing from the list:\n%s", list)
		}
	})

	t.Run("it is not reachable from another machine", func(t *testing.T) {
		host := outboundAddress(t)
		if host == "" {
			t.Skip("this machine has no other address to try")
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, viewer.address.Port()), time.Second)
		if err == nil {
			_ = conn.Close()
			t.Errorf("the viewer accepted a connection on %s, so it is not loopback only", host)
		}
	})
}

// viewer is a running serve command, and the secret it printed.
type viewer struct {
	address *url.URL
	token   string
}

// opening is the address a browser is sent to, carrying the secret once.
func (v viewer) opening() string {
	return v.address.String() + "/?t=" + url.QueryEscape(v.token)
}

// at is one of the viewer's own addresses, with the secret on it. A browser would
// carry a cookie instead; a test client that has one ignores this.
func (v viewer) at(path string) string {
	return v.address.String() + path + "?t=" + url.QueryEscape(v.token)
}

// plain is the same address without the secret, as another program on this machine
// that had found the port would reach it.
func (v viewer) plain(path string) string {
	return v.address.String() + path
}

// startViewer runs the serve command and finds the address it printed.
func startViewer(t *testing.T, db string) (running viewer, stop func()) {
	t.Helper()

	// The command prints the address it chose, which is the only way to learn the
	// port the system gave it and the secret it made.
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("making a pipe: %v", err)
	}
	realStdout := os.Stdout
	os.Stdout = writer

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runServe(ctx, []string{"--db", db, "--no-open", "--no-search"})
	}()

	found := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(reader)
		pattern := regexp.MustCompile(`http://127\.0\.0\.1:\d+/\?t=\S+`)
		for scanner.Scan() {
			if address := pattern.FindString(scanner.Text()); address != "" {
				found <- address
				return
			}
		}
		found <- ""
	}()

	var address string
	select {
	case address = <-found:
	case err := <-done:
		os.Stdout = realStdout
		t.Fatalf("the viewer stopped before it started: %v", err)
	case <-time.After(20 * time.Second):
		os.Stdout = realStdout
		cancel()
		t.Fatal("the viewer never printed an address")
	}

	if address == "" {
		os.Stdout = realStdout
		cancel()
		t.Fatal("the viewer printed no address to open")
	}

	opened, err := url.Parse(address)
	if err != nil {
		os.Stdout = realStdout
		cancel()
		t.Fatalf("the viewer printed an address that is not one: %q", address)
	}
	token := opened.Query().Get("t")
	opened.RawQuery, opened.Path = "", ""

	return viewer{address: opened, token: token}, func() {
		cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("the viewer stopped with an error: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("the viewer did not stop when it was asked to")
		}
		os.Stdout = realStdout
		_ = writer.Close()
		_ = reader.Close()
	}
}

// fetch makes one request and returns what came back.
func fetch(t *testing.T, client *http.Client, address string) (body string, status int) {
	t.Helper()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, address, http.NoBody)
	if err != nil {
		t.Fatalf("building a request for %s: %v", address, err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("requesting %s: %v", address, err)
	}
	defer func() { _ = response.Body.Close() }()

	read, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading the response: %v", err)
	}
	return string(read), response.StatusCode
}

// outboundAddress is one of this machine's own addresses that is not the loopback,
// for checking that the viewer is not listening on it.
func outboundAddress(t *testing.T) string {
	t.Helper()

	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addresses {
		if ip, ok := address.(*net.IPNet); ok && !ip.IP.IsLoopback() && ip.IP.To4() != nil {
			return ip.IP.String()
		}
	}
	return ""
}

func TestSecretIsDifferentEveryTime(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool, 16)
	for range 16 {
		token, err := secret()
		if err != nil {
			t.Fatalf("secret() failed: %v", err)
		}
		if len(token) < 40 {
			t.Errorf("the secret is %d characters, too few to be worth having", len(token))
		}
		if seen[token] {
			t.Fatal("the same secret was made twice")
		}
		seen[token] = true
	}
}

func TestParseDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		in    string
		want  string
		fails bool
	}{
		{name: "the usual way", in: "2019-06-14", want: "2019-06-14"},
		{name: "the way it is written in Spain", in: "14/06/2019", want: "2019-06-14"},
		{name: "a whole year", in: "2019", want: "2019-01-01"},
		{name: "nothing means no bound", in: "", want: ""},
		{name: "a month name", in: "June 2019", fails: true},
		{name: "rubbish", in: "yesterday", fails: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseDate(tt.in, time.UTC)
			if tt.fails {
				if err == nil {
					t.Fatalf("parseDate(%q) was accepted as %v", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDate(%q) failed: %v", tt.in, err)
			}
			if tt.want == "" {
				if !got.IsZero() {
					t.Errorf("parseDate(%q) = %v, want no bound", tt.in, got)
				}
				return
			}
			if got.Format("2006-01-02") != tt.want {
				t.Errorf("parseDate(%q) = %v, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestIndexPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		db     string
		chosen string
		want   string
	}{
		{
			name: "beside the archive by default",
			db:   filepath.Join("a", "b", "msgstore.db"),
			want: filepath.Join("a", "b", "msgstore.db.amberkeep-index"),
		},
		{
			name:   "where the caller asked",
			db:     filepath.Join("a", "msgstore.db"),
			chosen: filepath.Join("c", "index.db"),
			want:   filepath.Join("c", "index.db"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := app.IndexPath(tt.db, tt.chosen); got != tt.want {
				t.Errorf("app.IndexPath(%q, %q) = %q, want %q", tt.db, tt.chosen, got, tt.want)
			}
		})
	}
}

func TestSmallHelpers(t *testing.T) {
	t.Parallel()

	t.Run("app.Plural", func(t *testing.T) {
		t.Parallel()
		if got := app.Plural(1, "match", "matches"); got != "1 match" {
			t.Errorf("app.Plural(1) = %q", got)
		}
		if got := app.Plural(0, "match", "matches"); got != "0 matches" {
			t.Errorf("app.Plural(0) = %q", got)
		}
	})

	t.Run("a snippet stays on one line", func(t *testing.T) {
		t.Parallel()
		if got := oneLine("two\nlines   here"); got != "two lines here" {
			t.Errorf("oneLine() = %q", got)
		}
	})

	t.Run("the first word of a search", func(t *testing.T) {
		t.Parallel()
		if got := firstWord("  hola   mundo "); got != "hola" {
			t.Errorf("firstWord() = %q", got)
		}
		if got := firstWord(""); got != "" {
			t.Errorf("firstWord(\"\") = %q", got)
		}
	})

	t.Run("marks are plain when the output is not a terminal", func(t *testing.T) {
		t.Parallel()
		// Under test the output is a file or a pipe, never a terminal, so escape
		// codes must not appear: they would end up in whatever somebody piped it to.
		got := searchMarks()
		if strings.Contains(got.before, "\033") || strings.Contains(got.after, "\033") {
			t.Errorf("searchMarks() = %q/%q, want no escape codes when nobody is watching",
				got.before, got.after)
		}
	})
}
