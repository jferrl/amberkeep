package guide

import "time"

// What somebody has to be told, in the order they need it.
//
// Every one of these was learned by doing it. The identifiers are stable and are
// never reused: a translation, a screenshot, or somebody quoting one in a support
// conversation all have to keep meaning the same step a year from now.
//
// This is the structure only. The words live in words/*.json beside it, one file per
// language, because they are the most-read prose in the program and the only part of
// it somebody might correct without being able to write Go — which is a reason the
// package comment gave for keeping them out of the code that uses them, and which
// they were in anyway.

var steps = []Step{
	// ---------------------------------------------------------------- before
	{
		ID:       "safety-backup",
		Stage:    Before,
		Critical: true,
		Takes:    30 * time.Minute,
	},
	{
		ID:       "health-data-goes",
		Stage:    Before,
		Critical: true,
	},
	{
		ID:    "encryption-off",
		Stage: Before,
		Check: "backup-not-encrypted",
	},
	{
		ID:       "find-my-off",
		Stage:    Before,
		Critical: false,
	},
	{
		ID:    "fresh-backup",
		Stage: Before,
		Takes: 20 * time.Minute,
		Check: "backup-is-recent",
	},
	{
		ID:    "power-and-space",
		Stage: Before,
		Check: "enough-space",
	},

	// ------------------------------------------------------------- restoring
	{
		ID:    "start-restore",
		Stage: Restoring,
		Takes: 45 * time.Minute,
	},
	{
		ID:    "looks-stalled",
		Stage: Restoring,
		Takes: 25 * time.Minute,
	},

	// ----------------------------------------------------------------- after
	{
		ID:       "decline-icloud",
		Stage:    After,
		Critical: true,
	},
	{
		ID:    "reverify-number",
		Stage: After,
	},
	{
		ID:    "apps-and-passcode",
		Stage: After,
		Takes: time.Hour,
	},
	{
		ID:    "find-my-back-on",
		Stage: After,
	},
	{
		ID:    "check-it-worked",
		Stage: After,
	},

	// ----------------------------------------------------------------- wrong
	{
		// Not marked as losing something if skipped: it is the remedy rather than a
		// precaution, and the whole of this stage is shown together anyway. Marking
		// everything important would leave the mark meaning nothing.
		ID:    "go-back",
		Stage: Wrong,
		Takes: 45 * time.Minute,
	},
	{
		ID:    "error-211",
		Stage: Wrong,
	},
	{
		ID:    "wrong-backup",
		Stage: Wrong,
	},
}
