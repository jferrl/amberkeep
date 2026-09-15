package guide

// Advice is what to tell somebody about a failure they can do something about.
//
// It lives here, beside the migration steps, because it is the same kind of thing:
// prose that had to be learned rather than looked up, that has to read identically
// in a terminal and on a page, and that somebody who is not a programmer should be
// able to correct. It used to be a map of Go string literals in the layer above,
// which made it English by construction — and a failure is the worst place in a
// program to change language on somebody, because it is where they are already
// deciding whether this tool is going to work at all.
//
// Every failure a person can act on carries an identifier from the package that
// raised it. That identifier is the key here. A failure with no entry gets the
// error's own sentence and nothing else, which is honest: there is advice worth
// giving or there is not, and inventing some is worse than silence.
type Advice struct {
	// Title is the one line, read at a glance, instead of the error's own words.
	Title string `json:"title"`
	// Body is the several lines under it, with the line breaks already in them.
	Body string `json:"body"`
}

// AdviceOn returns what to say about the failure with this identifier.
//
// The second return says whether there was anything to say. A caller that ignores
// it shows an empty heading, which is how a screen ends up saying nothing where it
// used to say something.
func AdviceOn(id string, lang Language) (Advice, bool) {
	told, ok := said[lang].Advice[id]
	if !ok && lang != English {
		// A sentence written in English and not yet in the other language is shown
		// in English rather than withheld. Somebody stuck at a failure wants the
		// answer, not a demonstration that the translation is incomplete.
		told, ok = said[English].Advice[id]
	}
	return told, ok
}

// AllAdvice is every entry, for a page that would otherwise ask for them one at a
// time and can only ask after the failure it needs has already happened.
func AllAdvice(lang Language) map[string]Advice {
	spoken, english := said[lang].Advice, said[English].Advice

	all := make(map[string]Advice, len(english))
	for id, told := range english {
		all[id] = told
	}
	for id, told := range spoken {
		all[id] = told
	}
	return all
}
