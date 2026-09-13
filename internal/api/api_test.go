package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

var (
	ana   = model.ParseJID("34600111222@s.whatsapp.net")
	luis  = model.ParseJID("34600333444@s.whatsapp.net")
	group = model.ParseJID("120363001@g.us")
	start = time.Date(2019, 6, 14, 9, 0, 0, 0, time.UTC)
)

// stub is an archive held in memory, so the tests describe what is served rather
// than how a database stores it.
type stub struct {
	chats     []model.Chat
	messages  map[int64][]model.Message
	directory *model.Directory
	failWith  error
}

func (s *stub) Chats(context.Context) ([]model.Chat, error) {
	if s.failWith != nil {
		return nil, s.failWith
	}
	return s.chats, nil
}

func (s *stub) Directory() *model.Directory { return s.directory }
func (s *stub) Layout() string              { return "modern" }

// Page mirrors the reader: newest first by limit, handed back oldest first.
func (s *stub) Page(_ context.Context, chat model.Chat, before model.Cursor, limit int) ([]model.Message, model.Cursor, error) {
	if s.failWith != nil {
		return nil, model.Cursor{}, s.failWith
	}

	all := s.messages[chat.ID]
	end := len(all)
	if !before.IsZero() {
		for i, m := range all {
			if !m.SentAt.Before(before.SentAt) && m.ID >= before.ID {
				end = i
				break
			}
		}
	}
	from := max(0, end-limit)

	page := all[from:end]
	var next model.Cursor
	if from > 0 {
		next = page[0].At()
	}
	return page, next, nil
}

func fixture() *stub {
	directory := model.NewDirectory()
	directory.Add(model.Contact{JID: ana, Name: "Ana Lopez"})
	directory.Add(model.Contact{JID: luis, Name: "Luis"})

	said := func(id int64, minutes int, text string) model.Message {
		return model.Message{
			ID: id, ChatID: 1, Kind: model.KindText, Sender: ana, Text: text,
			SentAt: start.Add(time.Duration(minutes) * time.Minute),
		}
	}

	direct := model.Chat{
		ID: 1, JID: ana, Kind: model.ChatDirect, Name: "Ana Lopez",
		Messages: 5, LastAt: start.Add(4 * time.Minute),
	}
	chatGroup := model.Chat{
		ID: 2, JID: group, Kind: model.ChatGroup, Name: "Vermut",
		Messages: 1, LastAt: start.Add(time.Hour),
		Participants: []model.Participant{{JID: ana, Name: "Ana Lopez"}, {JID: luis, Name: "Luis"}},
	}
	empty := model.Chat{ID: 3, JID: luis, Kind: model.ChatDirect, Name: "Luis", Messages: 0}

	return &stub{
		chats:     []model.Chat{direct, chatGroup, empty},
		directory: directory,
		messages: map[int64][]model.Message{
			1: {
				said(1, 0, "one"), said(2, 1, "two"), said(3, 2, "three"),
				said(4, 3, "four"), said(5, 4, "five"),
			},
			2: {said(6, 60, "in the group")},
		},
	}
}

// serve returns a handler over the fixture, with no secret to carry.
func serve(t *testing.T, archive Archive) http.Handler {
	t.Helper()

	handler, err := New(context.Background(), archive, Options{
		Names: archive.Directory(), Location: time.UTC, Me: "You", Title: "Archive",
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	return handler
}

// ask makes a request and decodes the answer.
func ask(t *testing.T, handler http.Handler, path string) map[string]any {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, http.NoBody))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s returned %d: %s", path, recorder.Code, recorder.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET %s did not return JSON: %v", path, err)
	}
	return body
}

func TestArchiveSummary(t *testing.T) {
	t.Parallel()

	body := ask(t, serve(t, fixture()), "/api/archive")

	tests := []struct {
		key  string
		want any
	}{
		{key: "conversations", want: float64(2)}, // the empty one is not served
		{key: "messages", want: float64(6)},
		{key: "layout", want: "modern"},
		{key: "searchable", want: false},
		{key: "title", want: "Archive"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if body[tt.key] != tt.want {
				t.Errorf("%s = %v, want %v", tt.key, body[tt.key], tt.want)
			}
		})
	}
}

