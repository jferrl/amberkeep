package api

import (
	"encoding/json"
	"testing"
)

// TestASentenceIsWrittenOnceAndSaidTwice covers the one thing this type exists to
// prevent: a call site saying one thing in the English it sends and another in the
// values it sends beside it. There is only one template, so there is only one
// sentence, and the page's version of it takes the same substitutions.
func TestASentenceIsWrittenOnceAndSaidTwice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		note     Note
		wantText string
		wantVals map[string]string
	}{
		{
			name:     "a sentence with nothing in it",
			note:     Saying("checkingWritten", "Checking every message that was written."),
			wantText: "Checking every message that was written.",
		},
		{
			name: "one with holes in it",
			note: Noted("readingBackupOf", "Reading the backup of {device}, made {when}.",
				"device", "iPhone", "when", "2026-08-01"),
			wantText: "Reading the backup of iPhone, made 2026-08-01.",
			wantVals: map[string]string{"device": "iPhone", "when": "2026-08-01"},
		},
		{
			name:     "the same hole twice",
			note:     Noted("twice", "{who} and {who}.", "who", "Ana"),
			wantText: "Ana and Ana.",
			wantVals: map[string]string{"who": "Ana"},
		},
		{
			name:     "a hole nothing fills, which stays as it was rather than emptying",
			note:     Noted("partly", "Moving {messages} to {where}.", "messages", "four"),
			wantText: "Moving four to {where}.",
			wantVals: map[string]string{"messages": "four"},
		},
		{
			name:     "a line from a tool, which has no sentence to translate",
			note:     Quoting("[ 45%] /sdcard/msgstore.db.crypt15"),
			wantText: "[ 45%] /sdcard/msgstore.db.crypt15",
		},
		{
			name:     "an odd pair, dropped rather than panicking mid-migration",
			note:     Noted("odd", "Moving {messages}.", "messages"),
			wantText: "Moving {messages}.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.note.Text != tt.wantText {
				t.Errorf("said %q, want %q", tt.note.Text, tt.wantText)
			}
			if len(tt.note.Values) != len(tt.wantVals) {
				t.Fatalf("filled it with %v, want %v", tt.note.Values, tt.wantVals)
			}
			for name, want := range tt.wantVals {
				if got := tt.note.Values[name]; got != want {
					t.Errorf("{%s} was %q, want %q", name, got, want)
				}
			}
		})
	}
}

// TestTheNumbersTravelAsNumbers covers what the page needs to write 595.236 rather
// than 595236: the count beside the sentence, not only inside it.
func TestTheNumbersTravelAsNumbers(t *testing.T) {
	t.Parallel()

	note := Noted("indexedSoFar", "Indexed {conversations} conversations and {messages} messages so far.",
		"conversations", "1200", "messages", "595236").
		Counting("conversations", 1200).
		Counting("messages", 595236)

	if len(note.Counts) != 2 {
		t.Fatalf("it carried %d numbers, want 2", len(note.Counts))
	}
	if note.Counts[0] != (Count{Of: "conversations", N: 1200}) {
		t.Errorf("the first number was %v", note.Counts[0])
	}
	if note.Counts[1] != (Count{Of: "messages", N: 595236}) {
		t.Errorf("the second number was %v", note.Counts[1])
	}

	// Counting returns a note rather than changing one, so a note kept and added to
	// twice does not grow a third number behind the first caller's back.
	base := Saying("moving", "Moving.")
	one, two := base.Counting("messages", 1), base.Counting("messages", 2)
	if len(base.Counts) != 0 || len(one.Counts) != 1 || len(two.Counts) != 1 {
		t.Errorf("counting changed what it was given: %v %v %v", base.Counts, one.Counts, two.Counts)
	}
}

// TestNothingEmptyIsSent covers the shape the page is declared against: a note with
// no name and no numbers must not arrive as nulls and empty lists it has to guard.
func TestNothingEmptyIsSent(t *testing.T) {
	t.Parallel()

	body, err := json.Marshal(Quoting("[ 45%] pulling"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(body), `{"detail":"[ 45%] pulling"}`; got != want {
		t.Errorf("sent %s, want %s", got, want)
	}
}
