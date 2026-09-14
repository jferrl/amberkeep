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
	"io/fs"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jferrl/amberkeep/internal/export"
	"github.com/jferrl/amberkeep/internal/model"
	"github.com/jferrl/amberkeep/internal/search"
	"github.com/jferrl/amberkeep/internal/viewer"
)

// viewer is the built frontend, compiled into the binary by the web package.
//
// It is read once at startup rather than per request, and a binary built without it
// is a build mistake rather than a runtime condition, so that failure is reported
// when the server starts and not as a blank page later.
var builtViewer = sync.OnceValues(viewer.Assets)

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

	// Close releases the file. A session that opens a second archive closes the
	// first, so that somebody trying two backups in turn does not leave a database
	// open on each.
	Close() error
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

	// Importer does the work the wizard asks for. Without one the server serves an
	// archive it was given and nothing else, which is what the command line does.
	Importer Importer

	// Migrator moves a history onto a phone. Without one those endpoints answer 501
	// and the page leaves the whole thing off, which is what a build that only reads
	// archives should do.
	Migrator Migrator

	// Workspace is where anything this program writes will go. It is shown to
	// somebody before anything is written there, so they can find it afterwards.
	Workspace string

	// Advise turns a failure into the several lines of help that go with it. The
	// wording lives with the commands, which is where every other failure in this
	// program is explained.
	Advise func(error) string
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
	if o.Workspace == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = ""
		}
		o.Workspace = defaultWorkspace(home)
	}
	return o
}

// Searchable is an archive that came with a full-text index over it.
//
// It is an optional interface rather than part of Archive because an index is not
// part of reading one: an archive opened from the command line without `--search`
// is perfectly readable, and one the wizard opened built its index on the way in.
// Whoever produced the archive knows which, and says so by implementing this.
type Searchable interface {
	Index() *search.Index
}

// server answers questions about one archive.
type server struct {
	opts Options

	// assets is the built viewer.
	assets fs.FS

	// session is whatever archive is open, which may be none: the wizard has to be
	// reachable by somebody who does not have one yet.
	session *session

	// importer does the work the wizard asks for.
	importer Importer

	// migration is how far along a migration is, and migrator is what does it. Kept
	// apart from the archive session: bringing an archive in and moving one onto a
	// phone are different jobs, and neither should be able to put the other into a
	// state it did not ask for.
	migration *migration
	migrator  Migrator

	// background outlives the request that began a piece of work, so closing the tab
	// halfway through a decryption does not leave a half-written file.
	background context.Context
}

// Server answers requests about an archive, and lets go of it when asked.
//
// Letting go is the whole reason this is a type rather than a bare handler. A
// process that is about to exit does not need it, which is why nobody noticed for
// months; anything longer-lived does, and on Windows an open file cannot even be
// deleted. The first CI run there found it, in a test whose temporary directory
// could not be cleaned up.
type Server struct {
	handler http.Handler
	session *session
}

// ServeHTTP answers one request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// Close releases whatever archive is open, and the index built over it.
//
// It is safe to call more than once and safe to call while requests are in flight:
// closing waits for anything already reading, and everything afterwards is answered
// with "no archive is open yet", which is a state the page already knows.
func (s *Server) Close() error {
	s.session.close()
	return nil
}

