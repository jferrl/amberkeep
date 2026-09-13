// Package api serves an archive over HTTP, on this machine only.
//
// It exists for two reasons. A viewer needs to open a conversation at its end and
// scroll back, which no file on disk can do for ninety thousand messages, and it
// needs to search the whole archive at once. And the desktop application will speak
// to the engine through exactly this, so the shape here is the contract: the same
// JSON the export writes, from the same code, so a file and a response cannot
// disagree about what an archive contains.
//
// Nothing is reachable from another machine. The server binds to the loopback
// address, and every request carries a secret made fresh at each launch, so another
// program on the same computer cannot read somebody's messages by guessing a port.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "embed"

	"github.com/jferrl/amberkeep/internal/export"
	"github.com/jferrl/amberkeep/internal/model"
	"github.com/jferrl/amberkeep/internal/search"
)

//go:embed assets/app.js
var appJS string

//go:embed assets/app.html
var appHTML string

// Archive is the part of a reader this package needs.
//
// It is an interface so that serving an archive does not depend on which platform
// it came from: an iPhone reader satisfying this serves without a line changing
// here.
type Archive interface {
	Chats(ctx context.Context) ([]model.Chat, error)
	Page(ctx context.Context, chat model.Chat, before model.Cursor, limit int) ([]model.Message, model.Cursor, error)
	Directory() *model.Directory
	Layout() string
}

// Options say how an archive is presented.
type Options struct {
	// Names resolves addresses to people.
	Names *model.Directory
	// Location is the time zone timestamps are rendered in.
	Location *time.Location
	// Me is what to call the archive's owner.
	Me string
	// Title names the archive in the page.
	Title string

	// Token is the secret every request must carry. An empty token means no check,
	// which is only for tests: a server without one is readable by anything else
	// running on the machine.
	Token string

	// Index is the full-text index. Searching is unavailable without one, and says
	// so rather than returning nothing.
	Index *search.Index
}

func (o Options) withDefaults() Options {
	if o.Location == nil {
		o.Location = time.Local
	}
	if o.Me == "" {
		o.Me = "You"
	}
	if o.Names == nil {
		o.Names = model.NewDirectory()
	}
	if o.Title == "" {
		o.Title = "Archive"
	}
	return o
}

// server answers questions about one archive.
type server struct {
	archive Archive
	opts    Options

	// chats is the conversation list, held because it is small, needed on every
	// request, and expensive to rebuild.
	chats  []model.Chat
	byJID  map[string]model.Chat
	export export.Options
}

// New returns a handler serving the archive.
//
// The conversation list is read once here rather than per request, so a failure to
// read the archive is reported at startup instead of as a broken page later.
func New(ctx context.Context, archive Archive, opts Options) (http.Handler, error) {
	opts = opts.withDefaults()

	chats, err := archive.Chats(ctx)
	if err != nil {
		return nil, err
	}

	s := &server{
		archive: archive,
		opts:    opts,
		byJID:   make(map[string]model.Chat, len(chats)),
		export: export.Options{
			Names:    opts.Names,
			Location: opts.Location,
			Me:       opts.Me,
		},
	}
	for _, chat := range chats {
		if !chat.Includable() {
			continue
		}
		s.chats = append(s.chats, chat)
		s.byJID[chat.JID.String()] = chat
	}
	// Ordered once here rather than on every request. A real archive holds several
	// thousand conversations, and the order they are listed in cannot change while
	// the server is running.
	s.chats = sortedByRecency(s.chats)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/archive", s.handleArchive)
	mux.HandleFunc("GET /api/chats", s.handleChats)
	mux.HandleFunc("GET /api/chats/{jid}/messages", s.handleMessages)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /app.js", serveText("text/javascript; charset=utf-8", appJS))
	mux.HandleFunc("GET /app.css", serveText("text/css; charset=utf-8", export.Stylesheet()))
	mux.HandleFunc("GET /{$}", s.handlePage)

	return s.authenticated(mux), nil
}

