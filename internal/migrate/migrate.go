// Package migrate works out what moving an Android history into an iPhone's
// message store would do, and — separately, and only when asked — does it.
//
// The two halves are separate on purpose. This is the one thing this program does
// that ends with somebody restoring a backup onto a phone they rely on, and the
// Python prototype this is ported from worked out what to do and did it in the same
// pass, which means there was no way to see the answer without also getting it. A
// report nobody can read before the fact is not a safeguard.
//
// So [Plan] opens both sides read-only, decides every question — which Android
// conversation corresponds to which iPhone one, which messages are already there,
// which cannot be carried across — and returns all of it without writing a byte.
// Applying that plan is a second call, against a copy, and a person has to have seen
// the report first.
//
// Four things hold everywhere in this package:
//
//   - The Android database and the iPhone store handed in are opened read-only and
//     are never written to. Everything is written to a copy.
//   - Nothing is ever deleted or overwritten. A conversation that exists on both
//     phones is merged: the messages already on the iPhone are left exactly as they
//     are, and only the ones the iPhone has never seen are added.
//   - A message the iPhone already has is recognised by WhatsApp's own identifier
//     for it, which is stable across devices. That is what makes running this twice
//     harmless.
//   - No device is ever touched. This produces a file; putting it back on a phone is
//     Finder's job, and telling somebody how is the wizard's.
package migrate

import (
	"fmt"
	"time"
)

// Plan is what a migration would do, worked out without doing any of it.
//
// It is the thing a person reads before deciding, so every number in it is one
// somebody could act on: how many messages would arrive, how many are already there,
// and — the two that matter most — what would not come across and why.
type Plan struct {
	// Conversations is every Android conversation considered, including the ones
	// nothing will happen to, because "this chat was skipped and here is why" is the
	// part people go looking for afterwards.
	Conversations []Conversation `json:"conversations"`

	// Adding is how many messages would be written.
	Adding int `json:"adding"`
	// AlreadyThere is how many were found on the iPhone already and will be left
	// alone. On a first run this is usually zero; on a second it is everything.
	AlreadyThere int `json:"already_there"`
	// Untranslatable is how many Android rows carry nothing this can put into an
	// iPhone store: call entries, encryption notices, the housekeeping WhatsApp
	// writes into a conversation.
	Untranslatable int `json:"untranslatable"`

	// AsPlaceholders is how many messages would arrive as a line of text saying what
	// was sent rather than as the picture or recording itself. This is the biggest
	// limitation of the whole feature and it is counted rather than mentioned.
	AsPlaceholders int `json:"as_placeholders"`

	// Merging and Creating split the conversations by what will happen to them.
	Merging  int `json:"merging"`
	Creating int `json:"creating"`

	// Untouched is conversations on the iPhone that this migration does not go near.
	Untouched int `json:"untouched"`

	// Warnings are the things somebody has to read before agreeing, in their own
	// words rather than as codes.
	Warnings []string `json:"warnings,omitempty"`

	// Earliest and Latest bound what would arrive, so a person can recognise their
	// own history in the report rather than trusting a count.
	Earliest time.Time `json:"earliest,omitempty"`
	Latest   time.Time `json:"latest,omitempty"`
}

// total adds the conversations up.
//
// Counted here rather than as the conversations are walked, because one conversation
// can be walked more than once — two source conversations that turn out to be the
// same person on the iPhone are counted into one entry — and adding a running total
// on each pass counts the earlier passes again. It did, and the writer caught it by
// refusing to stand behind a store that did not match the plan.
func (p *Plan) total() {
	p.Adding, p.AlreadyThere, p.Untranslatable, p.AsPlaceholders = 0, 0, 0, 0
	p.Earliest, p.Latest = time.Time{}, time.Time{}

	for _, c := range p.Conversations {
		p.Adding += c.Adding
		p.AlreadyThere += c.AlreadyThere
		p.Untranslatable += c.Untranslatable
		p.AsPlaceholders += c.AsPlaceholders

		if !c.Earliest.IsZero() && (p.Earliest.IsZero() || c.Earliest.Before(p.Earliest)) {
			p.Earliest = c.Earliest
		}
		if c.Latest.After(p.Latest) {
			p.Latest = c.Latest
		}
	}
}

// Empty reports whether the plan would write nothing at all.
func (p Plan) Empty() bool { return p.Adding == 0 }

// Summary is the one line worth saying out loud about a plan.
func (p Plan) Summary() string {
	if p.Empty() {
		return "nothing to add: every message is already on the iPhone"
	}
	return fmt.Sprintf("%s into %s, leaving %s already there untouched",
		plural(p.Adding, "message", "messages"),
		plural(len(p.touched()), "conversation", "conversations"),
		plural(p.AlreadyThere, "message", "messages"))
}

// touched is the conversations something would actually happen to.
func (p Plan) touched() []Conversation {
	out := make([]Conversation, 0, len(p.Conversations))
	for _, c := range p.Conversations {
		if c.Adding > 0 {
			out = append(out, c)
		}
	}
	return out
}

// Conversation is what would happen to one Android chat.
type Conversation struct {
	// Address is the Android conversation's own address.
	Address string `json:"address"`
	// Name is what it is called, for somebody reading the report.
	Name string `json:"name"`
	// Kind is "direct" or "group".
	Kind string `json:"kind"`

	// Destination is the address the iPhone will file this under, which is not always
	// the address the Android archive files it under: one phone can know somebody by
	// their number and the other by a hidden identifier.
	Destination string `json:"destination"`

	// Folded is other Android conversations that turn out to be the same person on
	// the iPhone and are written into this one.
	//
	// Rare and real: on an archive of 4,286 conversations there was one. Left alone
	// it produces the same person twice in the conversation list, which is the exact
	// failure this whole feature is judged on.
	Folded []string `json:"folded,omitempty"`

	// Into is the iPhone conversation this would be merged into, when there is one.
	// Empty means a conversation would be created.
	Into string `json:"into,omitempty"`
	// Session is the iPhone conversation's own identifier, when merging.
	Session int64 `json:"session,omitempty"`

	// Adding, AlreadyThere and Untranslatable account for every message in the
	// Android conversation: the three add up to its total.
	Adding         int `json:"adding"`
	AlreadyThere   int `json:"already_there"`
	Untranslatable int `json:"untranslatable"`

	// AsPlaceholders is how many of Adding would arrive as a line of text rather
	// than as the file that was sent.
	AsPlaceholders int `json:"as_placeholders"`

	// OnPhoneAlready is how many messages the iPhone conversation holds now, which
	// is the number a person will compare against afterwards.
	OnPhoneAlready int `json:"on_phone_already"`

	// Earliest and Latest bound what would arrive from this conversation.
	Earliest time.Time `json:"earliest,omitempty"`
	Latest   time.Time `json:"latest,omitempty"`

	// Skipped says, in a sentence, why nothing would happen to this conversation.
	// It is empty when something would.
	Skipped string `json:"skipped,omitempty"`
}

// Merging reports whether this conversation already exists on the iPhone.
func (c Conversation) Merging() bool { return c.Into != "" }

// plural renders a count with the right noun, which is worth doing because these
// numbers are read by somebody deciding whether to let this touch their phone.
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
