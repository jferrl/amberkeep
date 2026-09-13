package android

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

// This file recovers everything a message carried beyond plain words. WhatsApp
// keeps each kind of content in its own table, so a reader that stops at the
// message table turns a shared place, a poll, a call or a contact card into a
// blank line.
//
// The most valuable of these is the embedded picture preview. Media files live
// outside the database and are usually long gone by the time anyone needs an
// archive, but a small copy of every photo and video survives here.

// perMessage runs one query over a page of message identifiers and hands each row
// to apply. Every recovery below has this shape: one query per page, never one per
// message, and never a join that would multiply the messages themselves.
func (r *Reader) perMessage(ctx context.Context, what, query string, ids []int64, apply func(*sql.Rows) error) error {
	rows, err := r.db.QueryContext(ctx, query, asArgs(ids)...)
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading %s: %w", what, err))
	}
	defer rows.Close()

	for rows.Next() {
		if err := apply(rows); err != nil {
			return ErrUnreadable.withCause(fmt.Errorf("reading %s: %w", what, err))
		}
	}
	if err := rows.Err(); err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading %s: %w", what, err))
	}
	return nil
}

// attachPreviews recovers the small copy of each picture and video that WhatsApp
// keeps inside the database.
//
// This is the single most valuable recovery in the whole reader. The original
// files are gone from most old archives, and without this a decade of photographs
// becomes a decade of the words "image omitted".
func (r *Reader) attachPreviews(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_thumbnail", "thumbnail") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, thumbnail FROM message_thumbnail
		 WHERE message_row_id IN (%s) AND thumbnail IS NOT NULL`, placeholders(len(ids)))

	return r.perMessage(ctx, "picture previews", query, ids, func(rows *sql.Rows) error {
		var id int64
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || len(data) == 0 {
			return nil
		}
		// A preview can exist for a message whose own media row is missing, which is
		// exactly the case where it matters most, so the attachment is created here
		// rather than assumed to exist already.
		if page[i].Attachment == nil {
			page[i].Attachment = &model.Attachment{}
		}
		page[i].Attachment.Preview = model.Thumbnail{Data: data}
		return nil
	})
}

// attachPlaces recovers shared locations.
func (r *Reader) attachPlaces(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_location", "latitude") {
		return nil
	}
	cols := []string{
		"message_location.latitude", "message_location.longitude",
		r.schema.columnOrNull("message_location", "place_name"),
		r.schema.columnOrNull("message_location", "place_address"),
		r.schema.columnOrNull("message_location", "url"),
		r.schema.columnOrNull("message_location", "live_location_share_duration"),
	}
	query := fmt.Sprintf(
		`SELECT message_location.message_row_id, %s, %s, %s, %s, %s, %s
		 FROM message_location WHERE message_location.message_row_id IN (%s)`,
		cols[0], cols[1], cols[2], cols[3], cols[4], cols[5], placeholders(len(ids)))

	return r.perMessage(ctx, "shared locations", query, ids, func(rows *sql.Rows) error {
		var (
			id       int64
			lat, lon sql.NullFloat64
			name     sql.NullString
			address  sql.NullString
			url      sql.NullString
			live     sql.NullInt64
		)
		if err := rows.Scan(&id, &lat, &lon, &name, &address, &url, &live); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok {
			return nil
		}
		page[i].Place = &model.Place{
			Latitude:  lat.Float64,
			Longitude: lon.Float64,
			Name:      name.String,
			Address:   address.String,
			URL:       url.String,
			Live:      live.Valid && live.Int64 > 0,
			SharedFor: time.Duration(live.Int64) * time.Second,
		}
		return nil
	})
}

// attachPolls recovers the answers a poll offered and how many people chose each.
// The question itself is the message's own text.
func (r *Reader) attachPolls(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_poll_option", "option_name") {
		return nil
	}

	if r.schema.has("message_poll") {
		query := fmt.Sprintf(
			`SELECT message_row_id, %s, %s FROM message_poll WHERE message_row_id IN (%s)`,
			r.schema.columnOrNull("message_poll", "selectable_options_count"),
			r.schema.columnOrNull("message_poll", "end_time"), placeholders(len(ids)))

		if err := r.perMessage(ctx, "polls", query, ids, func(rows *sql.Rows) error {
			var id int64
			var selectable, endTime sql.NullInt64
			if err := rows.Scan(&id, &selectable, &endTime); err != nil {
				return err
			}
			i, ok := index[id]
			if !ok {
				return nil
			}
			page[i].Poll = &model.Poll{
				Question:   page[i].Text,
				Selectable: int(selectable.Int64),
				Closed:     endTime.Valid && endTime.Int64 > 0,
			}
			return nil
		}); err != nil {
			return err
		}
	}

	query := fmt.Sprintf(
		`SELECT message_row_id, option_name, %s FROM message_poll_option
		 WHERE message_row_id IN (%s) ORDER BY _id`,
		r.schema.columnOrNull("message_poll_option", "vote_total"), placeholders(len(ids)))

	return r.perMessage(ctx, "poll answers", query, ids, func(rows *sql.Rows) error {
		var id int64
		var name sql.NullString
		var votes sql.NullInt64
		if err := rows.Scan(&id, &name, &votes); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok {
			return nil
		}
		// A database can hold answers for a poll whose own row is missing. Keeping
		// the answers is better than discarding them for want of a header.
		if page[i].Poll == nil {
			page[i].Poll = &model.Poll{Question: page[i].Text}
		}
		page[i].Poll.Options = append(page[i].Poll.Options, model.PollOption{
			Name:  name.String,
			Votes: int(votes.Int64),
		})
		return nil
	})
}

// attachCalls recovers the call history: whether a call was video, how long it
// lasted and how it ended.
func (r *Reader) attachCalls(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if r.schema.has("message_call_log") && r.schema.has("call_log") {
		query := fmt.Sprintf(`
			SELECT message_call_log.message_row_id, %s, %s, %s, %s
			FROM message_call_log
			JOIN call_log ON call_log._id = message_call_log.call_log_row_id
			WHERE message_call_log.message_row_id IN (%s)`,
			r.schema.columnOrNull("call_log", "video_call"),
			r.schema.columnOrNull("call_log", "duration"),
			r.schema.columnOrNull("call_log", "call_result"),
			r.schema.columnOrNull("call_log", "group_jid_row_id"),
			placeholders(len(ids)))

		if err := r.perMessage(ctx, "calls", query, ids, func(rows *sql.Rows) error {
			var id int64
			var video, duration, result, group sql.NullInt64
			if err := rows.Scan(&id, &video, &duration, &result, &group); err != nil {
				return err
			}
			i, ok := index[id]
			if !ok {
				return nil
			}
			page[i].Call = &model.Call{
				Video:    video.Valid && video.Int64 > 0,
				Group:    group.Valid && group.Int64 > 0,
				Duration: time.Duration(duration.Int64) * time.Second,
				Outcome:  callOutcome(result, duration),
			}
			return nil
		}); err != nil {
			return err
		}
	}

	if !r.schema.hasColumn("missed_call_logs", "message_row_id") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, %s, %s FROM missed_call_logs WHERE message_row_id IN (%s)`,
		r.schema.columnOrNull("missed_call_logs", "video_call"),
		r.schema.columnOrNull("missed_call_logs", "group_jid_row_id"), placeholders(len(ids)))

	return r.perMessage(ctx, "missed calls", query, ids, func(rows *sql.Rows) error {
		var id int64
		var video, group sql.NullInt64
		if err := rows.Scan(&id, &video, &group); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || page[i].Call != nil {
			return nil
		}
		page[i].Call = &model.Call{
			Video:   video.Valid && video.Int64 > 0,
			Group:   group.Valid && group.Int64 > 0,
			Outcome: model.CallMissed,
		}
		return nil
	})
}

