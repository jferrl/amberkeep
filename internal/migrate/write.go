package migrate

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jferrl/amberkeep/internal/export"
	"github.com/jferrl/amberkeep/internal/model"
)

// everything writes every conversation the plan said something would happen to.
//
// Driven by the plan rather than by the archive, because the plan is what somebody
// agreed to and because one entry in it can draw on more than one conversation in the
// archive — two chats that turn out to be the same person on the iPhone.
func (w *writer) everything(ctx context.Context, from Source, plan Plan) (Result, error) {
	var result Result

	chats, err := from.Chats(ctx)
	if err != nil {
		return result, fmt.Errorf("reading the conversations to move: %w", err)
	}
	byAddress := make(map[string]model.Chat, len(chats))
	for _, chat := range chats {
		byAddress[chat.JID.String()] = chat
	}

	for _, planned := range plan.Conversations {
		if planned.Adding == 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}

		added, err := w.conversation(ctx, from, byAddress, planned)
		if err != nil {
			return result, err
		}
		result.Added += added
		if planned.Merging() {
			result.Merged++
		} else {
			result.Created++
		}
	}
	return result, nil
}

// conversation writes everything destined for one iPhone conversation, then puts it
// back in order.
func (w *writer) conversation(ctx context.Context, from Source,
	byAddress map[string]model.Chat, planned Conversation,
) (int, error) {
	chat, ok := byAddress[planned.Address]
	if !ok {
		return 0, fmt.Errorf("%w: the archive no longer holds %s", ErrBrokePromise, planned.Address)
	}

	session := planned.Session
	if !planned.Merging() {
		var err error
		if session, err = w.newSession(ctx, chat, planned); err != nil {
			return 0, err
		}
	}

	// Read once for the whole conversation, and shared across everything written
	// into it: a message present in two source conversations goes in once.
	known, err := w.identifiers(ctx, session)
	if err != nil {
		return 0, err
	}

	var added int
	for _, address := range append([]string{planned.Address}, planned.Folded...) {
		source, held := byAddress[address]
		if !held {
			return 0, fmt.Errorf("%w: the archive no longer holds %s", ErrBrokePromise, address)
		}
		n, err := w.messagesFrom(ctx, from, source, session, planned.Destination, known)
		if err != nil {
			return 0, err
		}
		added += n
	}

	if added > 0 {
		if err := w.reorder(ctx, session); err != nil {
			return 0, err
		}
		if err := w.point(ctx, session); err != nil {
			return 0, err
		}
	}
	return added, nil
}

// messagesFrom writes one source conversation into a session.
func (w *writer) messagesFrom(ctx context.Context, from Source, chat model.Chat,
	session int64, destination string, known map[string]struct{},
) (int, error) {
	group := chat.Kind == model.ChatGroup

	var added int
	for m, err := range from.Messages(ctx, chat) {
		if err != nil {
			return 0, fmt.Errorf("reading %s: %w", chat.Title(), err)
		}
		if !carriable(m) {
			continue
		}
		if m.Key != "" {
			if _, seen := known[m.Key]; seen {
				continue
			}
			known[m.Key] = struct{}{}
		}

		if err := w.message(ctx, m, session, destination, group); err != nil {
			return 0, err
		}
		added++
	}
	return added, nil
}

// identifiers is what a conversation already holds, so nothing is written twice.
//
// Read here as well as in the plan, rather than carried over from it. The plan is a
// description; this is the thing that must not write a duplicate, and it should not
// depend on a number somebody could have edited in between.
func (w *writer) identifiers(ctx context.Context, session int64) (map[string]struct{}, error) {
	rows, err := w.tx.QueryContext(ctx,
		"SELECT ZSTANZAID FROM ZWAMESSAGE WHERE ZCHATSESSION = :s AND ZSTANZAID IS NOT NULL",
		sql.Named("s", session))
	if err != nil {
		return nil, fmt.Errorf("reading what the conversation already holds: %w", err)
	}
	defer func() { _ = rows.Close() }()

	known := make(map[string]struct{}, 1024)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("reading what the conversation already holds: %w", err)
		}
		known[id] = struct{}{}
	}
	return known, rows.Err()
}

