package ios

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

// pageSize is how many messages are read at a time.
//
// A page is one query and one pass, and the working set is the page itself, so
// this is the memory the reader uses regardless of how long a conversation is.
// Two thousand rows of an iPhone store is a few megabytes.
const pageSize = 2000

// Messages streams one conversation in the order it was written.
//
// Iteration stops at the first error, which is yielded with a zero message. The
// caller may stop early; the underlying statements are released either way.
func (r *Reader) Messages(ctx context.Context, chat model.Chat) iter.Seq2[model.Message, error] {
	return func(yield func(model.Message, error) bool) {
		var from model.Cursor
		for {
			page, err := r.read(ctx, chat, from, pageSize, forwards)
			if err != nil {
				yield(model.Message{}, err)
				return
			}
			if len(page) == 0 {
				return
			}
			if err := r.enrich(ctx, page); err != nil {
				yield(model.Message{}, err)
				return
			}
			for i := range page {
				if !yield(page[i], nil) {
					return
				}
			}
			if len(page) < pageSize {
				return
			}
			from = page[len(page)-1].At()
		}
	}
}

// Page reads up to limit messages ending just before a position, oldest first.
//
// The cursor returned names where the next page back begins, and is zero once the
// beginning of the conversation has been reached.
func (r *Reader) Page(ctx context.Context, chat model.Chat, before model.Cursor, limit int) ([]model.Message, model.Cursor, error) {
	if limit <= 0 {
		limit = 50
	}

	page, err := r.read(ctx, chat, before, limit, backwards)
	if err != nil {
		return nil, model.Cursor{}, err
	}
	if len(page) == 0 {
		return nil, model.Cursor{}, nil
	}
	if err := r.enrich(ctx, page); err != nil {
		return nil, model.Cursor{}, err
	}

	// Read newest first so the limit takes the most recent, then turned around so a
	// caller always sees a conversation in the order it was written.
	slices.Reverse(page)

	var next model.Cursor
	if len(page) == limit {
		next = page[0].At()
	}
	return page, next, nil
}

// direction is which way through a conversation a page is read.
type direction bool

const (
	forwards  direction = false
	backwards direction = true
)