func TestChatList(t *testing.T) {
	t.Parallel()

	handler := serve(t, fixture())

	t.Run("most recently used first", func(t *testing.T) {
		body := ask(t, handler, "/api/chats")
		chats, ok := body["chats"].([]any)
		if !ok || len(chats) != 2 {
			t.Fatalf("the list holds %v, want two conversations", body["chats"])
		}
		first, _ := chats[0].(map[string]any)
		if first["name"] != "Vermut" {
			t.Errorf("the first conversation is %v, want the one used most recently", first["name"])
		}
	})

	t.Run("an empty conversation is not offered", func(t *testing.T) {
		body := ask(t, handler, "/api/chats")
		for _, entry := range body["chats"].([]any) {
			if entry.(map[string]any)["name"] == "Luis" {
				t.Error("a conversation with no messages was listed")
			}
		}
	})

	t.Run("filtered by name, whatever the case", func(t *testing.T) {
		body := ask(t, handler, "/api/chats?q=VERM")
		if got := len(body["chats"].([]any)); got != 1 {
			t.Errorf("the filter returned %d conversations, want 1", got)
		}
		if body["total"] != float64(1) {
			t.Errorf("total = %v, want 1", body["total"])
		}
	})

	t.Run("a group carries its members", func(t *testing.T) {
		body := ask(t, handler, "/api/chats?q=vermut")
		chat := body["chats"].([]any)[0].(map[string]any)
		if got := len(chat["participants"].([]any)); got != 2 {
			t.Errorf("the group lists %d members, want 2", got)
		}
	})
}

// TestMessagePaging is what a viewer depends on: a conversation opens at its end
// and grows upwards, and every message is reached exactly once on the way.
func TestMessagePaging(t *testing.T) {
	t.Parallel()

	handler := serve(t, fixture())
	path := "/api/chats/" + url.PathEscape(ana.String()) + "/messages"

	body := ask(t, handler, path+"?limit=2")
	messages := body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("the first page holds %d messages, want 2", len(messages))
	}
	if text := messages[1].(map[string]any)["text"]; text != "five" {
		t.Errorf("the first page ends with %v, want the most recent message", text)
	}

	cursor, ok := body["before"].(string)
	if !ok || cursor == "" {
		t.Fatal("the first page offers no way back to the one before it")
	}

	var seen []string
	for range 10 {
		page := ask(t, handler, path+"?limit=2&before="+url.QueryEscape(cursor))
		// The page goes in front of what is already there, as a whole: prepending
		// one message at a time would turn each page back to front.
		texts := make([]string, 0, 2)
		for _, entry := range page["messages"].([]any) {
			texts = append(texts, entry.(map[string]any)["text"].(string))
		}
		seen = append(texts, seen...)
		next, _ := page["before"].(string)
		if next == "" {
			break
		}
		cursor = next
	}

	if want := "one two three"; strings.Join(seen, " ") != want {
		t.Errorf("paging back read %q, want %q", strings.Join(seen, " "), want)
	}
}

func TestMessagesRefuseWhatIsNotThere(t *testing.T) {
	t.Parallel()

	handler := serve(t, fixture())

	tests := []struct {
		name string
		path string
		want int
	}{
		{
			name: "a conversation that does not exist",
			path: "/api/chats/" + url.PathEscape("34699999999@s.whatsapp.net") + "/messages",
			want: http.StatusNotFound,
		},
		{
			name: "a position that is not one",
			path: "/api/chats/" + url.PathEscape(ana.String()) + "/messages?before=yesterday",
			want: http.StatusBadRequest,
		},
		{
			name: "searching without an index",
			path: "/api/search?q=hola",
			want: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.path, http.NoBody))
			if recorder.Code != tt.want {
				t.Errorf("GET %s returned %d, want %d", tt.path, recorder.Code, tt.want)
			}
		})
	}
}