// callOutcome interprets the result code, falling back to the duration. The codes
// shift between WhatsApp versions, but a call that lasted was answered in any of
// them, so the duration is trusted first.
func callOutcome(result, duration sql.NullInt64) model.CallOutcome {
	if duration.Valid && duration.Int64 > 0 {
		return model.CallConnected
	}
	if !result.Valid {
		return model.CallOutcomeUnknown
	}
	switch result.Int64 {
	case 5:
		return model.CallConnected
	case 2:
		return model.CallMissed
	case 3, 4:
		return model.CallDeclined
	default:
		return model.CallOutcomeUnknown
	}
}

// attachLinks recovers what WhatsApp showed beneath a shared link.
//
// The page may since have changed or disappeared, so this preview is often the
// only surviving record of what was actually shared.
func (r *Reader) attachLinks(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.has("message_text") {
		return nil
	}
	url := r.schema.pick("message_text", "url")
	title := r.schema.pick("message_text", "page_title")
	description := r.schema.pick("message_text", "description")
	if url == "" && title == "" && description == "" {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, %s, %s, %s FROM message_text WHERE message_row_id IN (%s)`,
		r.schema.columnOrNull("message_text", "url"),
		r.schema.columnOrNull("message_text", "page_title"),
		r.schema.columnOrNull("message_text", "description"), placeholders(len(ids)))

	return r.perMessage(ctx, "link previews", query, ids, func(rows *sql.Rows) error {
		var id int64
		var link, title, description sql.NullString
		if err := rows.Scan(&id, &link, &title, &description); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok {
			return nil
		}
		preview := model.LinkPreview{URL: link.String, Title: title.String, Description: description.String}
		if preview.IsEmpty() {
			return nil
		}
		page[i].Link = &preview
		return nil
	})
}

// attachContactCards recovers shared contact details, keeping each card exactly as
// it was sent.
func (r *Reader) attachContactCards(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_vcard", "vcard") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, vcard FROM message_vcard
		 WHERE message_row_id IN (%s) AND vcard IS NOT NULL AND vcard <> '' ORDER BY _id`,
		placeholders(len(ids)))

	if err := r.perMessage(ctx, "contact cards", query, ids, func(rows *sql.Rows) error {
		var id int64
		var card sql.NullString
		if err := rows.Scan(&id, &card); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok {
			return nil
		}
		page[i].Contacts = append(page[i].Contacts, model.ContactCard{
			Name:  nameFromVCard(card.String),
			VCard: card.String,
		})
		return nil
	}); err != nil {
		return err
	}

	// Some cards were matched to a WhatsApp address, which is worth keeping because
	// it links a shared card to a real conversation.
	if !r.schema.hasColumn("message_vcard_jid", "vcard_jid_row_id") {
		return nil
	}
	query = fmt.Sprintf(
		`SELECT message_row_id, vcard_jid_row_id FROM message_vcard_jid WHERE message_row_id IN (%s)`,
		placeholders(len(ids)))

	return r.perMessage(ctx, "contact card addresses", query, ids, func(rows *sql.Rows) error {
		var id, jidRow int64
		if err := rows.Scan(&id, &jidRow); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || len(page[i].Contacts) == 0 {
			return nil
		}
		if j, ok := r.jids[jidRow]; ok {
			// The database does not say which card an address belongs to when several
			// were sent together, so it is recorded against the first.
			page[i].Contacts[0].JIDs = append(page[i].Contacts[0].JIDs, j)
		}
		return nil
	})
}

