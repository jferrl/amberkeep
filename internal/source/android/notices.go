package android

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jferrl/amberkeep/internal/model"
)

// System notices are roughly two per cent of a real archive and almost none of
// them carry text of their own: in a database of 1.1 million messages, 28,361 of
// 28,909 notices were blank. What happened is recorded as a numeric action code,
// with the particulars spread across a dozen small tables.
//
// This file gathers those particulars. Turning them into a sentence is a separate
// job, done in phrasing.go, because the sentence has to be translated and the facts
// do not.

// attachSystemNotices recovers what each notice was about.
func (r *Reader) attachSystemNotices(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_system", "action_type") {
		return nil
	}

	// Which notices are on this page, so the detail queries below can skip work
	// entirely for a page that holds none.
	query := fmt.Sprintf(
		`SELECT message_row_id, action_type FROM message_system WHERE message_row_id IN (%s)`,
		placeholders(len(ids)))

	var notices []int64
	if err := r.perMessage(ctx, "system notices", query, ids, func(rows *sql.Rows) error {
		var id int64
		var action sql.NullInt64
		if err := rows.Scan(&id, &action); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok {
			return nil
		}
		page[i].Kind = model.KindSystem
		page[i].Notice = &model.Notice{Action: int(action.Int64)}
		notices = append(notices, id)
		return nil
	}); err != nil {
		return err
	}
	if len(notices) == 0 {
		return nil
	}

	for _, load := range []func(context.Context, []int64, map[int64]int, []model.Message) error{
		r.noticeParticipants,
		r.noticeGroupJoin,
		r.noticeValueChange,
		r.noticeNumberChange,
		r.noticeDeviceChange,
		r.noticeBusinessState,
		r.noticeUsernameChange,
		r.noticeBlockContact,
		r.noticeGroupNodes,
	} {
		if err := load(ctx, notices, index, page); err != nil {
			return err
		}
	}

	// The sender of a notice is the person who caused it, which is exactly the
	// actor every phrasing needs.
	for _, id := range notices {
		i, ok := index[id]
		if !ok || page[i].Notice == nil {
			continue
		}
		page[i].Notice.Actor = page[i].Sender
		page[i].SystemText = phraseNotice(*page[i].Notice, r.directory)
	}
	return nil
}

// noticeParticipants recovers who a notice was about: the people added, removed,
// invited or promoted.
func (r *Reader) noticeParticipants(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_system_chat_participant", "user_jid_row_id") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, user_jid_row_id FROM message_system_chat_participant
		 WHERE message_row_id IN (%s)`, placeholders(len(ids)))

	return r.perMessage(ctx, "notice participants", query, ids, func(rows *sql.Rows) error {
		var id, jidRow int64
		if err := rows.Scan(&id, &jidRow); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || page[i].Notice == nil {
			return nil
		}
		if j, ok := r.jids[jidRow]; ok {
			page[i].Notice.Targets = append(page[i].Notice.Targets, j)
		}
		return nil
	})
}

// noticeGroupJoin distinguishes a notice about the archive's owner from one about
// somebody else, which changes the sentence from "they joined" to "you joined".
func (r *Reader) noticeGroupJoin(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_system_group", "is_me_joined") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, is_me_joined FROM message_system_group WHERE message_row_id IN (%s)`,
		placeholders(len(ids)))

	return r.perMessage(ctx, "group notices", query, ids, func(rows *sql.Rows) error {
		var id int64
		var joined sql.NullInt64
		if err := rows.Scan(&id, &joined); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || page[i].Notice == nil {
			return nil
		}
		page[i].Notice.Joined = joined.Valid && joined.Int64 > 0
		return nil
	})
}

// noticeValueChange recovers what a group's subject or description used to be.
func (r *Reader) noticeValueChange(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_system_value_change", "old_data") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, old_data FROM message_system_value_change WHERE message_row_id IN (%s)`,
		placeholders(len(ids)))

	return r.perMessage(ctx, "changed values", query, ids, func(rows *sql.Rows) error {
		var id int64
		var old sql.NullString
		if err := rows.Scan(&id, &old); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || page[i].Notice == nil {
			return nil
		}
		page[i].Notice.Old = old.String
		// The new value is the message's own text on the versions that record it.
		if page[i].Notice.New == "" {
			page[i].Notice.New = page[i].Text
		}
		return nil
	})
}

