package api

import "github.com/jferrl/amberkeep/internal/guide"

// Count is a number the work has reached, for a page to phrase itself.
//
// The sentence used to be built here and sent as prose, which made every progress
// line English — and unformatted, so a Spanish reader watching an index build was
// told about "595236 messages" rather than 595.236. The server knows the number and
// the page knows the reader, so the number travels and the sentence is composed
// where the language is.
type Count struct {
	// Of is what was counted: "conversations", "messages".
	Of string `json:"of"`
	N  int    `json:"n"`
}

// Note is a sentence the work wants said.
//
// Named rather than only written out, because the terminal and the page do not have
// the same reader. The name is what a page looks the sentence up by, so somebody who
// has been reading Spanish for twenty minutes is not handed English at the one point
// where the program goes quiet for several minutes over their whole history — which
// is exactly the moment a person decides it has hung.
//
// The English is sent as well, and it is not a fallback nobody uses: the command
// prints exactly this, and a page meeting a name from a newer program has to say
// something rather than nothing.
//
// The holes are written in braces, the way the page writes them, so one sentence and
// its translation take the same substitutions. Values fill them here; Counts leave
// the numbers as numbers, because 595236 is not how a Spanish reader writes 595.236
// and only the page knows that.
type Note struct {
	// Name is what the page looks the sentence up by. Empty means there is nothing
	// to look up — a line copied from a tool, say — and the text is all there is.
	Name string `json:"note,omitempty"`
	// Text is the sentence in English, already filled in.
	Text string `json:"detail,omitempty"`
	// Values are what filled it: a device, a date, a size.
	Values map[string]string `json:"values,omitempty"`
	// Counts are the numbers in it, kept as numbers.
	Counts []Count `json:"counts,omitempty"`
}

// Noted writes a note from a sentence with holes in it and the things that fill
// them, given as name, value, name, value.
//
// The English is filled from the same values the page will use, from the same
// template, which is the point: a call site cannot say one thing in the sentence it
// sends and another in the values it sends beside it, because it only says it once.
//
// An odd number of pairs is a programming error, and the last one is dropped rather
// than panicking in the middle of somebody's migration.
func Noted(name, template string, pairs ...string) Note {
	note := Note{Name: name, Text: template}
	if len(pairs) < 2 {
		return note
	}

	note.Values = make(map[string]string, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		note.Values[pairs[i]] = pairs[i+1]
	}
	note.Text = guide.Fill(template, note.Values)
	return note
}

// Saying is a note with no holes in it.
func Saying(name, sentence string) Note {
	return Note{Name: name, Text: sentence}
}

// Quoting is a line from something else — a tool's own output — which has no name
// because there is no sentence to translate, only what it said.
func Quoting(line string) Note {
	return Note{Text: line}
}

// Counting returns the note with one more number in it.
//
// The number goes beside the sentence rather than into it. A page that knows the
// name writes "1.234 mensajes" where this wrote "1,234 messages", which is the
// difference between a program that speaks somebody's language and one that has
// been translated except for the numbers.
func (n Note) Counting(of string, x int) Note {
	n.Counts = append(append([]Count(nil), n.Counts...), Count{Of: of, N: x})
	return n
}