// TestTheArchiveIsNotOpenToEverything is the security property. A port on this
// machine is reachable by everything else on this machine, so the secret is what
// actually keeps somebody's messages private.
func TestTheArchiveIsNotOpenToEverything(t *testing.T) {
	t.Parallel()

	const token = "the-launch-secret"
	archive := fixture()
	handler, err := New(context.Background(), archive, Options{
		Names: archive.Directory(), Location: time.UTC, Token: token,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	tests := []struct {
		name    string
		request func() *http.Request
		want    int
	}{
		{
			name:    "no secret at all",
			request: func() *http.Request { return httptest.NewRequest(http.MethodGet, "/api/archive", http.NoBody) },
			want:    http.StatusForbidden,
		},
		{
			name: "the wrong secret",
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/archive?t=guess", http.NoBody)
			},
			want: http.StatusForbidden,
		},
		{
			name: "a secret that is nearly right",
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/archive?t="+token+"x", http.NoBody)
			},
			want: http.StatusForbidden,
		},
		{
			name: "the secret in the address",
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/archive?t="+token, http.NoBody)
			},
			want: http.StatusOK,
		},
		{
			name: "the secret in a cookie",
			request: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/api/archive", http.NoBody)
				r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
				return r
			},
			want: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, tt.request())
			if recorder.Code != tt.want {
				t.Errorf("the request returned %d, want %d", recorder.Code, tt.want)
			}
		})
	}

	t.Run("the secret is kept for the rest of the session", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/archive?t="+token, http.NoBody))

		var kept bool
		for _, cookie := range recorder.Result().Cookies() {
			if cookie.Name == cookieName && cookie.Value == token {
				kept = true
				if !cookie.HttpOnly {
					t.Error("the secret is readable by scripts in the page")
				}
				if cookie.SameSite != http.SameSiteStrictMode {
					t.Error("the secret would be sent on requests from other sites")
				}
			}
		}
		if !kept {
			t.Error("the secret was not kept, so every request would need it in the address")
		}
	})

	t.Run("opening the page puts the secret away", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?t="+token, http.NoBody))
		if recorder.Code != http.StatusSeeOther {
			t.Errorf("the page returned %d, want a redirect that drops the secret from the address",
				recorder.Code)
		}
	})
}

// TestThePageFetchesNothingFromAnywhereElse: a viewer for private messages must not
// be able to reach out, whatever a mistake in the page might ask for.
func TestThePageFetchesNothingFromAnywhereElse(t *testing.T) {
	t.Parallel()

	handler := serve(t, fixture())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", http.NoBody))

	if recorder.Code != http.StatusOK {
		t.Fatalf("the page returned %d", recorder.Code)
	}
	policy := recorder.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'none'", "connect-src 'self'", "script-src 'self'"} {
		if !strings.Contains(policy, want) {
			t.Errorf("the policy %q is missing %q", policy, want)
		}
	}

	page := recorder.Body.String()
	for _, forbidden := range []string{"src=\"http", "href=\"http", "cdn."} {
		if strings.Contains(page, forbidden) {
			t.Errorf("the page refers to %q, which is not on this machine", forbidden)
		}
	}

	t.Run("nothing is cached to disk by the browser", func(t *testing.T) {
		if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", got)
		}
	})
}

func TestCursorsSurviveTheirOwnFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cursor model.Cursor
	}{
		{name: "an ordinary position", cursor: model.Cursor{SentAt: start, ID: 42}},
		{name: "before the epoch", cursor: model.Cursor{SentAt: start.AddDate(-60, 0, 0), ID: 1}},
		{name: "a large identifier", cursor: model.Cursor{SentAt: start, ID: 1 << 40}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseCursor(formatCursor(tt.cursor))
			if err != nil {
				t.Fatalf("parseCursor(formatCursor(%v)) failed: %v", tt.cursor, err)
			}
			if !got.SentAt.Equal(tt.cursor.SentAt) || got.ID != tt.cursor.ID {
				t.Errorf("the position came back as %v, want %v", got, tt.cursor)
			}
		})
	}

	t.Run("nothing means the end of the conversation", func(t *testing.T) {
		t.Parallel()
		got, err := parseCursor("")
		if err != nil || !got.IsZero() {
			t.Errorf("parseCursor(\"\") = %v, %v; want the zero position", got, err)
		}
	})

	t.Run("rubbish is refused", func(t *testing.T) {
		t.Parallel()
		for _, bad := range []string{"yesterday", "abc-def", "12", "12-x"} {
			if _, err := parseCursor(bad); err == nil {
				t.Errorf("parseCursor(%q) was accepted", bad)
			}
		}
	})
}

func TestIntParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		query string
		want  int
	}{
		{query: "", want: 60},
		{query: "?limit=10", want: 10},
		{query: "?limit=0", want: 60},
		{query: "?limit=-5", want: 60},
		{query: "?limit=nonsense", want: 60},
		{query: "?limit=99999", want: 500},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/x"+tt.query, http.NoBody)
			if got := intParam(r, "limit", 60, 500); got != tt.want {
				t.Errorf("intParam(%q) = %d, want %d", tt.query, got, tt.want)
			}
		})
	}
}

// TestAnUnreadableArchiveIsReportedAtTheStart, rather than as a broken page later.
func TestAnUnreadableArchiveIsReportedAtTheStart(t *testing.T) {
	t.Parallel()

	broken := fixture()
	broken.failWith = fmt.Errorf("the disk gave up")

	if _, err := New(context.Background(), broken, Options{}); err == nil {
		t.Error("New() succeeded over an archive that cannot be read")
	}
}