// newSession creates a conversation the phone has never had.
func (w *writer) newSession(ctx context.Context, chat model.Chat, planned Conversation) (int64, error) {
	session := w.next("WAChatSession")
	group := chat.Kind == model.ChatGroup

	var groupInfo any
	if group {
		if _, ok := w.entities["WAGroupInfo"]; ok {
			pk := w.next("WAGroupInfo")
			if err := w.insert(ctx, "ZWAGROUPINFO", map[string]any{
				"Z_PK": pk, "Z_ENT": w.entityOf["WAGroupInfo"], "Z_OPT": 1,
				"ZSTATE": 0, "ZCHATSESSION": session, "ZCREATIONDATE": toCoreData(chat.CreatedAt),
			}); err != nil {
				return 0, err
			}
			groupInfo = pk
		}
	}

	kind := int64(0)
	spotlight := w.conv.spotlightDirect
	if group {
		kind, spotlight = sessionGroup, w.conv.spotlightGrouped
	}

	if err := w.insert(ctx, "ZWACHATSESSION", map[string]any{
		"Z_PK": session, "Z_ENT": w.entityOf["WAChatSession"], "Z_OPT": 1,
		"ZARCHIVED": boolAsInt(chat.Archived), "ZCONTACTABID": 0,
		"ZFLAGS": w.conv.sessionFlags, "ZHIDDEN": 0,
		"ZIDENTITYVERIFICATIONEPOCH": 0, "ZIDENTITYVERIFICATIONSTATE": 0,
		"ZMESSAGECOUNTER": 0, "ZREMOVED": 0, "ZSESSIONTYPE": kind,
		"ZSPOTLIGHTSTATUS": spotlight, "ZUNREADCOUNT": 0,
		"ZGROUPINFO": groupInfo, "ZCONTACTJID": planned.Destination,
		"ZPARTNERNAME": chat.Title(),
	}); err != nil {
		return 0, err
	}
	return session, nil
}

// message writes one message into a conversation.
func (w *writer) message(ctx context.Context, m model.Message, session int64, address string, group bool) error {
	at := toCoreData(m.SentAt)
	fromMe := m.IsFromMe()

	// The two addresses are kept as any so that a column can be written NULL, which
	// is what the convention requires: an outgoing message says who it went to and
	// nothing about where it came from, and an incoming one the other way round.
	var from, to, pushName any
	var sender string
	if fromMe {
		to = address
	} else {
		sender = address
		if !m.Sender.IsZero() {
			sender = w.names.Resolve(m.Sender).String()
		}
		from = sender
		if m.PushName != "" {
			pushName = m.PushName
		}
	}

	var member any
	if group && !fromMe && sender != "" {
		pk, err := w.member(ctx, session, sender, m.PushName)
		if err != nil {
			return err
		}
		member = pk
	}

	flags, status := w.conv.incomingFlags, w.conv.incomingStatus
	if fromMe {
		flags, status = w.conv.outgoingFlags, w.conv.outgoingStatus
	}

	return w.insert(ctx, "ZWAMESSAGE", map[string]any{
		"Z_PK": w.next("WAMessage"), "Z_ENT": w.entityOf["WAMessage"], "Z_OPT": 1,
		"ZCHILDMESSAGESDELIVEREDCOUNT": 0, "ZCHILDMESSAGESPLAYEDCOUNT": 0,
		"ZCHILDMESSAGESREADCOUNT": 0, "ZDATAITEMVERSION": 3, "ZDOCID": 0,
		"ZENCRETRYCOUNT": 0, "ZFILTEREDRECIPIENTCOUNT": 0,
		"ZFLAGS": flags, "ZGROUPEVENTTYPE": 0, "ZISFROMME": boolAsInt(fromMe),
		"ZMESSAGEERRORSTATUS": 0, "ZMESSAGESTATUS": status, "ZMESSAGETYPE": textMessage,
		"ZSORT": 0, "ZSPOTLIGHTSTATUS": w.conv.spotlight, "ZSTARRED": boolAsInt(m.Starred),
		"ZCHATSESSION": session, "ZGROUPMEMBER": member,
		"ZMESSAGEDATE": at, "ZSENTDATE": at,
		"ZFROMJID": from, "ZTOJID": to, "ZPUSHNAME": pushName,
		"ZSTANZAID": nilIfEmpty(m.Key), "ZTEXT": w.text(m),
	})
}

// text is what a message becomes on the phone.
//
// Never empty: a message with nothing to show is a blank line in somebody's history,
// and the checks refuse a store containing one. Where a reader recovered nothing at
// all, the kind of thing it was is still worth more than silence.
func (w *writer) text(m model.Message) string {
	if line := export.Line(m, w.saying); line != "" {
		return line
	}
	return "<" + m.Kind.String() + ">"
}

// member finds or creates somebody's entry in a group.
func (w *writer) member(ctx context.Context, session int64, address, pushName string) (int64, error) {
	key := fmt.Sprintf("%d:%s", session, address)
	if pk, ok := w.known[key]; ok {
		return pk, nil
	}

	var existing sql.NullInt64
	if err := w.tx.QueryRowContext(ctx,
		"SELECT Z_PK FROM ZWAGROUPMEMBER WHERE ZCHATSESSION = :s AND ZMEMBERJID = :j",
		sql.Named("s", session), sql.Named("j", address)).Scan(&existing); err == nil && existing.Valid {
		w.known[key] = existing.Int64
		return existing.Int64, nil
	}

	name := pushName
	if w.names != nil {
		if known := w.names.NameOf(model.ParseJID(address)); known != "" {
			name = known
		}
	}

	pk := w.next("WAGroupMember")
	if err := w.insert(ctx, "ZWAGROUPMEMBER", map[string]any{
		"Z_PK": pk, "Z_ENT": w.entityOf["WAGroupMember"], "Z_OPT": 1,
		"ZCONTACTABID": 0, "ZISACTIVE": 1, "ZISADMIN": 0,
		"ZCHATSESSION": session, "ZCONTACTNAME": name, "ZMEMBERJID": address,
	}); err != nil {
		return 0, err
	}
	w.known[key] = pk
	return pk, nil
}

