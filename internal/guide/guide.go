// Package guide holds what somebody has to be told, and when.
//
// Everything in here was learned the hard way, over one weekend, moving a real
// history onto a real phone. None of it is discoverable from Apple's or WhatsApp's
// documentation and most of it is the difference between a migration that works and
// one that appears to work: turn off Find My or the restore fails at the end with a
// number instead of a sentence; archive the safety backup or Finder overwrites it
// with the next one; decline the offer to restore chats from iCloud afterwards or it
// replaces everything that was just merged.
//
// It is data rather than code, and separate from the thing that does the work, for
// three reasons. Somebody who is not a programmer should be able to correct a
// sentence. The same words have to appear in a terminal, in a browser and in a
// printed checklist without three of them drifting apart. And a step that cannot be
// checked mechanically still has to be said, which means the saying and the checking
// are different things and only one of them is a function.
package guide

import (
	"fmt"
	"strings"
	"time"
)

// Stage is where in the whole business a step belongs.
type Stage string

// The stages, in the order somebody passes through them.
const (
	// Before is everything that must be true before anything is written.
	Before Stage = "before"
	// Restoring is what happens on the computer and on the phone, and how long each
	// part takes, so that nothing looks like a failure when it is not.
	Restoring Stage = "restoring"
	// After is what the phone asks for once it comes back, including the one
	// question that must be answered no.
	After Stage = "after"
	// Wrong is what to do when it did not work.
	Wrong Stage = "wrong"
)

// Step is one thing to do or to know.
type Step struct {
	// ID is stable and never reused, so a translation, a screenshot or a support
	// conversation can refer to one step and go on meaning it.
	ID string
	// Stage says when it belongs.
	Stage Stage
	// Title is what the step is, in a few words.
	Title string
	// Body is the several lines that explain it. Line breaks are meaningful and it
	// is rendered as text, never as markup.
	Body string
	// Expect is what somebody will see next, which is the part that stops a slow
	// step looking like a broken one.
	Expect string
	// Takes is roughly how long, when that is worth saying. Zero means it is not.
	Takes time.Duration
	// Critical marks a step that loses data if it is skipped. There are four.
	Critical bool
	// Check names a mechanical check that can confirm this step, when one exists.
	// Most cannot be checked: nothing here can see a phone.
	Check string
}

// Steps is the whole guide, in order.
//
// Written out rather than assembled, because the order is the content: somebody
// reading this is following it, and a step in the wrong place is a step done at the
// wrong time.
func Steps() []Step { return append([]Step(nil), steps...) }

// At returns the steps for one stage.
func At(stage Stage) []Step {
	var out []Step
	for _, s := range steps {
		if s.Stage == stage {
			out = append(out, s)
		}
	}
	return out
}

// Find returns one step by its identifier.
func Find(id string) (Step, bool) {
	for _, s := range steps {
		if s.ID == id {
			return s, true
		}
	}
	return Step{}, false
}

// Critical is the steps that lose something if they are skipped.
func Critical() []Step {
	var out []Step
	for _, s := range steps {
		if s.Critical {
			out = append(out, s)
		}
	}
	return out
}

// Render writes a step the way it is read: the title, the body, what to expect.
func (s Step) Render() string {
	var b strings.Builder
	b.WriteString(s.Title)
	if s.Critical {
		b.WriteString("  [do not skip]")
	}
	b.WriteString("\n\n")
	b.WriteString(s.Body)
	if s.Expect != "" {
		b.WriteString("\n\nWhat you will see: ")
		b.WriteString(s.Expect)
	}
	if s.Takes > 0 {
		fmt.Fprintf(&b, "\n\nThis takes about %s.", plainDuration(s.Takes))
	}
	return b.String()
}

// plainDuration says how long in words somebody would use.
func plainDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	case d < 2*time.Hour:
		return "an hour"
	default:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
}
