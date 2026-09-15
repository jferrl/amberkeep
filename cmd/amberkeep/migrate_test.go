package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jferrl/amberkeep/internal/fixture"

	"github.com/jferrl/amberkeep/internal/guide"
)

// TestNothingHappensWithoutTheWord covers the step that ends with somebody's phone
// being written to.
//
// A flag would do, and a flag lives in a shell history where an arrow key can recall
// it. A word typed out cannot be there by accident.
func TestNothingHappensWithoutTheWord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		typed string
		agree bool
	}{
		{name: "the word", typed: "migrate\n", agree: true},
		{name: "the word with a stray space", typed: "  migrate  \n", agree: true},
		{name: "yes", typed: "yes\n"},
		{name: "y", typed: "y\n"},
		{name: "nothing at all", typed: "\n"},
		{name: "a closed terminal", typed: ""},
		{name: "the word inside a sentence", typed: "I want to migrate\n"},
		{name: "the word in capitals", typed: "MIGRATE\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := confirm(strings.NewReader(tt.typed), io.Discard)
			if agreed := err == nil; agreed != tt.agree {
				t.Errorf("typing %q was accepted = %v, want %v", tt.typed, agreed, tt.agree)
			}
		})
	}
}

// TestWrappingKeepsTheLineBreaksSomebodyPutThere covers the guidance being readable
// rather than a wall: the text carries its own paragraphs and they have to survive.
func TestWrappingKeepsTheLineBreaksSomebodyPutThere(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		give  string
		width int
		want  []string
	}{
		{
			name: "a long sentence is broken up",
			give: "one two three four five six seven eight nine ten",
			// 20 characters is enough for three words of these lengths.
			width: 20,
			want:  []string{"one two three four", "five six seven eight", "nine ten"},
		},
		{
			name:  "a paragraph break is kept",
			give:  "first\n\nsecond",
			width: 40,
			want:  []string{"first", "", "second"},
		},
		{
			name:  "a word longer than the line is left alone rather than cut",
			give:  "a supercalifragilisticexpialidocious word",
			width: 10,
			want:  []string{"a", "supercalifragilisticexpialidocious", "word"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := strings.Split(wrap(tt.give, tt.width, ""), "\n")
			if len(got) != len(tt.want) {
				t.Fatalf("wrapped to %d lines, want %d: %q", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("line %d is %q, want %q", i+1, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestTheCommandCanShowEveryStage is a smoke test for the part nobody would notice
// breaking: a stage whose heading falls through to its own identifier.
func TestTheCommandCanShowEveryStage(t *testing.T) {
	t.Parallel()

	for _, stage := range []guide.Stage{guide.Before, guide.Restoring, guide.After, guide.Wrong} {
		t.Run(string(stage), func(t *testing.T) {
			t.Parallel()

			// In both languages. A stage with no heading falls back to its own
			// identifier, which is how a screen ends up saying "restoring" at
			// somebody in the middle of restoring.
			for _, lang := range []guide.Language{guide.English, guide.Spanish} {
				if heading := guide.Heading(stage, lang); heading == string(stage) || heading == "" {
					t.Errorf("%s has no %s heading, so it shows as %q", stage, lang, heading)
				}
			}
		})
	}
}

// TestOnlyTakesAListOfAddresses covers how somebody tries one conversation first,
// which is the recommended way to use this at all.
func TestOnlyTakesAListOfAddresses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
		want int
	}{
		{name: "nothing means everything", give: "", want: 0},
		{name: "only spaces means everything", give: "   ", want: 0},
		{name: "one", give: "a@s.whatsapp.net", want: 1},
		{name: "several", give: "a@s.whatsapp.net, b@s.whatsapp.net", want: 2},
		{name: "a trailing comma is not an address", give: "a@s.whatsapp.net,", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := addresses(tt.give); len(got) != tt.want {
				t.Errorf("addresses(%q) = %v, want %d of them", tt.give, got, tt.want)
			}
		})
	}
}

// TestTheWholeGuidedFlow drives the command the way somebody would, against a
// synthetic backup and a synthetic archive, and reads what they would have seen.
//
// It is one test rather than several because the thing worth checking is the journey:
// that the checks run before the plan, that the plan is shown before anything is
// written, that nothing is written without the word, and that what is written is
// checked before anybody is told to restore it.
func TestTheWholeGuidedFlow(t *testing.T) {
	dir := t.TempDir()
	backup := fixture.BackupWithAStore(t, dir)

	android := filepath.Join(dir, "msgstore.db")
	fixture.TinyArchive(t, android)

	// The command talks through these rather than the terminal.
	var said bytes.Buffer
	told = &said
	t.Cleanup(func() { told = os.Stdout; asked = os.Stdin })

	t.Run("saying nothing writes nothing", func(t *testing.T) {
		said.Reset()
		if err := run(t.Context(), []string{"migrate", "--android", android, "--backup", backup}); err != nil {
			t.Fatalf("the dry run failed: %v", err)
		}

		shown := said.String()
		for _, want := range []string{
			"Checks",
			"The backup is not encrypted",
			"A safety backup exists and has been archived",
			"What would move",
			"Nothing has been written",
			"BEFORE YOU RESTORE",
			"Make a safety backup, and archive it",
			"[do not skip]",
		} {
			if !strings.Contains(shown, want) {
				t.Errorf("it never said %q", want)
			}
		}
		// And nothing about what comes after, because nothing has happened yet.
		if strings.Contains(shown, "ONCE THE PHONE COMES BACK") {
			t.Error("it explained the restore before anything had been written")
		}
	})

	t.Run("nor does asking to write without typing the word", func(t *testing.T) {
		said.Reset()
		asked = strings.NewReader("yes\n")

		err := run(t.Context(), []string{"migrate", "--android", android, "--backup", backup, "--write"})
		if err == nil {
			t.Fatal("it went ahead without the word")
		}
		if !strings.Contains(err.Error(), "nothing was done") {
			t.Errorf("it stopped with %q", err)
		}
	})

	t.Run("typing the word carries it out and checks the result", func(t *testing.T) {
		said.Reset()
		asked = strings.NewReader(confirmation + "\n")
		into := filepath.Join(t.TempDir(), "patched")

		if err := run(t.Context(), []string{
			"migrate", "--android", android, "--backup", backup, "--out", into, "--write",
		}); err != nil {
			t.Fatalf("the migration failed: %v", err)
		}

		shown := said.String()
		for _, want := range []string{
			"Moving the messages",
			"Checking what was written",
			"checks passed",
			"The original is not touched",
			"RESTORING, AND WHAT YOU WILL SEE",
			"ONCE THE PHONE COMES BACK",
			"Say no to restoring chats from iCloud",
			"IF IT DID NOT WORK",
		} {
			if !strings.Contains(shown, want) {
				t.Errorf("it never said %q", want)
			}
		}

		// And there is a backup to restore, which is the point of all of it.
		if _, err := os.Stat(filepath.Join(into, "Manifest.db")); err != nil {
			t.Errorf("no backup was produced: %v", err)
		}
	})
}

// TestItRefusesABackupItCannotUse covers the first thing that happens, which is the
// one that saves somebody an hour.
func TestItRefusesABackupItCannotUse(t *testing.T) {
	dir := t.TempDir()
	android := filepath.Join(dir, "msgstore.db")
	fixture.TinyArchive(t, android)

	var said bytes.Buffer
	told = &said
	t.Cleanup(func() { told = os.Stdout })

	err := run(t.Context(), []string{"migrate", "--android", android, "--backup", t.TempDir()})
	if err == nil {
		t.Fatal("it accepted a folder that is not a backup")
	}
	if !strings.Contains(said.String(), "FAIL") {
		t.Errorf("the checks did not say what was wrong:\n%s", said.String())
	}
	// And it says what to do about it rather than only that it is wrong.
	if !strings.Contains(said.String(), "What to do") {
		t.Error("it did not say what to do about it")
	}
}