// authenticated turns away anything that does not carry the launch secret.
//
// The secret arrives once in the address the browser was opened with and is kept in
// a cookie afterwards, so it stays out of the page's own links and out of whatever
// the browser records about where somebody has been.
func (s *server) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A page fetched by another site must never reach the archive, whatever a
		// browser decides to do with cookies.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		if s.opts.Token == "" {
			next.ServeHTTP(w, r)
			return
		}

		if given := r.URL.Query().Get("t"); matches(given, s.opts.Token) {
			// Not marked Secure: the viewer is plain HTTP on the loopback address,
			// which browsers treat as trustworthy but which a Secure cookie is not
			// reliably sent over. HttpOnly and SameSite=Strict are the ones that
			// matter here, and both are set.
			// #nosec G124 -- see above.
			http.SetCookie(w, &http.Cookie{
				Name: cookieName, Value: s.opts.Token, Path: "/",
				HttpOnly: true, SameSite: http.SameSiteStrictMode,
			})
			// Redirected so the secret does not stay in the address bar, or in
			// whatever the browser remembers of it.
			if r.URL.Path == "/" {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
		} else if cookie, err := r.Cookie(cookieName); err != nil || !matches(cookie.Value, s.opts.Token) {
			http.Error(w, "this archive is open to this browser session only", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// cookieName is where the launch secret is kept between requests.
const cookieName = "amberkeep"

// matches compares secrets in constant time, so a wrong guess tells nothing about
// how nearly right it was.
func matches(given, want string) bool {
	return subtle.ConstantTimeCompare([]byte(given), []byte(want)) == 1
}

// handleArchive reports what the archive holds.
func (s *server) handleArchive(w http.ResponseWriter, r *http.Request) {
	var (
		messages int
		earliest time.Time
		latest   time.Time
		byKind   = make(map[string]int)
	)
	for _, chat := range s.chats {
		messages += chat.Messages
		byKind[chat.Kind.String()]++
		if !chat.CreatedAt.IsZero() && (earliest.IsZero() || chat.CreatedAt.Before(earliest)) {
			earliest = chat.CreatedAt
		}
		if chat.LastAt.After(latest) {
			latest = chat.LastAt
		}
	}

	body := map[string]any{
		"title":         s.opts.Title,
		"layout":        s.archive.Layout(),
		"conversations": len(s.chats),
		"messages":      messages,
		"by_kind":       byKind,
		"people":        s.opts.Names.Len(),
		"named":         s.opts.Names.Identified(),
		"searchable":    s.opts.Index != nil,
		"time_zone":     s.opts.Location.String(),
	}
	if !earliest.IsZero() {
		body["earliest"] = earliest.UTC()
	}
	if !latest.IsZero() {
		body["latest"] = latest.UTC()
	}
	write(w, r, body)
}

// handleChats lists conversations, most recently used first.
func (s *server) handleChats(w http.ResponseWriter, r *http.Request) {
	var (
		q             = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		limit         = intParam(r, "limit", 200, 1000)
		offset        = intParam(r, "offset", 0, 1<<30)
		matched       = make([]model.Chat, 0, limit)
		total         int
		skipRemaining = offset
	)

	for _, chat := range s.chats {
		if q != "" && !strings.Contains(strings.ToLower(chat.Title()), q) {
			continue
		}
		total++
		if skipRemaining > 0 {
			skipRemaining--
			continue
		}
		if len(matched) < limit {
			matched = append(matched, chat)
		}
	}

	list := make([]any, 0, len(matched))
	for _, chat := range matched {
		list = append(list, export.ChatValue(chat, s.export))
	}
	write(w, r, map[string]any{"total": total, "chats": list})
}

// handleMessages returns a page of one conversation, ending at a position and
// reading backwards, which is the order somebody reads a conversation in.
func (s *server) handleMessages(w http.ResponseWriter, r *http.Request) {
	chat, ok := s.byJID[r.PathValue("jid")]
	if !ok {
		http.Error(w, "no such conversation", http.StatusNotFound)
		return
	}

	before, err := parseCursor(r.URL.Query().Get("before"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	page, next, err := s.archive.Page(r.Context(), chat, before, intParam(r, "limit", 60, 500))
	if err != nil {
		fail(w, r, err)
		return
	}

	messages := make([]any, 0, len(page))
	for _, m := range page {
		if !m.Displayable() {
			continue
		}
		messages = append(messages, export.MessageValue(m, s.export))
	}

	body := map[string]any{
		"chat":     export.ChatValue(chat, s.export),
		"messages": messages,
	}
	if !next.IsZero() {
		body["before"] = formatCursor(next)
	}
	write(w, r, body)
}

// handleSearch finds messages anywhere in the archive.
func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if s.opts.Index == nil {
		http.Error(w, "this archive was served without a search index", http.StatusServiceUnavailable)
		return
	}

	term := r.URL.Query().Get("q")
	query := search.Query{
		Limit:  intParam(r, "limit", 50, 500),
		Offset: intParam(r, "offset", 0, 1<<20),
		Chat:   r.URL.Query().Get("chat"),
		Before: search.MarkOpen, After: search.MarkClose,
	}

	hits, err := s.opts.Index.Search(r.Context(), term, query)
	if err != nil {
		if errors.Is(err, search.ErrEmptyQuery) {
			write(w, r, map[string]any{"total": 0, "hits": []any{}})
			return
		}
		fail(w, r, err)
		return
	}
	total, err := s.opts.Index.Count(r.Context(), term, query)
	if err != nil {
		fail(w, r, err)
		return
	}

	results := make([]any, 0, len(hits))
	for _, hit := range hits {
		results = append(results, map[string]any{
			"chat":     hit.Chat,
			"chat_jid": hit.ChatJID,
			"sender":   hit.Sender,
			"from_me":  hit.FromMe,
			"sent_at":  hit.SentAt,
			"kind":     hit.Kind,
			"snippet":  hit.Snippet,
		})
	}
	write(w, r, map[string]any{"total": total, "hits": results})
}

// handlePage serves the viewer itself.
func (s *server) handlePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The page loads nothing but itself. Said here as well as meant, so a mistake
	// in the page cannot quietly start fetching from somewhere else.
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'")
	send(w, appHTML)
}

// serveText serves one of the page's own files.
func serveText(mediaType, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", mediaType)
		send(w, body)
	}
}

// send writes a response whose status has already gone out.
//
// A failure here means the reader closed the page or the connection dropped. The
// status line is already sent, so there is no way to report it and nobody left to
// report it to.
func send(w http.ResponseWriter, body string) {
	_, _ = io.WriteString(w, body)
}

// sortedByRecency orders conversations the way somebody looks for one.
func sortedByRecency(chats []model.Chat) []model.Chat {
	out := make([]model.Chat, len(chats))
	copy(out, chats)
	// A stable sort, so two conversations with the same last message keep whatever
	// order the reader gave them rather than changing between requests.
	slices.SortStableFunc(out, func(a, b model.Chat) int { return b.LastAt.Compare(a.LastAt) })
	return out
}

// intParam reads a bounded number from the query, falling back rather than failing:
// a viewer asking for a silly page size should get a sensible one, not an error.
func intParam(r *http.Request, name string, fallback, ceiling int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return fallback
	}
	if n > ceiling {
		return ceiling
	}
	if n == 0 && fallback > 0 {
		return fallback
	}
	return n
}

// cursorSeparator divides the two halves of a position.
//
// Not a dash: a message dated before 1970 has a negative timestamp, and splitting
// that on a dash takes the sign off rather than the two halves apart. Corrupt dates
// are exactly the case a viewer should survive.
const cursorSeparator = "_"

// formatCursor renders a position so it can travel in a URL.
func formatCursor(c model.Cursor) string {
	return strconv.FormatInt(c.SentAt.UnixMilli(), 10) + cursorSeparator + strconv.FormatInt(c.ID, 10)
}

// parseCursor reads a position back. An empty one means the end of the conversation.
func parseCursor(s string) (model.Cursor, error) {
	if s == "" {
		return model.Cursor{}, nil
	}
	at, id, found := strings.Cut(s, cursorSeparator)
	if !found {
		return model.Cursor{}, fmt.Errorf("%q is not a position in a conversation", s)
	}
	millis, err := strconv.ParseInt(at, 10, 64)
	if err != nil {
		return model.Cursor{}, fmt.Errorf("%q is not a position in a conversation", s)
	}
	row, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return model.Cursor{}, fmt.Errorf("%q is not a position in a conversation", s)
	}
	return model.Cursor{SentAt: time.UnixMilli(millis).UTC(), ID: row}, nil
}

// write sends a JSON response.
func write(w http.ResponseWriter, r *http.Request, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(body); err != nil && r.Context().Err() == nil {
		// The status is already sent, so this can only be recorded, not reported.
		http.Error(w, "", http.StatusInternalServerError)
	}
}

// fail reports a problem reading the archive, without putting its internals into a
// response somebody might paste somewhere.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	if r.Context().Err() != nil {
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