// New returns a server.
//
// The archive may be nil. The server then serves the wizard until one is opened
// through it, which is the case that matters: somebody whose phone died does not
// have a readable archive, and getting from what they do have to one is the part
// they need help with.
//
// When an archive is given, the conversation list is read here rather than per
// request, so a failure to read it is reported at startup instead of as a broken
// page later.
func New(ctx context.Context, archive Archive, opts Options) (*Server, error) {
	opts = opts.withDefaults()

	s := &server{
		opts: opts, importer: opts.Importer, migrator: opts.Migrator,
		migration: newMigration(), background: ctx,
	}

	var open *opened
	if archive != nil {
		var err error
		open, err = s.prepare(ctx, archive)
		if err != nil {
			return nil, err
		}
		// An archive given at startup was opened by the command line, which built
		// or found the index separately and passes it here.
		if open.index == nil {
			open.index = opts.Index
		}
	}
	s.session = newSession(opts.Workspace, open)

	assets, err := builtViewer()
	if err != nil {
		return nil, fmt.Errorf("this binary was built without the viewer: %w", err)
	}
	if _, err := fs.Stat(assets, "index.html"); err != nil {
		return nil, fmt.Errorf("this binary was built without the viewer: %w", err)
	}
	s.assets = assets

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/archive", s.handleArchive)
	mux.HandleFunc("GET /api/chats", s.handleChats)
	mux.HandleFunc("GET /api/chats/{jid}/messages", s.handleMessages)
	mux.HandleFunc("GET /api/search", s.handleSearch)

	// The wizard, which is reachable before there is anything to read.
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/backups", s.handleBackups)
	mux.HandleFunc("POST /api/open", s.handleOpen)
	mux.HandleFunc("POST /api/extract", s.handleExtract)
	mux.HandleFunc("POST /api/decrypt", s.handleDecrypt)
	mux.HandleFunc("POST /api/close", s.handleClose)

	// Moving a history onto a phone. Nothing here moves from one stage to the next
	// on its own, and the last one needs a word typed out.
	mux.HandleFunc("GET /api/migration", s.handleMigration)
	mux.HandleFunc("GET /api/migration/guide", s.handleGuide)
	mux.HandleFunc("POST /api/migration/check", s.handleCheck)
	mux.HandleFunc("POST /api/migration/plan", s.handlePlanMigration)
	mux.HandleFunc("POST /api/migration/carry-out", s.handleCarryOut)
	mux.HandleFunc("POST /api/migration/forget", s.handleForgetMigration)

	// The built page names its stylesheet and script with a hash, so they are served
	// as a tree rather than one by one. Nothing outside it is reachable: the file
	// system is the embedded one and holds only what the build produced.
	mux.Handle("GET /assets/", s.immutable(http.FileServerFS(assets)))

	// The mark, served from the same binary as everything else. Not marked
	// immutable: its name carries no hash, so a browser that cached it forever
	// would keep an old one forever.
	mux.Handle("GET /favicon.svg", http.FileServerFS(assets))

	mux.HandleFunc("GET /{$}", s.handlePage)

	return &Server{handler: s.authenticated(mux), session: s.session}, nil
}

// prepare reads everything from an archive that is needed on every request.
func (s *server) prepare(ctx context.Context, archive Archive) (*opened, error) {
	chats, err := archive.Chats(ctx)
	if err != nil {
		return nil, err
	}

	open := &opened{
		archive: archive,
		index:   indexOver(archive),
		byJID:   make(map[string]model.Chat, len(chats)),
		export: export.Options{
			Names:    archive.Directory(),
			Location: s.opts.Location,
			Me:       s.opts.Me,
		},
	}
	for _, chat := range chats {
		if !chat.Includable() {
			continue
		}
		open.chats = append(open.chats, chat)
		open.byJID[chat.JID.String()] = chat
	}
	// Ordered once here rather than on every request. A real archive holds several
	// thousand conversations, and the order they are listed in cannot change while
	// that archive is the one open.
	open.chats = sortedByRecency(open.chats)
	return open, nil
}

// indexOver returns the search index an archive brought with it, if any.
func indexOver(archive Archive) *search.Index {
	searchable, ok := archive.(Searchable)
	if !ok {
		return nil
	}
	return searchable.Index()
}

// reading returns the open archive, or answers the caller when there is none.
//
// A page that asks for conversations before the wizard has finished is not an
// error to be logged; it is a page that has not caught up, and the status says so
// plainly enough for it to.
func (s *server) reading(w http.ResponseWriter) (*opened, bool) {
	open := s.session.archive()
	if open == nil {
		http.Error(w, "no archive is open yet", http.StatusConflict)
		return nil, false
	}
	return open, true
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
	open, ok := s.reading(w)
	if !ok {
		return
	}
	write(w, r, s.summarise(open))
}

// summarise is what an archive says about itself, which the wizard also shows the
// moment one becomes readable.
func (s *server) summarise(open *opened) map[string]any {
	var (
		messages int
		earliest time.Time
		latest   time.Time
		byKind   = make(map[string]int)
	)
	for _, chat := range open.chats {
		messages += chat.Messages
		byKind[chat.Kind.String()]++
		if !chat.CreatedAt.IsZero() && (earliest.IsZero() || chat.CreatedAt.Before(earliest)) {
			earliest = chat.CreatedAt
		}
		if chat.LastAt.After(latest) {
			latest = chat.LastAt
		}
	}

	names := open.archive.Directory()
	body := map[string]any{
		"title":         s.opts.Title,
		"layout":        open.archive.Layout(),
		"conversations": len(open.chats),
		"messages":      messages,
		"by_kind":       byKind,
		"people":        names.Len(),
		"named":         names.Identified(),
		"searchable":    open.index != nil,
		"time_zone":     s.opts.Location.String(),
	}
	if !earliest.IsZero() {
		body["earliest"] = earliest.UTC()
	}
	if !latest.IsZero() {
		body["latest"] = latest.UTC()
	}
	return body
}