// nameFromVCard pulls the display name out of a card without parsing the rest.
// The card is kept verbatim; this is only for showing something readable.
func nameFromVCard(card string) string {
	for line := range strings.SplitSeq(card, "\n") {
		name, ok := strings.CutPrefix(strings.TrimSpace(line), "FN:")
		if ok {
			return strings.TrimSpace(name)
		}
	}
	return ""
}

// attachDeletions records that a message was withdrawn, and by whom.
func (r *Reader) attachDeletions(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.has("message_revoked") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, %s, %s FROM message_revoked WHERE message_row_id IN (%s)`,
		r.schema.columnOrNull("message_revoked", "admin_jid_row_id"),
		r.schema.columnOrNull("message_revoked", "revoke_timestamp"), placeholders(len(ids)))

	return r.perMessage(ctx, "deleted messages", query, ids, func(rows *sql.Rows) error {
		var id int64
		var admin, at sql.NullInt64
		if err := rows.Scan(&id, &admin, &at); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok {
			return nil
		}
		deletion := &model.Deletion{At: epochMillis(at)}
		if admin.Valid {
			if j, ok := r.jids[admin.Int64]; ok {
				deletion.By = j
			}
		}
		page[i].Deleted = deletion
		page[i].Kind = model.KindDeleted
		return nil
	})
}

// attachForwardCounts records how widely a message had travelled before it arrived.
func (r *Reader) attachForwardCounts(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_forwarded", "forward_score") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, forward_score FROM message_forwarded WHERE message_row_id IN (%s)`,
		placeholders(len(ids)))

	return r.perMessage(ctx, "forwarded messages", query, ids, func(rows *sql.Rows) error {
		var id int64
		var score sql.NullInt64
		if err := rows.Scan(&id, &score); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok {
			return nil
		}
		page[i].Forwarded = true
		page[i].ForwardScore = int(score.Int64)
		return nil
	})
}

