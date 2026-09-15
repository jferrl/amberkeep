package guide

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

// The words, one file per language.
//
// Embedded rather than read from disk so that a single binary still carries the
// whole guide: the command prints it in a terminal, the server hands it to a page,
// and neither may depend on a file somebody could move. JSON rather than Go because
// these sentences are the most-read prose in the program and the likeliest thing
// anybody will ever want to correct, and correcting them should not require being
// able to write Go.
//
//go:embed words/*.json
var catalogues embed.FS

// Language is which one to say them in.
type Language string

// The languages this is written in. Both are written natively rather than
// translated: a restore instruction that reads like a translation is one somebody
// trusts less at the moment they most need to trust it.
const (
	English Language = "en"
	Spanish Language = "es"
)

// Spoken reports the language to use for a tag that may be anything at all — a
// browser's Accept-Language, an operating system's locale, a flag somebody typed.
// Anything that is not Spanish is English, because English is the one this program
// was written in and a half-understood tag should not produce a half-translated
// guide.
func Spoken(tag string) Language {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(tag)), "es") {
		return Spanish
	}
	return English
}

// words is what one step says.
type words struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Expect string `json:"expect,omitempty"`
}

// catalogue is one language's whole guide.
type catalogue struct {
	// Stages are the headings the four parts are read under.
	Stages map[string]string `json:"stages"`
	// Steps are the words, by the identifier steps.go gives them.
	Steps map[string]words `json:"steps"`
	// Chrome is the renderer's own few phrases, which are as user-facing as the
	// steps and were English literals in the middle of it.
	Chrome map[string]string `json:"chrome"`
	// Advice is what to say about a failure, by the identifier the error carries.
	Advice map[string]Advice `json:"advice"`
}

// said is every language's catalogue, read once.
var said = func() map[Language]catalogue {
	out := map[Language]catalogue{}
	for _, lang := range []Language{English, Spanish} {
		raw, err := catalogues.ReadFile("words/" + string(lang) + ".json")
		if err != nil {
			// Embedded at build time: a missing file is a broken build, not a
			// runtime condition somebody could be in.
			panic(fmt.Sprintf("the %s guide is missing from this build: %v", lang, err))
		}
		var one catalogue
		if err := json.Unmarshal(raw, &one); err != nil {
			panic(fmt.Sprintf("the %s guide will not parse: %v", lang, err))
		}
		out[lang] = one
	}
	return out
}()

// Heading is what a stage is called.
func Heading(stage Stage, lang Language) string {
	if heading, ok := said[lang].Stages[string(stage)]; ok && heading != "" {
		return heading
	}
	return said[English].Stages[string(stage)]
}

// spoken fills a step's words in, falling back to English for anything a language
// has not been written for yet. A step that says nothing would be a step somebody
// skips, which is how the parts that lose data get lost.
func (s Step) spoken(lang Language) Step {
	w, ok := said[lang].Steps[s.ID]
	if !ok || w.Title == "" {
		w = said[English].Steps[s.ID]
	}
	s.Title, s.Body, s.Expect = w.Title, w.Body, w.Expect
	return s
}

// phrase is one of the renderer's own words.
func phrase(name string, lang Language) string {
	if said, ok := said[lang].Chrome[name]; ok && said != "" {
		return said
	}
	return said[English].Chrome[name]
}
