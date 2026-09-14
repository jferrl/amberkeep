package migrate

import "errors"

// Guidance identifiers, as the principles require: every failure a person can act on
// maps to an explanation rather than to raw text. Never renumber or reuse one.
const (
	GuidanceNotAStore       = "migrate.not-a-store"
	GuidanceUnfamiliarStore = "migrate.unfamiliar-store"
	GuidanceNothingToDo     = "migrate.nothing-to-do"
	GuidanceWouldOverwrite  = "migrate.would-overwrite"
	GuidanceBrokePromise    = "migrate.broke-promise"
)

var (
	// ErrNotAStore reports a file that is not an iPhone WhatsApp message store.
	ErrNotAStore = errors.New("this is not an iPhone WhatsApp message store")

	// ErrUnfamiliarStore reports a store this build does not know how to write into.
	//
	// Reading one is forgiving, because a column that is missing means a detail that
	// cannot be shown. Writing one is not: a store whose shape has changed is a store
	// where a guess ends up on somebody's phone.
	ErrUnfamiliarStore = errors.New(
		"this iPhone store has a layout this version does not know how to write into")

	// ErrNothingToDo reports a plan that would write nothing. It is not a failure —
	// it is what a second run looks like — but it is not something to act on either,
	// and producing an identical copy of a store would only be confusing.
	ErrNothingToDo = errors.New("there is nothing to add: every message is already on the iPhone")

	// ErrWouldOverwrite reports a destination that already holds something.
	//
	// The thing most likely to be at that path is the result of the last attempt,
	// which somebody may still need — and it is the only copy of a migration that
	// took an hour to produce.
	ErrWouldOverwrite = errors.New("there is already a file there")

	// ErrBrokePromise reports a write that did not match the plan it came from.
	//
	// This should never happen, and that is exactly why it is checked: the plan is
	// what a person read and agreed to, and a store holding something else is a store
	// nobody agreed to, whatever it looks like. It is thrown away rather than
	// reported.
	ErrBrokePromise = errors.New("what was written is not what the plan said would be written")
)
