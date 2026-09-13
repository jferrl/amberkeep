package ios

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/jferrl/amberkeep/internal/model"
)

// Chats lists every conversation in the store, most recently used first.
//
// The list is read once and kept: it is a few hundred kilobytes even for a large
// archive, every message names its conversation by row rather than by address, and
// rebuilding it per request would dominate the cost of reading anything.
func (r *Reader) Chats(ctx context.Context) ([]model.Chat, error) {
	if r.sessions != nil {
		return r.sorted(), nil
	}

	counts, err := r.messageCounts(ctx)
	if err != nil {
		return nil, err
	}
	created, err := r.groupCreation(ctx)
	if err != nil {
		return nil, err
	}

	archived := r.schema.columnOrNull(tableSession, "ZARCHIVED")
	lastAt := r.schema.columnOrNull(tableSession, "ZLASTMESSAGEDATE")
	partner := r.schema.columnOrNull(tableSession, "ZPARTNERNAME")

	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT Z_PK, ZCONTACTJID, %s, %s, %s FROM %s`,
		partner, lastAt, archived, tableSession))
	if err != nil {
		return nil, ErrUnreadable.withCause(fmt.Errorf("reading conversations: %w", err))
	}
	defer rows.Close()

	sessions := make(map[int64]model.Chat, len(counts)+16)
	for rows.Next() {
		var (
			pk       int64
			address  sql.NullString
			name     sql.NullString
			last     sql.NullFloat64
			archived sql.NullBool
		)
		if err := rows.Scan(&pk, &address, &name, &last, &archived); err != nil {
			return nil, ErrUnreadable.withCause(fmt.Errorf("reading a conversation: %w", err))
		}

		jid := model.ParseJID(address.String)
		chat := model.Chat{
			ID:        pk,
			JID:       jid,
			Kind:      model.ChatKindOf(jid),
			Name:      name.String,
			LastAt:    coreDataTime(last),
			CreatedAt: created[pk],
			Archived:  archived.Bool,
			Messages:  counts[pk],
		}

		// The name the phone's address book gave this person is the best there is,
		// and on an iPhone it is right here rather than in a separate export. A
		// group's subject is not a person's name and belongs to the conversation
		// alone.
		if chat.Name != "" && chat.Kind == model.ChatDirect && !jid.IsZero() {
			r.directory.Add(model.Contact{JID: jid, Name: chat.Name})
		}
		sessions[pk] = chat
	}
	if err := rows.Err(); err != nil {
		return nil, ErrUnreadable.withCause(fmt.Errorf("reading conversations: %w", err))
	}

	r.sessions = sessions
	if err := r.loadParticipants(ctx); err != nil {
		return nil, err
	}
	return r.sorted(), nil
}

// sorted returns the conversations in the order somebody looks for one.
func (r *Reader) sorted() []model.Chat {
	out := make([]model.Chat, 0, len(r.sessions))
	for _, chat := range r.sessions {
		out = append(out, chat)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].LastAt.Equal(out[j].LastAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].LastAt.After(out[j].LastAt)
	})
	return out
}

// messageCounts counts each conversation in one pass.
//
// The store keeps its own counter in ZMESSAGECOUNTER and it is wrong: on a real
// iPhone it disagreed with the true count for 551 of 554 conversations, always by
// one. Counting is a single grouped scan and is worth it to be right.
func (r *Reader) messageCounts(ctx context.Context) (map[int64]int, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT ZCHATSESSION, count(*) FROM `+tableMessage+`
		 WHERE ZCHATSESSION IS NOT NULL GROUP BY ZCHATSESSION`)
	if err != nil {
		return nil, ErrUnreadable.withCause(fmt.Errorf("counting messages: %w", err))
	}
	defer rows.Close()

	counts := make(map[int64]int, 512)
	for rows.Next() {
		var session, n int64
		if err := rows.Scan(&session, &n); err != nil {
			return nil, ErrUnreadable.withCause(fmt.Errorf("counting messages: %w", err))
		}
		counts[session] = int(n)
	}
	if err := rows.Err(); err != nil {
		return nil, ErrUnreadable.withCause(fmt.Errorf("counting messages: %w", err))
	}
	return counts, nil
}

// groupCreation reads when each group was made, which is the only creation date an
// iPhone store records.
func (r *Reader) groupCreation(ctx context.Context) (map[int64]time.Time, error) {
	created := make(map[int64]time.Time)
	if !r.schema.hasColumn(tableGroup, "ZCREATIONDATE") {
		return created, nil
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT ZCHATSESSION, ZCREATIONDATE FROM `+tableGroup+` WHERE ZCHATSESSION IS NOT NULL`)
	if err != nil {
		return nil, ErrUnreadable.withCause(fmt.Errorf("reading group details: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var (
			session int64
			at      sql.NullFloat64
		)
		if err := rows.Scan(&session, &at); err != nil {
			return nil, ErrUnreadable.withCause(fmt.Errorf("reading group details: %w", err))
		}
		created[session] = coreDataTime(at)
	}
	if err := rows.Err(); err != nil {
		return nil, ErrUnreadable.withCause(fmt.Errorf("reading group details: %w", err))
	}
	return created, nil
}

// loadParticipants attaches each group's members to its conversation.
func (r *Reader) loadParticipants(ctx context.Context) error {
	if !r.schema.hasColumn(tableMember, "ZCHATSESSION") {
		return nil
	}

	admin := r.schema.columnOrNull(tableMember, "ZISADMIN")
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT ZCHATSESSION, Z_PK, %s FROM %s WHERE ZCHATSESSION IS NOT NULL ORDER BY Z_PK`,
		admin, tableMember))
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading group members: %w", err))
	}
	defer rows.Close()

	for rows.Next() {
		var (
			session, member int64
			isAdmin         sql.NullBool
		)
		if err := rows.Scan(&session, &member, &isAdmin); err != nil {
			return ErrUnreadable.withCause(fmt.Errorf("reading a group member: %w", err))
		}
		chat, known := r.sessions[session]
		if !known {
			continue
		}
		jid, found := r.members[member]
		if !found {
			continue
		}
		chat.Participants = append(chat.Participants, model.Participant{
			JID:   jid,
			Name:  r.directory.NameOf(jid),
			Admin: isAdmin.Bool,
		})
		r.sessions[session] = chat
	}
	if err := rows.Err(); err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("reading group members: %w", err))
	}
	return nil
}
