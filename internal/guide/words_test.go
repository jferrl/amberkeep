package guide

import (
	"strings"
	"testing"
	"time"
)

// The words, in every language this is written in.
//
// These are the restore instructions. A step that says nothing is a step somebody
// skips, and the ones that lose data if they are skipped are exactly the ones a
// missing translation would quietly blank. So the catalogues are checked against the
// structure rather than against each other's line count.

// languages is everything Steps can be asked for.
var languages = []Language{English, Spanish}

func TestEveryStepIsWrittenInEveryLanguage(t *testing.T) {
	t.Parallel()

	for _, lang := range languages {
		t.Run(string(lang), func(t *testing.T) {
			t.Parallel()

			for _, step := range Steps(lang) {
				if strings.TrimSpace(step.Title) == "" {
					t.Errorf("%s has no title", step.ID)
				}
				if strings.TrimSpace(step.Body) == "" {
					t.Errorf("%s has no body", step.ID)
				}
			}
		})
	}
}

// TestNothingIsLeftInEnglishByAccident covers the failure this refactor exists to
// prevent: a step that falls through to English without anybody noticing, on the
// screen where getting it wrong costs somebody their messages.
func TestNothingIsLeftInEnglishByAccident(t *testing.T) {
	t.Parallel()

	english := map[string]string{}
	for _, step := range Steps(English) {
		english[step.ID] = step.Body
	}

	for _, step := range Steps(Spanish) {
		if step.Body == english[step.ID] {
			t.Errorf("%s reads the same in both languages, so one of them is not written", step.ID)
		}
	}
}

// TestTheStagesAreNamedEverywhere covers the headings, which live in the same
// catalogues and were previously written out twice in Go — once for the page and
// once for the terminal, which is the drift this package was arranged to prevent.
func TestTheStagesAreNamedEverywhere(t *testing.T) {
	t.Parallel()

	for _, lang := range languages {
		for _, stage := range []Stage{Before, Restoring, After, Wrong} {
			said := Heading(stage, lang)
			if said == "" || said == string(stage) {
				t.Errorf("%s has no %s heading; it would show as %q", stage, lang, said)
			}
		}
	}
}

// TestAnUnknownLanguageIsEnglish covers whatever a browser or a locale actually
// sends, which is rarely the two letters anybody expects.
func TestAnUnknownLanguageIsEnglish(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tag  string
		want Language
	}{
		{"es", Spanish},
		{"es-ES", Spanish},
		{"es_ES.UTF-8", Spanish},
		{"ES", Spanish},
		{"  es-419 ", Spanish},
		{"en", English},
		{"en-GB", English},
		{"", English},
		{"de-DE", English},
		{"gibberish", English},
	}
	for _, tt := range tests {
		if got := Spoken(tt.tag); got != tt.want {
			t.Errorf("Spoken(%q) = %q, want %q", tt.tag, got, tt.want)
		}
	}
}

// TestTheCriticalStepsSayWhyInEveryLanguage covers the four that lose something.
// A missing translation anywhere is bad; a missing one here costs somebody their
// messages, and it is worth a test of its own that says so.
func TestTheCriticalStepsSayWhyInEveryLanguage(t *testing.T) {
	t.Parallel()

	for _, lang := range languages {
		critical := Critical(lang)
		if len(critical) == 0 {
			t.Fatalf("%s has no critical steps at all", lang)
		}
		for _, step := range critical {
			// Short enough to be a placeholder is short enough to be wrong.
			if len(strings.Fields(step.Body)) < 20 {
				t.Errorf("%s in %s is %d words, which is too short to explain what it costs",
					step.ID, lang, len(strings.Fields(step.Body)))
			}
		}
	}
}

// TestTheRendererSaysNothingInEnglishToASpanishReader covers the three phrases the
// renderer supplies itself — "do not skip", "what you will see", and how long a step
// takes. They were English literals in the middle of a function that was otherwise
// assembling translated prose, which is the easiest kind of leak to miss: the step
// reads correctly and the frame around it does not.
func TestTheRendererSaysNothingInEnglishToASpanishReader(t *testing.T) {
	t.Parallel()

	leaks := []string{"do not skip", "What you will see", "This takes about", "minutes", "an hour"}

	for _, step := range Steps(Spanish) {
		rendered := step.Render(Spanish)
		for _, leak := range leaks {
			if strings.Contains(rendered, leak) {
				t.Errorf("%s renders %q to a Spanish reader", step.ID, leak)
			}
		}
	}
}

// TestHowLongIsSaidInBothLanguages covers the durations, which are assembled from a
// number and a word and so cannot simply be looked up.
func TestHowLongIsSaidInBothLanguages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		d    time.Duration
		lang Language
		want string
	}{
		{30 * time.Second, English, "30 seconds"},
		{30 * time.Second, Spanish, "30 segundos"},
		{20 * time.Minute, English, "20 minutes"},
		{20 * time.Minute, Spanish, "20 minutos"},
		{90 * time.Minute, English, "an hour"},
		{90 * time.Minute, Spanish, "una hora"},
		{3 * time.Hour, English, "3 hours"},
		{3 * time.Hour, Spanish, "3 horas"},
	}
	for _, tt := range tests {
		if got := plainDuration(tt.d, tt.lang); got != tt.want {
			t.Errorf("plainDuration(%s, %s) = %q, want %q", tt.d, tt.lang, got, tt.want)
		}
	}
}

// TestEveryFailureIsExplainedInEveryLanguage covers the advice, which is the prose a
// person reads at the worst moment they will have with this program. A failure
// explained in English to somebody who has been reading Spanish since the first
// screen is the point where they decide the tool is not for them.
func TestEveryFailureIsExplainedInEveryLanguage(t *testing.T) {
	t.Parallel()

	english := AllAdvice(English)
	if len(english) < 20 {
		t.Fatalf("there are only %d pieces of advice; every failure somebody can act on needs one", len(english))
	}

	for id, told := range english {
		if told.Title == "" || told.Body == "" {
			t.Errorf("%s has no %s", id, map[bool]string{true: "heading", false: "body"}[told.Title == ""])
		}

		spanish, ok := AdviceOn(id, Spanish)
		switch {
		case !ok:
			t.Errorf("%s is not explained in Spanish", id)
		case spanish.Body == told.Body:
			t.Errorf("%s is the English advice in both languages", id)
		case spanish.Title == "":
			t.Errorf("%s has no Spanish heading", id)
		}
	}
}

// TestAdviceNobodyWroteIsAbsentRatherThanEmpty covers the other half: a failure with
// nothing useful to say gets the error's own sentence, and inventing advice for it
// would be worse than saying nothing.
func TestAdviceNobodyWroteIsAbsentRatherThanEmpty(t *testing.T) {
	t.Parallel()

	if told, ok := AdviceOn("nothing.like-this", Spanish); ok {
		t.Errorf("advice was invented for an unknown failure: %q", told.Title)
	}
}
