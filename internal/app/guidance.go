package app

import (
	"errors"

	"github.com/jferrl/amberkeep/internal/backupfs"
	"github.com/jferrl/amberkeep/internal/crypt15"
	"github.com/jferrl/amberkeep/internal/export"
	"github.com/jferrl/amberkeep/internal/guide"
	"github.com/jferrl/amberkeep/internal/search"
	"github.com/jferrl/amberkeep/internal/source"
	"github.com/jferrl/amberkeep/internal/source/android"
	"github.com/jferrl/amberkeep/internal/source/ios"
)

// Every failure a person can do something about carries an identifier from the
// package that raised it, and this is where an error becomes one.
//
// The point is that nobody should have to search the internet to get past a
// problem this tool already understands. An error that says "the backup could not
// be decrypted with this key" is accurate; an error that also says what to check,
// in what order, is useful.
//
// What to say lives in internal/guide, in both languages, rather than as Go string
// literals here. This file is the part that has to know about error types; that one
// is the part somebody who is not a programmer can correct.

// GuidanceFor returns the identifier of the advice that goes with a failure, or
// nothing when there is no advice worth giving beyond the message itself.
func GuidanceFor(err error) string {
	var decryption *crypt15.Error
	if errors.As(err, &decryption) {
		return decryption.Guidance
	}

	var reading *android.Error
	if errors.As(err, &reading) {
		return reading.Guidance
	}

	var searching *search.Error
	if errors.As(err, &searching) {
		return searching.Guidance
	}

	var backup *backupfs.Error
	if errors.As(err, &backup) {
		return backup.Guidance
	}

	var iphone *ios.Error
	if errors.As(err, &iphone) {
		return iphone.Guidance
	}

	var unrecognised *source.Error
	if errors.As(err, &unrecognised) {
		return unrecognised.Guidance
	}

	if errors.Is(err, export.ErrExists) {
		return export.GuidanceExists
	}
	return ""
}

// AdviseOn returns what to tell somebody about a failure, in their own language,
// and whether there was anything to tell them.
func AdviseOn(err error, lang guide.Language) (guide.Advice, bool) {
	id := GuidanceFor(err)
	if id == "" {
		return guide.Advice{}, false
	}
	return guide.AdviceOn(id, lang)
}