// read fetches one batch of messages from a position.
//
// The media row is joined in rather than fetched afterwards, because it is at most
// one per message and carries most of what a message is: the file it referred to,
// the place it pointed at, the card it shared, and the protobuf that says what it
// was answering. One query and one pass beats a second query and a map.
func (r *Reader) read(ctx context.Context, chat model.Chat, from model.Cursor, limit int, back direction) ([]model.Message, error) {
	column, at, err := r.boundOf(ctx, from, back)
	if err != nil {
		return nil, err
	}

	// Reading forwards starts before the first message and backwards after the
	// last. A cursor of nothing means whichever end that is, written as a bound the
	// comparison is always true against, so one query shape serves both directions.
	bound := fmt.Sprintf("m.%s > :at OR (m.%s = :at AND m.Z_PK > :id)", column, column)
	order := fmt.Sprintf("m.%s, m.Z_PK", column)
	if back {
		bound = fmt.Sprintf("m.%s < :at OR (m.%s = :at AND m.Z_PK < :id)", column, column)
		order = fmt.Sprintf("m.%s DESC, m.Z_PK DESC", column)
	}
	id := from.ID
	if from.IsZero() {
		id = math.MaxInt64
		if !back {
			id = 0
		}
	}

	// The columns are listed rather than interpolated one by one. A format string
	// with nineteen verbs in it is one somebody adds a column to and gets wrong, and
	// the scan below has to stay in step with this order.
	columns := []string{
		"m.Z_PK", "m.ZISFROMME", "m.ZMESSAGEDATE", "m.ZMESSAGETYPE", "m.ZTEXT",
		r.schema.columnAs(tableMessage, "m", "ZSTANZAID"),
		r.schema.columnAs(tableMessage, "m", "ZFROMJID"),
		r.schema.columnAs(tableMessage, "m", "ZGROUPMEMBER"),
		r.schema.columnAs(tableMessage, "m", "ZPUSHNAME"),
		r.schema.columnAs(tableMessage, "m", "ZSTARRED"),
		r.schema.columnAs(tableMessage, "m", "ZGROUPEVENTTYPE"),
		r.schema.columnAs(tableMedia, "mi", "ZVCARDSTRING"),
		r.schema.columnAs(tableMedia, "mi", "ZVCARDNAME"),
		r.schema.columnAs(tableMedia, "mi", "ZTITLE"),
		r.schema.columnAs(tableMedia, "mi", "ZFILESIZE"),
		r.schema.columnAs(tableMedia, "mi", "ZMOVIEDURATION"),
		r.schema.columnAs(tableMedia, "mi", "ZLATITUDE"),
		r.schema.columnAs(tableMedia, "mi", "ZLONGITUDE"),
		r.schema.columnAs(tableMedia, "mi", "ZMEDIALOCALPATH"),
		r.schema.columnAs(tableMedia, "mi", "ZMETADATA"),
		r.schema.columnAs(tableMedia, "mi", "ZXMPPTHUMBPATH"),
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM %s m
		LEFT JOIN %s mi ON mi.Z_PK = m.ZMEDIAITEM
		WHERE m.ZCHATSESSION = :chat AND (%s)
		ORDER BY %s
		LIMIT :limit`,
		strings.Join(columns, ", "), tableMessage, tableMedia, bound, order)

	rows, err := r.db.QueryContext(ctx, query,
		sql.Named("at", at), sql.Named("id", id),
		sql.Named("chat", chat.ID), sql.Named("limit", limit))
	if err != nil {
		return nil, ErrUnreadable.withCause(fmt.Errorf("reading messages: %w", err))
	}
	defer rows.Close()

	page := make([]model.Message, 0, limit)
	for rows.Next() {
		var row messageRow
		if err := row.scan(rows); err != nil {
			return nil, ErrUnreadable.withCause(fmt.Errorf("reading a message: %w", err))
		}
		page = append(page, r.messageOf(row, chat))
	}
	if err := rows.Err(); err != nil {
		return nil, ErrUnreadable.withCause(fmt.Errorf("reading messages: %w", err))
	}
	return page, nil
}

// messageRow is one joined row, held only as long as it takes to turn it into a
// message.
type messageRow struct {
	pk        int64
	fromMe    sql.NullBool
	sentAt    sql.NullFloat64
	kind      sql.NullInt64
	text      sql.NullString
	stanza    sql.NullString
	fromJID   sql.NullString
	member    sql.NullInt64
	pushName  sql.NullString
	starred   sql.NullBool
	eventType sql.NullInt64
	mediaType sql.NullString
	vcardName sql.NullString
	title     sql.NullString
	fileSize  sql.NullInt64
	duration  sql.NullInt64
	latitude  sql.NullFloat64
	longitude sql.NullFloat64
	localPath sql.NullString
	metadata  []byte
	// thumbPath names a picture that lives outside the store. It is the only way an
	// iPhone archive shows a photograph at all; see media.go.
	thumbPath sql.NullString
}

func (row *messageRow) scan(rows *sql.Rows) error {
	return rows.Scan(&row.pk, &row.fromMe, &row.sentAt, &row.kind, &row.text,
		&row.stanza, &row.fromJID, &row.member, &row.pushName, &row.starred, &row.eventType,
		&row.mediaType, &row.vcardName, &row.title, &row.fileSize, &row.duration,
		&row.latitude, &row.longitude, &row.localPath, &row.metadata, &row.thumbPath)
}

// messageOf turns one row into a message.
func (r *Reader) messageOf(row messageRow, chat model.Chat) model.Message {
	sourceType := int(row.kind.Int64)

	m := model.Message{
		ID:         row.pk,
		ChatID:     chat.ID,
		Key:        row.stanza.String,
		Kind:       kindOf(sourceType, row.mediaType.String),
		SentAt:     coreDataTime(row.sentAt),
		Text:       row.text.String,
		PushName:   row.pushName.String,
		Starred:    row.starred.Bool,
		SourceType: sourceType,
	}

	if row.fromMe.Bool {
		m = m.NewOutgoing()
	} else {
		m.Sender = r.senderOf(row, chat)
		m.PushName = firstNonEmpty(m.PushName, r.directory.Lookup(m.Sender).PushName)
	}

	r.attachContent(&m, row, sourceType)
	return m
}

// senderOf resolves who wrote an incoming message.
//
// In a group the message names a member row and nothing else; in a one-to-one
// conversation the sender is whoever the conversation is with, which the store
// often leaves out because there is only one person it could be.
func (r *Reader) senderOf(row messageRow, chat model.Chat) model.JID {
	if row.member.Valid {
		if jid, known := r.members[row.member.Int64]; known {
			return jid
		}
	}
	if jid := model.ParseJID(row.fromJID.String); !jid.IsZero() && !jid.IsGroup() {
		return jid
	}
	if chat.Kind == model.ChatDirect {
		return chat.JID
	}
	return model.JID{}
}

// attachContent fills in everything a message carried beyond its words.
func (r *Reader) attachContent(m *model.Message, row messageRow, sourceType int) {
	switch {
	case isSystem(sourceType):
		// What happened is kept as the store's own code. This build does not have a
		// verified table of what those codes mean on iOS, and inventing one would put
		// sentences in an archive that nobody said, so the code is carried through
		// and the text the store wrote is used when it wrote any.
		m.Notice = &model.Notice{Action: int(row.eventType.Int64)}
		m.SystemText = row.text.String

	case sourceType == typeDeleted:
		// The store records who removed it in the field it otherwise uses for a
		// media type, which is only ever an address here.
		m.Deleted = &model.Deletion{At: m.SentAt}
		if by := model.ParseJID(row.mediaType.String); !by.IsZero() {
			m.Deleted.By = by
		}

	case sourceType == typeContact:
		// The card is kept exactly as it was sent; parsing it would lose something.
		if row.mediaType.String != "" {
			m.Contacts = append(m.Contacts, model.ContactCard{
				Name:  row.vcardName.String,
				VCard: row.mediaType.String,
			})
		}

	case sourceType == typeLocation:
		m.Place = &model.Place{
			Latitude:  row.latitude.Float64,
			Longitude: row.longitude.Float64,
			Name:      row.title.String,
		}

	case m.Kind.HasAttachment():
		m.Attachment = &model.Attachment{
			MediaType: row.mediaType.String,
			FileName:  fileNameOf(row),
			Size:      row.fileSize.Int64,
			Duration:  time.Duration(row.duration.Int64) * time.Second,
			Caption:   row.text.String,
		}
	}

	r.attachPicture(m, row)

	// A reply names what it answers in the protobuf beside it, whatever kind of
	// message it is.
	if len(row.metadata) > 0 {
		if quoted := decodeContext(row.metadata); !quoted.IsEmpty() {
			m.Quote = &model.Quote{
				Sender: model.ParseJID(quoted.QuotedSender),
				Kind:   model.KindText,
				Text:   quoted.QuotedText,
			}
		}
	}
}

// attachPicture fetches the small copy of a photograph that survived its file.
//
// An iPhone store holds only the path; whoever opened the reader decides whether
// anything can follow it. A picture that cannot be found is not an error: a backup
// can be incomplete, and on a real device 519 of 9,941 paths led nowhere.
//
// The attachment is created when there is none, because the picture is worth more
// than the row that should have described it: a message whose media row is missing
// is exactly the case where the surviving image matters most.
func (r *Reader) attachPicture(m *model.Message, row messageRow) {
	if r.media == nil || !row.thumbPath.Valid || row.thumbPath.String == "" {
		return
	}
	picture, found := r.media.ReadFile(row.thumbPath.String)
	if !found || len(picture) == 0 {
		return
	}

	if m.Attachment == nil {
		m.Attachment = &model.Attachment{}
	}
	m.Attachment.Preview = model.Thumbnail{Data: picture}
}

// The numeric types this reader treats specially. The rest are decided by kindOf.
const (
	typeContact  = 4
	typeLocation = 5
	typeDeleted  = 14
)

// fileNameOf is what to call the file a message referred to.
//
// The store keeps a title for documents and the path it saved the file at for
// everything else. Only the last part of that path is a name; the rest says where
// the file sat on a phone that may no longer exist.
func fileNameOf(row messageRow) string {
	if row.title.Valid && row.title.String != "" {
		return row.title.String
	}
	if path := row.localPath.String; path != "" {
		if cut := strings.LastIndexByte(path, '/'); cut >= 0 {
			return path[cut+1:]
		}
		return path
	}
	return ""
}

// enrich attaches what could not be joined onto the main query.
//
// Only link previews land here, because a message can carry more than one data
// item and joining them would have duplicated messages. It is one query for the
// whole page.
func (r *Reader) enrich(ctx context.Context, page []model.Message) error {
	if !r.schema.hasColumn(tableDataItem, "ZMESSAGE") || len(page) == 0 {
		return nil
	}

	index := make(map[int64]int, len(page))
	ids := make([]any, 0, len(page))
	placeholders := make([]string, 0, len(page))
	for i := range page {
		index[page[i].ID] = i
		ids = append(ids, page[i].ID)
		placeholders = append(placeholders, "?")
	}

	// The placeholders are counted here, never supplied by a caller, and every value
	// is still bound.
	query := fmt.Sprintf(
		`SELECT ZMESSAGE, %s, %s, %s FROM %s WHERE ZMESSAGE IN (%s)`,
		r.schema.columnOrNull(tableDataItem, "ZTITLE"),
		r.schema.columnOrNull(tableDataItem, "ZSUMMARY"),
		r.schema.columnOrNull(tableDataItem, "ZMATCHEDTEXT"),
		tableDataItem, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, ids...)
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading link previews: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var (
			message                 int64
			title, summary, matched sql.NullString
		)
		if err := rows.Scan(&message, &title, &summary, &matched); err != nil {
			return ErrUnreadable.withCause(fmt.Errorf("reading a link preview: %w", err))
		}
		at, known := index[message]
		if !known {
			continue
		}
		preview := model.LinkPreview{
			URL:         matched.String,
			Title:       title.String,
			Description: summary.String,
		}
		if !preview.IsEmpty() {
			page[at].Link = &preview
		}
	}
	if err := rows.Err(); err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading link previews: %w", err))
	}
	return nil
}

// boundOf decides which column the pages are cut on, and where this page starts.
//
// It matters more than it looks. The obvious column to order a conversation by is
// the date, and it is the wrong one: the store has no index over a conversation and
// a date, so every page sorts the entire conversation in a temporary table before
// throwing away all but fifty rows. On a conversation of 89,894 messages that made
// reading it backwards take fifteen seconds.
//
// ZSORT is the store's own display order and it is indexed together with the
// conversation, so a page reads only the rows it returns. On the same conversation
// that is a hundred times faster, and it agrees with the date exactly: across a
// million messages there was not one pair the two orders disagreed about.
//
// The date remains the fallback, because a store without ZSORT should be slow
// rather than unreadable.
func (r *Reader) boundOf(ctx context.Context, from model.Cursor, back direction) (column string, at float64, err error) {
	if !r.schema.hasColumn(tableMessage, "ZSORT") {
		if from.IsZero() {
			if back {
				return "ZMESSAGEDATE", math.MaxInt32, nil
			}
			return "ZMESSAGEDATE", 0, nil
		}
		return "ZMESSAGEDATE", coreDataSeconds(from.SentAt), nil
	}

	if from.IsZero() {
		if back {
			return "ZSORT", math.MaxInt64, nil
		}
		return "ZSORT", math.MinInt64, nil
	}

	// The cursor names a message, not a sort value, so that a position means the
	// same thing whichever source produced it. Looking the value up is one hit on
	// the primary key, next to nothing beside the page it bounds.
	var sort sql.NullFloat64
	row := r.db.QueryRowContext(ctx,
		`SELECT ZSORT FROM `+tableMessage+` WHERE Z_PK = ?`, from.ID)
	if err := row.Scan(&sort); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// The message a caller is paging from is gone. Starting from the end is
			// better than refusing to show the conversation at all.
			if back {
				return "ZSORT", math.MaxInt64, nil
			}
			return "ZSORT", math.MinInt64, nil
		}
		return "", 0, ErrUnreadable.withCause(fmt.Errorf("finding a position: %w", err))
	}
	return "ZSORT", sort.Float64, nil
}

// coreDataSeconds converts a time back to the store's own count.
func coreDataSeconds(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	return t.Sub(appleEpoch).Seconds()
}

// firstNonEmpty returns the first value with anything in it.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