// handleChats lists conversations, most recently used first.
func (s *server) handleChats(w http.ResponseWriter, r *http.Request) {
	open, ok := s.reading(w)
	if !ok {
		return
	}

	var (
		q             = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		limit         = intParam(r, "limit", 200, 1000)
		offset        = intParam(r, "offset", 0, 1<<30)
		matched       = make([]model.Chat, 0, limit)
		total         int
		skipRemaining = offset
	)

	for _, chat := range open.chats {
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
		list = append(list, export.ChatValue(chat, open.export))
	}
	write(w, r, map[string]any{"total": total, "chats": list})
}

// handleMessages returns a page of one conversation, ending at a position and
// reading backwards, which is the order somebody reads a conversation in.
func (s *server) handleMessages(w http.ResponseWriter, r *http.Request) {
	open, reading := s.reading(w)
	if !reading {
		return
	}

	chat, ok := open.byJID[r.PathValue("jid")]
	if !ok {
		http.Error(w, "no such conversation", http.StatusNotFound)
		return
	}

	before, err := parseCursor(r.URL.Query().Get("before"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	page, next, err := open.archive.Page(r.Context(), chat, before, intParam(r, "limit", 60, 500))
	if err != nil {
		fail(w, r, err)
		return
	}

	messages := make([]any, 0, len(page))
	for _, m := range page {
		if !m.Displayable() {
			continue
		}
		messages = append(messages, export.MessageValue(m, open.export))
	}

	body := map[string]any{
		"chat":     export.ChatValue(chat, open.export),
		"messages": messages,
	}
	if !next.IsZero() {
		body["before"] = formatCursor(next)
	}
	write(w, r, body)
}

// handleSearch finds messages anywhere in the archive.
func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	open, ok := s.reading(w)
	if !ok {
		return
	}
	if open.index == nil {
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

	hits, err := open.index.Search(r.Context(), term, query)
	if err != nil {
		if errors.Is(err, search.ErrEmptyQuery) {
			write(w, r, map[string]any{"total": 0, "hits": []any{}})
			return
		}
		fail(w, r, err)
		return
	}
	total, err := open.index.Count(r.Context(), term, query)
	if err != nil {
		fail(w, r, err)
		return
	}

	results := make([]any, 0, len(hits))
	for _, hit := range hits {
		// The conversation is named the same way here as it is everywhere else in
		// this API. It used to be "chat" and "chat_jid" only in a search result,
		// which meant a caller had to know two vocabularies for one thing.
		results = append(results, map[string]any{
			"chat_name":    hit.Chat,
			"chat_address": hit.ChatJID,
			"sender":       hit.Sender,
			"from_me":      hit.FromMe,
			"sent_at":      hit.SentAt,
			"kind":         hit.Kind,
			"snippet":      hit.Snippet,
		})
	}
	write(w, r, map[string]any{"total": total, "hits": results})
}

// handlePage serves the viewer itself.
func (s *server) handlePage(w http.ResponseWriter, r *http.Request) {
	page, err := fs.ReadFile(s.assets, "index.html")
	if err != nil {
		fail(w, r, fmt.Errorf("the viewer is missing from this binary: %w", err))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The page loads nothing but itself. Said here as well as meant, so a mistake in
	// the page cannot quietly start fetching from somewhere else. The pictures are
	// data URIs carried in the messages, which is why images allow that and nothing
	// else does.
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'")
	if _, err := w.Write(page); err != nil {
		return
	}
}

// immutable marks the built files as never changing.
//
// Their names carry a hash of their contents, so a browser that has one has the
// right one for as long as this binary is the one serving it. The no-store rule
// applied to everything else stays where it belongs, on the archive's own contents.
func (s *server) immutable(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
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
	writeStatus(w, r, http.StatusOK, body)
}

// writeStatus sends a JSON response with a particular status.
//
// The header is set before the status rather than after, which is the whole reason
// this is one function: a header set once the status has gone out is silently
// dropped, and the caller is left wondering why their JSON arrived as text.
func writeStatus(w http.ResponseWriter, r *http.Request, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

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