// attachAlbums records how many items were sent together as one album.
func (r *Reader) attachAlbums(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_album", "image_count") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, image_count, %s FROM message_album WHERE message_row_id IN (%s)`,
		r.schema.columnOrNull("message_album", "video_count"), placeholders(len(ids)))

	return r.perMessage(ctx, "albums", query, ids, func(rows *sql.Rows) error {
		var id int64
		var images, videos sql.NullInt64
		if err := rows.Scan(&id, &images, &videos); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok {
			return nil
		}
		page[i].AlbumSize = int(images.Int64 + videos.Int64)
		return nil
	})
}

// attachExpiry marks disappearing messages and how long they were set to last.
func (r *Reader) attachExpiry(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_ephemeral", "duration") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, duration FROM message_ephemeral WHERE message_row_id IN (%s)`,
		placeholders(len(ids)))

	return r.perMessage(ctx, "disappearing messages", query, ids, func(rows *sql.Rows) error {
		var id int64
		var duration sql.NullInt64
		if err := rows.Scan(&id, &duration); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || duration.Int64 <= 0 {
			return nil
		}
		page[i].Expires = time.Duration(duration.Int64) * time.Second
		return nil
	})
}

// attachInvites recovers invitations to groups.
func (r *Reader) attachInvites(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_group_invite", "group_name") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, group_name, %s, %s FROM message_group_invite WHERE message_row_id IN (%s)`,
		r.schema.columnOrNull("message_group_invite", "group_jid_row_id"),
		r.schema.columnOrNull("message_group_invite", "expiration"), placeholders(len(ids)))

	return r.perMessage(ctx, "group invitations", query, ids, func(rows *sql.Rows) error {
		var id int64
		var name sql.NullString
		var group, expiry sql.NullInt64
		if err := rows.Scan(&id, &name, &group, &expiry); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok {
			return nil
		}
		invite := &model.GroupInvite{GroupName: name.String, ExpiresAt: epochMillis(expiry)}
		if group.Valid {
			if j, ok := r.jids[group.Int64]; ok {
				invite.Group = j
			}
		}
		page[i].Invite = invite
		return nil
	})
}

// attachQuotedMedia recovers what a reply was replying to when the answer was a
// picture, so a conversation does not lose half of every exchange about a photo.
func (r *Reader) attachQuotedMedia(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.has("message_quoted_media") {
		return nil
	}
	query := fmt.Sprintf(`
		SELECT message_row_id, %s, %s, %s, %s, %s
		FROM message_quoted_media WHERE message_row_id IN (%s)`,
		r.schema.columnOrNull("message_quoted_media", "mime_type"),
		r.schema.columnOrNull("message_quoted_media", "media_name"),
		r.schema.columnOrNull("message_quoted_media", "media_caption"),
		r.schema.columnOrNull("message_quoted_media", "media_duration"),
		r.schema.columnOrNull("message_quoted_media", "thumbnail"), placeholders(len(ids)))

	return r.perMessage(ctx, "quoted attachments", query, ids, func(rows *sql.Rows) error {
		var (
			id        int64
			mediaType sql.NullString
			name      sql.NullString
			caption   sql.NullString
			duration  sql.NullInt64
			preview   []byte
		)
		if err := rows.Scan(&id, &mediaType, &name, &caption, &duration, &preview); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || page[i].Quote == nil {
			return nil
		}
		page[i].Quote.Attachment = &model.Attachment{
			MediaType: mediaType.String,
			FileName:  name.String,
			Caption:   caption.String,
			Duration:  time.Duration(duration.Int64) * time.Second,
			Preview:   model.Thumbnail{Data: preview},
		}
		if page[i].Quote.Text == "" && caption.Valid {
			page[i].Quote.Text = caption.String
		}
		return nil
	})
}
