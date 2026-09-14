package migrate

import "errors"

// Guidance identifiers, as the principles require: every failure a person can act on
// maps to an explanation rather than to raw text. Never renumber or reuse one.
const (
	GuidanceNotAStore       = "migrate.not-a-store"
	GuidanceUnfamiliarStore = "migrate.unfamiliar-store"
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
)
