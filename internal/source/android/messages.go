package android

import (
	"context"
	"database/sql"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

// pageSize is how many messages are read, and enriched, at a time.
//
// Enrichment costs one query per kind of detail per page, so a larger page means
// fewer queries; a smaller one means flatter memory. Two thousand keeps the working
// set to a few megabytes while reducing a conversation of a hundred thousand
// messages to a few hundred queries instead of half a million.
const pageSize = 2000

// Messages streams one conversation in the order it was written.
//
// Iteration stops at the first error, which is yielded with a zero message. The
// caller may stop early; the underlying statements are released either way.
func (r *Reader) Messages(ctx context.Context, chat model.Chat) iter.Seq2[model.Message, error] {
	return func(yield func(model.Message, error) bool) {
		var (
			afterTime int64
			afterID   int64
		)
		for {
			page, err := r.readPage(ctx, chat, afterTime, afterID)
			if err != nil {
				yield(model.Message{}, err)
				return
			}
			if len(page) == 0 {
				return
			}
			if err := r.enrich(ctx, chat, page); err != nil {
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
			last := page[len(page)-1]
			afterTime, afterID = last.SentAt.UnixMilli(), last.ID
		}
	}
}

// readPage fetches the next batch of messages after a position, without their
// details. Paging on the sort key rather than an offset keeps every page cheap no
// matter how deep into a conversation it is.
func (r *Reader) readPage(ctx context.Context, chat model.Chat, afterTime, afterID int64) ([]model.Message, error) {
	origin := r.schema.columnOrNull("message", "origin")
	starred := r.schema.columnOrNull("message", "starred")
	flags := r.schema.columnOrNull("message", "origination_flags")

	query := fmt.Sprintf(`
		SELECT message._id, message.from_me, message.sender_jid_row_id, message.timestamp,
		       message.message_type, message.text_data, message.key_id, %s, %s, %s
		FROM message
		WHERE message.chat_row_id = ?
		  AND (message.timestamp > ? OR (message.timestamp = ? AND message._id > ?))
		ORDER BY message.timestamp, message._id
		LIMIT ?`, origin, starred, flags)

	rows, err := r.db.QueryContext(ctx, query, chat.ID, afterTime, afterTime, afterID, pageSize)
	if err != nil {
		return nil, ErrUnreadable.withCause(fmt.Errorf("reading messages: %w", err))
	}
	defer rows.Close()

	page := make([]model.Message, 0, pageSize)
	for rows.Next() {
		var (
			id        int64
			fromMe    sql.NullBool
			senderRow sql.NullInt64
			timestamp sql.NullInt64
			kindCode  sql.NullInt64
			text      sql.NullString
			key       sql.NullString
			origin    sql.NullInt64
			starred   sql.NullBool
			flags     sql.NullInt64
		)
		if err := rows.Scan(&id, &fromMe, &senderRow, &timestamp, &kindCode,
			&text, &key, &origin, &starred, &flags); err != nil {
			return nil, ErrUnreadable.withCause(fmt.Errorf("reading a message: %w", err))
		}

		m := model.Message{
			ID:         id,
			ChatID:     chat.ID,
			Key:        key.String,
			Kind:       kindOf(int(kindCode.Int64), int(origin.Int64)),
			SentAt:     epochMillis(timestamp),
			Text:       text.String,
			Starred:    starred.Bool,
			Forwarded:  flags.Valid && flags.Int64&1 != 0,
			SourceType: int(kindCode.Int64),
		}
		if fromMe.Bool {
			m = m.NewOutgoing()
		} else {
			m.Sender = r.senderOf(senderRow, chat)
			m.PushName = r.directory.Lookup(m.Sender).PushName
		}
		page = append(page, m)
	}
	if err := rows.Err(); err != nil {
		return nil, ErrUnreadable.withCause(fmt.Errorf("reading messages: %w", err))
	}
	return page, nil
}

// senderOf resolves who wrote an incoming message. In a one-to-one conversation the
// sender column is often empty because there is only one person it could be, so the
// conversation's own address is the answer.
func (r *Reader) senderOf(senderRow sql.NullInt64, chat model.Chat) model.JID {
	if senderRow.Valid {
		if j, ok := r.jids[senderRow.Int64]; ok {
			return j
		}
	}
	if chat.Kind == model.ChatDirect {
		return chat.JID
	}
	return model.JID{}
}

// enrich attaches details to a page of messages. Each kind of detail is one query
// over the whole page, because joining them onto the main query would multiply rows
// and silently duplicate messages that carry several reactions.
func (r *Reader) enrich(ctx context.Context, chat model.Chat, page []model.Message) error {
	ids := make([]int64, len(page))
	index := make(map[int64]int, len(page))
	for i, m := range page {
		ids[i] = m.ID
		index[m.ID] = i
	}

	// Order matters in two places: quotes are read before quoted attachments,
	// which fill them in, and the media row is read before the embedded preview,
	// which may have to create an attachment the media table never had.
	for _, load := range []func(context.Context, []int64, map[int64]int, []model.Message) error{
		r.attachMedia,
		r.attachPreviews,
		r.attachQuotes,
		r.attachQuotedMedia,
		r.attachReactions,
		r.attachEdits,
		r.attachMentions,
		r.attachPlaces,
		r.attachPolls,
		r.attachCalls,
		r.attachLinks,
		r.attachContactCards,
		r.attachDeletions,
		r.attachForwardCounts,
		r.attachAlbums,
		r.attachExpiry,
		r.attachInvites,
		r.attachSystemNotices,
	} {
		if err := load(ctx, ids, index, page); err != nil {
			return err
		}
	}
	_ = chat
	return nil
}

// attachMedia adds the description of any file a message referred to.
func (r *Reader) attachMedia(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.has("message_media") {
		return nil
	}
	cols := []string{
		r.schema.columnOrNull("message_media", "mime_type"),
		r.schema.columnOrNull("message_media", "media_name"),
		r.schema.columnOrNull("message_media", "media_caption"),
		r.schema.columnOrNull("message_media", "media_duration"),
		r.schema.columnOrNull("message_media", "file_size"),
		r.schema.columnOrNull("message_media", "width"),
		r.schema.columnOrNull("message_media", "height"),
	}
	query := fmt.Sprintf(
		`SELECT message_media.message_row_id, %s FROM message_media WHERE message_media.message_row_id IN (%s)`,
		strings.Join(cols, ", "), placeholders(len(ids)))

	rows, err := r.db.QueryContext(ctx, query, asArgs(ids)...)
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading attachments: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var (
			messageID int64
			mediaType sql.NullString
			name      sql.NullString
			caption   sql.NullString
			duration  sql.NullInt64
			size      sql.NullInt64
			width     sql.NullInt64
			height    sql.NullInt64
		)
		if err := rows.Scan(&messageID, &mediaType, &name, &caption, &duration, &size, &width, &height); err != nil {
			return ErrUnreadable.withCause(fmt.Errorf("reading an attachment: %w", err))
		}
		i, ok := index[messageID]
		if !ok {
			continue
		}
		page[i].Attachment = &model.Attachment{
			MediaType: mediaType.String,
			FileName:  name.String,
			Size:      size.Int64,
			Duration:  time.Duration(duration.Int64) * time.Second,
			Width:     int(width.Int64),
			Height:    int(height.Int64),
			Caption:   caption.String,
		}
		// A caption is the message's words; the database keeps it beside the file
		// rather than in the message row.
		if page[i].Text == "" && caption.Valid {
			page[i].Text = caption.String
		}
	}
	return rows.Err()
}

// attachQuotes adds the message a reply pointed at.
func (r *Reader) attachQuotes(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.has("message_quoted") {
		return nil
	}
	query := fmt.Sprintf(`
		SELECT message_quoted.message_row_id, message_quoted.from_me,
		       message_quoted.sender_jid_row_id, message_quoted.message_type, message_quoted.text_data
		FROM message_quoted WHERE message_quoted.message_row_id IN (%s)`, placeholders(len(ids)))

	rows, err := r.db.QueryContext(ctx, query, asArgs(ids)...)
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading replies: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var (
			messageID int64
			fromMe    sql.NullBool
			senderRow sql.NullInt64
			kindCode  sql.NullInt64
			text      sql.NullString
		)
		if err := rows.Scan(&messageID, &fromMe, &senderRow, &kindCode, &text); err != nil {
			return ErrUnreadable.withCause(fmt.Errorf("reading a reply: %w", err))
		}
		i, ok := index[messageID]
		if !ok {
			continue
		}
		q := &model.Quote{
			FromMe: fromMe.Bool,
			Kind:   kindOf(int(kindCode.Int64), 0),
			Text:   text.String,
		}
		if !fromMe.Bool && senderRow.Valid {
			q.Sender = r.jids[senderRow.Int64]
		}
		page[i].Quote = q
	}
	return rows.Err()
}

