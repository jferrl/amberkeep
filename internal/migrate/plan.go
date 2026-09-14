package migrate

import (
	"context"
	"fmt"
	"iter"
	"sort"
	"strings"

	"github.com/jferrl/amberkeep/internal/model"
)

// Source is the history being moved. It is the reader this program already has for
// an Android archive, narrowed to what a migration asks of it.
type Source interface {
	Chats(ctx context.Context) ([]model.Chat, error)
	Messages(ctx context.Context, chat model.Chat) iter.Seq2[model.Message, error]
	Directory() *model.Directory
}

// Options say what a migration would include.
type Options struct {
	// Groups includes group conversations. Off by default: a group arrives without
	// its members' own history and is the part most likely to look wrong afterwards,
	// so it is something somebody chooses rather than something that happens.
	Groups bool

	// Hidden includes conversations that exist only behind a hidden identifier with
	// no phone address to resolve it to. Off by default, because such a conversation
	// cannot be matched to anything on the iPhone and would always be created rather
	// than merged — which is how somebody ends up with the same person twice.
	Hidden bool

	// Only, when set, limits the migration to these addresses. This is how somebody
	// tries one conversation before trusting the rest, which is the recommended way
	// to use this at all.
	Only []string
}

// includes reports whether an address was asked for.
func (o Options) includes(address string) bool {
	if len(o.Only) == 0 {
		return true
	}
	for _, want := range o.Only {
		if want == address {
			return true
		}
	}
	return false
}

// Build works out what moving this history onto this phone would do.
//
// It writes nothing. Both sides are read-only and stay that way; what comes back is
// a description a person can read and refuse. Every message in the source is
// accounted for in exactly one of three ways — it would be added, it is already
// there, or it cannot be carried across — so the numbers add up and a missing
// message is a bug rather than a rounding.
func Build(ctx context.Context, from Source, to *Target, opts Options) (Plan, error) {
	chats, err := from.Chats(ctx)
	if err != nil {
		return Plan{}, fmt.Errorf("reading the conversations to move: %w", err)
	}

	names := from.Directory()

	plan := Plan{Conversations: make([]Conversation, 0, len(chats))}
	touchedSessions := make(map[int64]bool)

	// Which destination each conversation claimed, and what it has already counted
	// there. Two source conversations can be one person on the iPhone, and the
	// second must join the first rather than become a second entry in the list.
	claimed := make(map[string]int, len(chats))
	seen := make(map[string]map[string]struct{}, len(chats))

	for _, chat := range chats {
		if err := ctx.Err(); err != nil {
			return Plan{}, err
		}

		c, ok := consider(chat, names, opts)
		if !ok {
			continue
		}
		if c.Skipped != "" {
			plan.Conversations = append(plan.Conversations, c)
			continue
		}

		// Where it would land. A conversation the phone already has is merged into;
		// anything else is created.
		c.Destination = destinationOf(chat, names)
		if session, found := to.Session(c.Destination); found {
			c.Into, c.Session, c.OnPhoneAlready = session.Address, session.PK, session.Messages
		}

		// Somebody the iPhone already has under this address, from an earlier
		// conversation in this same run. Fold rather than create them twice.
		if at, taken := claimed[c.Destination]; taken {
			into := &plan.Conversations[at]
			into.Folded = append(into.Folded, c.Address)
			if err := count(ctx, from, to, chat, into, seen[c.Destination]); err != nil {
				return Plan{}, err
			}
			continue
		}
		seen[c.Destination] = make(map[string]struct{}, 1024)

		if err := count(ctx, from, to, chat, &c, seen[c.Destination]); err != nil {
			return Plan{}, err
		}

		switch {
		case c.Adding == 0 && c.AlreadyThere > 0:
			c.Skipped = "every message is already on the iPhone"
		case c.Adding == 0:
			c.Skipped = "nothing in it can be carried across"
		case c.Merging():
			plan.Merging++
			touchedSessions[c.Session] = true
		default:
			plan.Creating++
		}
		claimed[c.Destination] = len(plan.Conversations)
		plan.Conversations = append(plan.Conversations, c)
	}

	plan.total()
	plan.Untouched = to.Sessions() - len(touchedSessions)
	sortBySize(plan.Conversations)
	plan.Warnings = warningsFor(plan, opts, to)
	return plan, nil
}

// consider decides whether a conversation is in scope at all, and says why when it
// is not. A conversation left out silently is a conversation somebody discovers is
// missing weeks later, on a phone they can no longer compare against.
func consider(chat model.Chat, names *model.Directory, opts Options) (Conversation, bool) {
	c := Conversation{
		Address: chat.JID.String(),
		Name:    chat.Title(),
		Kind:    chat.Kind.String(),
	}
	if !opts.includes(c.Address) {
		return c, false
	}

	switch {
	case chat.Messages == 0:
		c.Skipped = "it is empty"
	case chat.Kind == model.ChatGroup && !opts.Groups:
		c.Skipped = "groups were not included"
	case chat.Kind != model.ChatGroup && chat.Kind != model.ChatDirect:
		c.Skipped = "only conversations and groups can be moved"
	// Resolving a hidden identifier and getting another hidden one back means this
	// archive holds no phone number for that person at all.
	case chat.JID.Server == model.ServerHidden && names.Resolve(chat.JID).Server == model.ServerHidden && !opts.Hidden:
		c.Skipped = "it has only a hidden identity, so it cannot be matched to a conversation on the iPhone"
	}
	return c, true
}

