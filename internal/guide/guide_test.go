package guide

import (
	"strings"
	"testing"
	"time"
)

// The guide is data, and what is worth testing about data is that it is consistent
// and that nothing has quietly gone missing. The words themselves are somebody's
// judgement; the shape around them is not.

func TestEveryStepIsUsable(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool, len(Steps()))
	for _, step := range Steps() {
		t.Run(step.ID, func(t *testing.T) {
			if step.ID == "" {
				t.Fatal("a step has no identifier, so nothing can refer to it")
			}
			if seen[step.ID] {
				t.Errorf("%q is used twice; identifiers are how a translation or a "+
					"support conversation refers to one step", step.ID)
			}
			if step.Title == "" || step.Body == "" {
				t.Error("a step with no title or no body says nothing")
			}
			// Written for somebody who is frightened, not for somebody who wrote it.
			if strings.Contains(step.Body, "TODO") || strings.Contains(step.Body, "XXX") {
				t.Error("the body is unfinished")
			}
		})
		seen[step.ID] = true
	}
}

func TestTheStagesAreAllThere(t *testing.T) {
	t.Parallel()

	for _, stage := range []Stage{Before, Restoring, After, Wrong} {
		t.Run(string(stage), func(t *testing.T) {
			t.Parallel()
			if len(At(stage)) == 0 {
				t.Errorf("nothing is said about %s at all", stage)
			}
		})
	}
}

// TestTheThingsThatLoseDataAreMarked is the one piece of content this does assert.
//
// Three steps lose something irreversibly when skipped: the archived safety backup,
// knowing that an unencrypted backup drops Health data and passwords, and declining
// WhatsApp's offer to restore chats from iCloud. If one of them stops being marked,
// it stops being shown differently, and somebody skips it.
func TestTheThingsThatLoseDataAreMarked(t *testing.T) {
	t.Parallel()

	want := map[string]bool{"safety-backup": true, "health-data-goes": true, "decline-icloud": true}
	for _, step := range Critical() {
		delete(want, step.ID)
	}
	for id := range want {
		t.Errorf("%q loses something if it is skipped and is no longer marked as such", id)
	}
}

func TestFindingOneStep(t *testing.T) {
	t.Parallel()

	if _, ok := Find("decline-icloud"); !ok {
		t.Error("the step that undoes the whole migration in one tap cannot be found")
	}
	if _, ok := Find("no-such-step"); ok {
		t.Error("it found a step that does not exist")
	}
}

func TestRenderingAStep(t *testing.T) {
	t.Parallel()

	step, ok := Find("safety-backup")
	if !ok {
		t.Fatal("the step is gone")
	}
	said := step.Render()

	for _, want := range []string{step.Title, "do not skip", "What you will see:", "about 30 minutes"} {
		if !strings.Contains(said, want) {
			t.Errorf("the rendered step does not carry %q:\n%s", want, said)
		}
	}
}

// TestHowLongThingsTakeIsSaidInWords covers the numbers that stop a slow step looking
// like a broken one. A restore that says "2700 seconds" tells nobody anything.
func TestHowLongThingsTakeIsSaidInWords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give time.Duration
		want string
	}{
		{name: "seconds", give: 40 * time.Second, want: "40 seconds"},
		{name: "minutes", give: 25 * time.Minute, want: "25 minutes"},
		{name: "about an hour", give: 70 * time.Minute, want: "an hour"},
		{name: "longer", give: 3 * time.Hour, want: "3 hours"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := plainDuration(tt.give); got != tt.want {
				t.Errorf("plainDuration(%s) = %q, want %q", tt.give, got, tt.want)
			}
		})
	}
}