// attachReactions adds the emoji people put on a message.
func (r *Reader) attachReactions(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.has("message_add_on") || !r.schema.has("message_add_on_reaction") {
		return nil
	}
	query := fmt.Sprintf(`
		SELECT message_add_on.parent_message_row_id, message_add_on.from_me,
		       message_add_on.sender_jid_row_id, message_add_on_reaction.reaction,
		       message_add_on_reaction.sender_timestamp
		FROM message_add_on
		JOIN message_add_on_reaction
		  ON message_add_on_reaction.message_add_on_row_id = message_add_on._id
		WHERE message_add_on.parent_message_row_id IN (%s)
		  AND message_add_on_reaction.reaction IS NOT NULL
		  AND message_add_on_reaction.reaction <> ''`, placeholders(len(ids)))

	rows, err := r.db.QueryContext(ctx, query, asArgs(ids)...)
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading reactions: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var (
			parentID  int64
			fromMe    sql.NullBool
			senderRow sql.NullInt64
			emoji     sql.NullString
			at        sql.NullInt64
		)
		if err := rows.Scan(&parentID, &fromMe, &senderRow, &emoji, &at); err != nil {
			return ErrUnreadable.withCause(fmt.Errorf("reading a reaction: %w", err))
		}
		i, ok := index[parentID]
		if !ok {
			continue
		}
		reaction := model.Reaction{FromMe: fromMe.Bool, Emoji: emoji.String, At: epochMillis(at)}
		if !fromMe.Bool && senderRow.Valid {
			reaction.Sender = r.jids[senderRow.Int64]
		}
		page[i].Reactions = append(page[i].Reactions, reaction)
	}
	return rows.Err()
}