// noticeNumberChange recovers a contact's old and new phone numbers. Without it, a
// conversation appears to change person partway through.
func (r *Reader) noticeNumberChange(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_system_number_change", "old_jid_row_id") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, old_jid_row_id, %s FROM message_system_number_change
		 WHERE message_row_id IN (%s)`,
		r.schema.columnOrNull("message_system_number_change", "new_jid_row_id"), placeholders(len(ids)))

	return r.perMessage(ctx, "number changes", query, ids, func(rows *sql.Rows) error {
		var id int64
		var oldRow, newRow sql.NullInt64
		if err := rows.Scan(&id, &oldRow, &newRow); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || page[i].Notice == nil {
			return nil
		}
		if oldRow.Valid {
			if j, ok := r.jids[oldRow.Int64]; ok {
				page[i].Notice.Old = r.directory.NameOf(j)
			}
		}
		if newRow.Valid {
			if j, ok := r.jids[newRow.Int64]; ok {
				page[i].Notice.New = r.directory.NameOf(j)
			}
		}
		return nil
	})
}

// noticeDeviceChange recovers how many linked devices a security notice reported.
func (r *Reader) noticeDeviceChange(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_system_device_change", "device_added_count") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, device_added_count, %s FROM message_system_device_change
		 WHERE message_row_id IN (%s)`,
		r.schema.columnOrNull("message_system_device_change", "device_removed_count"), placeholders(len(ids)))

	return r.perMessage(ctx, "device changes", query, ids, func(rows *sql.Rows) error {
		var id int64
		var added, removed sql.NullInt64
		if err := rows.Scan(&id, &added, &removed); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || page[i].Notice == nil {
			return nil
		}
		page[i].Notice.DevicesAdded = int(added.Int64)
		page[i].Notice.DevicesRemoved = int(removed.Int64)
		return nil
	})
}

// noticeBusinessState recovers the verified name behind a business notice.
func (r *Reader) noticeBusinessState(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_system_business_state", "business_name") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, business_name FROM message_system_business_state
		 WHERE message_row_id IN (%s)`, placeholders(len(ids)))

	return r.perMessage(ctx, "business notices", query, ids, func(rows *sql.Rows) error {
		var id int64
		var name sql.NullString
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || page[i].Notice == nil {
			return nil
		}
		page[i].Notice.Business = name.String
		return nil
	})
}

// noticeUsernameChange recovers a username that changed.
func (r *Reader) noticeUsernameChange(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_system_username_change", "new_username") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, %s, new_username FROM message_system_username_change
		 WHERE message_row_id IN (%s)`,
		r.schema.columnOrNull("message_system_username_change", "old_username"), placeholders(len(ids)))

	return r.perMessage(ctx, "username changes", query, ids, func(rows *sql.Rows) error {
		var id int64
		var old, updated sql.NullString
		if err := rows.Scan(&id, &old, &updated); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || page[i].Notice == nil {
			return nil
		}
		page[i].Notice.Old = old.String
		page[i].Notice.New = updated.String
		return nil
	})
}

// noticeBlockContact recovers whether a contact was blocked or unblocked.
func (r *Reader) noticeBlockContact(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_system_block_contact", "is_blocked") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, is_blocked FROM message_system_block_contact WHERE message_row_id IN (%s)`,
		placeholders(len(ids)))

	return r.perMessage(ctx, "blocked contacts", query, ids, func(rows *sql.Rows) error {
		var id int64
		var blocked sql.NullInt64
		if err := rows.Scan(&id, &blocked); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || page[i].Notice == nil {
			return nil
		}
		page[i].Notice.Blocked = blocked.Valid && blocked.Int64 > 0
		return nil
	})
}

// noticeGroupNodes recovers the group subject a community notice referred to.
func (r *Reader) noticeGroupNodes(ctx context.Context, ids []int64, index map[int64]int, page []model.Message) error {
	if !r.schema.hasColumn("message_system_with_group_nodes", "group_subject") {
		return nil
	}
	query := fmt.Sprintf(
		`SELECT message_row_id, group_subject FROM message_system_with_group_nodes
		 WHERE message_row_id IN (%s)`, placeholders(len(ids)))

	return r.perMessage(ctx, "community notices", query, ids, func(rows *sql.Rows) error {
		var id int64
		var subject sql.NullString
		if err := rows.Scan(&id, &subject); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok || page[i].Notice == nil {
			return nil
		}
		page[i].Notice.Subject = subject.String
		return nil
	})
}
