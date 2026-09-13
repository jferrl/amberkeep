package search

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode"
)

// Turning what somebody typed into something the index will accept.
//
// A search box is not a query language, and nobody should see a syntax error for
// typing an apostrophe or a bracket. So every word is quoted before it reaches
// SQLite, which both neutralises the full-text syntax and keeps the words intact.
// What remains available on purpose is the small vocabulary people already expect
// from a search box: "a phrase in quotes", a trailing star for the start of a word,
// and a leading minus to exclude.

// Query narrows a search.
type Query struct {
	// Limit is how many results to return. Zero means a sensible default rather
	// than everything: an archive can match a common word a hundred thousand times.
	Limit int
	// Offset is where to start, for asking for the next page of results.
	Offset int

	// Chat restricts the search to one conversation, by its address.
	Chat string
	// Since and Until restrict the search to a period. The zero value means no bound.
	Since time.Time
	Until time.Time

	// Before and After wrap the words that matched, inside the snippet. Empty marks
	// leave the snippet unmarked.
	Before string
	After  string
}

// defaultLimit is how many results a search returns when the caller says nothing.
const defaultLimit = 50

func (q Query) withDefaults() Query {
	if q.Limit <= 0 {
		q.Limit = defaultLimit
	}
	return q
}

// MarkOpen and MarkClose wrap the words that matched, for a caller that will turn
// them into something visible rather than showing them.
//
// They are control characters because no message contains one, so nothing a person
// wrote can be mistaken for a mark. Not the null character, which looks like the
// obvious choice and is the wrong one: SQLite's own snippet function builds its
// output with C string handling, and a null passed in as a mark is silently dropped,
// which loses the opening of every match while leaving the closing in place.
const (
	MarkOpen  = "\x02"
	MarkClose = "\x03"
)

// Hit is one message the search found.
type Hit struct {
	Chat    string
	ChatJID string
	Sender  string
	FromMe  bool
	SentAt  time.Time
	Kind    string

	// Snippet is the part of the message around the words that matched, with those
	// words wrapped in the marks the query asked for.
	Snippet string

	// Rank is the index's own score. Smaller is a better match; it is exposed for
	// ordering results from more than one index, not for showing to anybody.
	Rank float64
}

// Search finds messages matching what somebody typed, best match first.
func (i *Index) Search(ctx context.Context, term string, q Query) ([]Hit, error) {
	q = q.withDefaults()

	expression, err := expression(term)
	if err != nil {
		return nil, err
	}

	// The bounds are written as "unset or matching" so that one statement serves
	// every combination of them, which keeps SQLite's own plan cache useful.
	const query = `
SELECT c.title, c.jid, m.sender, m.from_me, m.sent_at, m.kind,
       snippet(fts, 0, :before, :after, '…', 12), bm25(fts)
FROM fts
JOIN messages m ON m.id = fts.rowid
JOIN chats c ON c.id = m.chat_id
WHERE fts MATCH :match
  AND (:chat = '' OR c.jid = :chat)
  AND (:since = 0 OR m.sent_at >= :since)
  AND (:until = 0 OR m.sent_at <= :until)
ORDER BY bm25(fts)
LIMIT :limit OFFSET :offset`

	rows, err := i.db.QueryContext(ctx, query,
		sql.Named("before", q.Before),
		sql.Named("after", q.After),
		sql.Named("match", expression),
		sql.Named("chat", q.Chat),
		sql.Named("since", millis(q.Since)),
		sql.Named("until", millis(q.Until)),
		sql.Named("limit", q.Limit),
		sql.Named("offset", q.Offset))
	if err != nil {
		return nil, ErrUnreadable.withCause(err)
	}
	defer func() { _ = rows.Close() }()

	var hits []Hit
	for rows.Next() {
		var (
			hit  Hit
			sent int64
		)
		if err := rows.Scan(&hit.Chat, &hit.ChatJID, &hit.Sender, &hit.FromMe,
			&sent, &hit.Kind, &hit.Snippet, &hit.Rank); err != nil {
			return nil, ErrUnreadable.withCause(err)
		}
		hit.SentAt = time.UnixMilli(sent).UTC()
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, ErrUnreadable.withCause(err)
	}
	return hits, nil
}

// Count is how many messages match, which is usually far more than are shown.
func (i *Index) Count(ctx context.Context, term string, q Query) (int, error) {
	expression, err := expression(term)
	if err != nil {
		return 0, err
	}

	const query = `
SELECT count(*)
FROM fts
JOIN messages m ON m.id = fts.rowid
JOIN chats c ON c.id = m.chat_id
WHERE fts MATCH :match
  AND (:chat = '' OR c.jid = :chat)
  AND (:since = 0 OR m.sent_at >= :since)
  AND (:until = 0 OR m.sent_at <= :until)`

	var n int
	row := i.db.QueryRowContext(ctx, query,
		sql.Named("match", expression),
		sql.Named("chat", q.Chat),
		sql.Named("since", millis(q.Since)),
		sql.Named("until", millis(q.Until)))
	if err := row.Scan(&n); err != nil {
		return 0, ErrUnreadable.withCause(err)
	}
	return n, nil
}

// millis is a time as the index stores it, or zero for no bound.
func millis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// expression turns what somebody typed into a full-text query.
//
// Every word is wrapped in quotes, which is what makes this safe: inside quotes
// the full-text syntax has no special characters left, so an apostrophe, a bracket
// or the word "AND" is searched for rather than interpreted.
func expression(term string) (string, error) {
	var include, exclude []string

	for _, token := range tokenize(term) {
		negated := strings.HasPrefix(token, "-") && len(token) > 1
		if negated {
			token = token[1:]
		}
		prefix := strings.HasSuffix(token, "*") && len(token) > 1
		if prefix {
			token = strings.TrimSuffix(token, "*")
		}
		if !hasWord(token) {
			// Punctuation alone matches nothing and is a syntax error unquoted.
			continue
		}

		// Doubling any quote inside the token is belt and braces: the tokenizer
		// above cannot produce one today, and this is what keeps that from
		// becoming a full-text syntax error if it ever can.
		quoted := `"` + strings.ReplaceAll(token, `"`, `""`) + `"`
		if prefix {
			quoted += "*"
		}
		if negated {
			exclude = append(exclude, quoted)
		} else {
			include = append(include, quoted)
		}
	}

	if len(include) == 0 {
		// A search for absence alone would match every message that does not
		// contain a word, which is not a search anybody meant to run.
		return "", ErrEmptyQuery
	}

	out := strings.Join(include, " AND ")
	for _, term := range exclude {
		out += " NOT " + term
	}
	return out, nil
}

// tokenize splits what somebody typed into words, keeping "quoted phrases" whole.
//
// An unclosed quote is treated as if it were closed at the end, because somebody
// halfway through typing a phrase should see results, not an error.
func tokenize(s string) []string {
	var (
		tokens  []string
		current strings.Builder
		quoted  bool
	)
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for _, r := range s {
		switch {
		case r == '"':
			// A quote always ends whatever came before it, so a stray one in the
			// middle of a word splits it rather than disappearing into it.
			flush()
			quoted = !quoted
		case unicode.IsSpace(r) && !quoted:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}

// hasWord reports whether a token holds anything the index could match.
func hasWord(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