// attachEdits marks messages that were changed after they were sent. WhatsApp keeps
// only the final wording, so an archive can say that an edit happened but not what
// the message used to say.
func (r *Reader) attachEdits(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.has("message_edit_info") {
		return nil
	}
	edited := r.schema.pick("message_edit_info", "edited_timestamp", "sender_timestamp")
	if edited == "" {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, %s FROM message_edit_info WHERE message_row_id IN (%s)`,
		edited, placeholders(len(ids)))

	rows, err := r.db.QueryContext(ctx, query, asArgs(ids)...)
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading edits: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var messageID int64
		var at sql.NullInt64
		if err := rows.Scan(&messageID, &at); err != nil {
			return ErrUnreadable.withCause(fmt.Errorf("reading an edit: %w", err))
		}
		i, ok := index[messageID]
		if !ok {
			continue
		}
		when := epochMillis(at)
		if when.IsZero() {
			// The row's presence is the fact that matters; a missing timestamp should
			// not erase it, so fall back to when the message was sent.
			when = page[i].SentAt
		}
		page[i].EditedAt = when
	}
	return rows.Err()
}

// attachMentions adds the people a message named.
func (r *Reader) attachMentions(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_mentions", "jid_row_id") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, jid_row_id FROM message_mentions WHERE message_row_id IN (%s)`,
		placeholders(len(ids)))

	rows, err := r.db.QueryContext(ctx, query, asArgs(ids)...)
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading mentions: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var messageID, jidRow int64
		if err := rows.Scan(&messageID, &jidRow); err != nil {
			return ErrUnreadable.withCause(fmt.Errorf("reading a mention: %w", err))
		}
		i, ok := index[messageID]
		if !ok {
			continue
		}
		if j, ok := r.jids[jidRow]; ok {
			page[i].Mentions = append(page[i].Mentions, j)
		}
	}
	return rows.Err()
}

// placeholders builds "?, ?, ?" for an IN clause.
func placeholders(n int) string {
	if n == 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// asArgs widens identifiers for the database driver.
func asArgs(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}