// destinationOf is the address the iPhone will file a conversation under.
//
// A group is filed under its own address on both phones. A person may be known by a
// number on one and by a hidden identifier on the other, and the resolved form is
// what has to be matched against, or the same person arrives twice.
func destinationOf(chat model.Chat, names *model.Directory) string {
	if chat.Kind == model.ChatGroup {
		return chat.JID.String()
	}
	return names.Resolve(chat.JID).String()
}

// count walks one conversation and accounts for every message in it.
//
// seen is the identifiers already counted towards this destination, and belongs to
// the caller because two source conversations can feed one: a message in both must
// be counted once, or the plan promises more than the writing will do and the writing
// is thrown away for breaking a promise it kept.
func count(ctx context.Context, from Source, to *Target, chat model.Chat,
	c *Conversation, seen map[string]struct{},
) error {
	for m, err := range from.Messages(ctx, chat) {
		if err != nil {
			return fmt.Errorf("reading %s: %w", c.Name, err)
		}

		switch {
		case !carriable(m):
			c.Untranslatable++
			continue
		case m.Key != "":
			if _, twice := seen[m.Key]; twice {
				c.AlreadyThere++
				continue
			}
			if c.Session != 0 {
				known, err := to.Knows(ctx, c.Session, m.Key)
				if err != nil {
					return err
				}
				if known {
					c.AlreadyThere++
					continue
				}
			}
			seen[m.Key] = struct{}{}
		}

		c.Adding++
		if placeholder(m) {
			c.AsPlaceholders++
		}
		if !m.SentAt.IsZero() {
			if c.Earliest.IsZero() || m.SentAt.Before(c.Earliest) {
				c.Earliest = m.SentAt
			}
			if m.SentAt.After(c.Latest) {
				c.Latest = m.SentAt
			}
		}
	}
	return nil
}

// carriable reports whether a message has anything an iPhone store can be given.
//
// What cannot be carried is housekeeping rather than conversation: WhatsApp's own
// notices about encryption and group membership, entries in the call history, and
// the rows WhatsApp writes and never displays. Putting those across would mean
// inventing iPhone rows for events that did not happen on the iPhone.
func carriable(m model.Message) bool {
	switch m.Kind {
	case model.KindSystem, model.KindCall, model.KindIgnored:
		return false
	default:
		return m.Displayable()
	}
}

// placeholder reports whether a message would arrive as a line of text describing
// what was sent rather than as the thing itself.
func placeholder(m model.Message) bool {
	return m.Kind != model.KindText && m.Kind != model.KindDeleted
}

// sortBySize puts the conversations somebody cares about first: the ones where the
// most would arrive, then the ones where nothing would.
func sortBySize(conversations []Conversation) {
	sort.SliceStable(conversations, func(i, j int) bool {
		return conversations[i].Adding > conversations[j].Adding
	})
}

// warningsFor is what somebody has to read before agreeing.
func warningsFor(plan Plan, opts Options, to *Target) []string {
	var out []string

	// The one that produces a wrong result which looks entirely right. WhatsApp files
	// some people under a hidden identifier rather than a number, and which of the two
	// it uses can differ between the phones. Its own record of which is which lives in
	// a second file; without that file the two cannot be matched, so the same person
	// arrives a second time — under a different address, so nothing downstream can
	// notice, and nobody finds out until they look at their own conversation list.
	if hidden := to.Hidden(); hidden > 0 && to.Pairings() == 0 {
		out = append(out, fmt.Sprintf(
			"The iPhone files %s under a hidden identity rather than a phone number, and "+
				"WhatsApp's own record of which is which was not supplied. Anybody in that "+
				"position who is known by their number on the Android will arrive as a second "+
				"conversation rather than joining the one already there. Supply LID.sqlite from "+
				"the same backup to avoid it.",
			plural(hidden, "conversation", "conversations")))
	}

	if folded := countFolded(plan); folded > 0 {
		out = append(out, fmt.Sprintf(
			"%s turned out to be the same person the iPhone already has under another name, "+
				"and will be written into the conversation that is already there rather than "+
				"added beside it.", plural(folded, "conversation", "conversations")))
	}

	if plan.AsPlaceholders > 0 {
		out = append(out, fmt.Sprintf(
			"%s will arrive as a line of text saying what was sent, not as the picture, "+
				"recording or file itself. Those files are not in the backup this reads, and "+
				"nothing here can invent them.", plural(plan.AsPlaceholders, "message", "messages")))
	}
	if plan.Untranslatable > 0 {
		out = append(out, fmt.Sprintf(
			"%s will not come across at all: call history, and the notices WhatsApp writes "+
				"into a conversation about itself. Nothing anybody typed is in that number.",
			plural(plan.Untranslatable, "entry", "entries")))
	}
	if hidden := countSkipped(plan, "hidden identity"); hidden > 0 && !opts.Hidden {
		out = append(out, fmt.Sprintf(
			"%s could not be matched to anyone, because WhatsApp hides some people behind an "+
				"identifier this archive has no phone number for. They were left out rather than "+
				"added as somebody new.", plural(hidden, "conversation", "conversations")))
	}
	if groups := countSkipped(plan, "groups were not"); groups > 0 {
		out = append(out, fmt.Sprintf(
			"%s were left out because groups were not included.",
			plural(groups, "group", "groups")))
	}
	return out
}

// countFolded counts the source conversations written into another.
func countFolded(plan Plan) int {
	var n int
	for _, c := range plan.Conversations {
		n += len(c.Folded)
	}
	return n
}

// countSkipped counts the conversations skipped for one reason.
func countSkipped(plan Plan, because string) int {
	var n int
	for _, c := range plan.Conversations {
		if c.Skipped != "" && strings.Contains(c.Skipped, because) {
			n++
		}
	}
	return n
}