// reorder renumbers a whole conversation by date.
//
// ZSORT is what puts a conversation in order on the phone, and it has to be 1..N with
// no gaps. Messages arriving from another phone land among the ones already there
// rather than after them, so the whole conversation is renumbered rather than the new
// messages appended — which is also why an existing conversation's ZSORT is the one
// column a merge is allowed to change.
//
// One statement rather than a row at a time: a real conversation runs to ninety
// thousand messages.
func (w *writer) reorder(ctx context.Context, session int64) error {
	_, err := w.tx.ExecContext(ctx, `
		UPDATE ZWAMESSAGE SET ZSORT = ordered.n
		FROM (SELECT Z_PK, row_number() OVER (ORDER BY ZMESSAGEDATE, Z_PK) AS n
		      FROM ZWAMESSAGE WHERE ZCHATSESSION = :s) AS ordered
		WHERE ZWAMESSAGE.Z_PK = ordered.Z_PK`, sql.Named("s", session))
	if err != nil {
		return fmt.Errorf("putting the conversation back in order: %w", err)
	}
	return nil
}

// point makes a conversation agree with itself about where it ends.
//
// The phone reads the conversation list from these, not from the messages, so a
// conversation whose pointers are stale shows the wrong last line, or none.
func (w *writer) point(ctx context.Context, session int64) error {
	var (
		last  sql.NullInt64
		at    sql.NullFloat64
		text  sql.NullString
		total int64
	)
	if err := w.tx.QueryRowContext(ctx, `
		SELECT (SELECT Z_PK FROM ZWAMESSAGE WHERE ZCHATSESSION = :s ORDER BY ZSORT DESC LIMIT 1),
		       (SELECT ZMESSAGEDATE FROM ZWAMESSAGE WHERE ZCHATSESSION = :s ORDER BY ZSORT DESC LIMIT 1),
		       (SELECT ZTEXT FROM ZWAMESSAGE WHERE ZCHATSESSION = :s ORDER BY ZSORT DESC LIMIT 1),
		       (SELECT count(*) FROM ZWAMESSAGE WHERE ZCHATSESSION = :s)`,
		sql.Named("s", session)).Scan(&last, &at, &text, &total); err != nil {
		return fmt.Errorf("finding where the conversation ends: %w", err)
	}
	if !last.Valid {
		return nil
	}

	// The preview the list shows is a short one. Cut by runes rather than bytes: a
	// history in Spanish is full of accents, and half a character is not a character.
	preview := any(nil)
	if text.Valid {
		preview = shorten(text.String, 200)
	}

	if _, err := w.tx.ExecContext(ctx, `
		UPDATE ZWACHATSESSION
		SET ZMESSAGECOUNTER = max(:counter, coalesce(ZMESSAGECOUNTER, 0)),
		    ZLASTMESSAGE = :last, ZLASTMESSAGEDATE = :at, ZLASTMESSAGETEXT = :text
		WHERE Z_PK = :s`,
		sql.Named("counter", total+1), sql.Named("last", last.Int64),
		sql.Named("at", at.Float64), sql.Named("text", preview),
		sql.Named("s", session)); err != nil {
		return fmt.Errorf("pointing the conversation at its own end: %w", err)
	}

	// The last message of a conversation carries the conversation back, and only
	// that one does. A merge moves the end, so whatever held it lets go.
	if _, err := w.tx.ExecContext(ctx,
		"UPDATE ZWAMESSAGE SET ZLASTSESSION = NULL WHERE ZCHATSESSION = :s AND ZLASTSESSION = :s AND Z_PK != :last",
		sql.Named("s", session), sql.Named("last", last.Int64)); err != nil {
		return fmt.Errorf("pointing the conversation at its own end: %w", err)
	}
	if _, err := w.tx.ExecContext(ctx,
		"UPDATE ZWAMESSAGE SET ZLASTSESSION = :s WHERE Z_PK = :last",
		sql.Named("s", session), sql.Named("last", last.Int64)); err != nil {
		return fmt.Errorf("pointing the conversation at its own end: %w", err)
	}
	return nil
}

// shorten cuts text to a number of characters, never through one.
func shorten(s string, runes int) string {
	r := []rune(s)
	if len(r) <= runes {
		return s
	}
	return string(r[:runes])
}

func boolAsInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
