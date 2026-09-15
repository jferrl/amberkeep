package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/app"
	"github.com/jferrl/amberkeep/internal/backupfs"
	"github.com/jferrl/amberkeep/internal/guide"
	"github.com/jferrl/amberkeep/internal/licence"
	"github.com/jferrl/amberkeep/internal/migrate"
	"github.com/jferrl/amberkeep/internal/source"
)

// Moving an Android history onto an iPhone, one told step at a time.
//
// Nothing here touches a phone. What it produces is a backup folder, and putting that
// onto a phone is Finder's job — which is the whole design: the risky part is done by
// Apple's own tool, doing the thing it does every day, and this program's job is to
// hand it something correct and to tell somebody exactly what to expect.
//
// It does nothing at all unless asked twice. Running it shows what would happen and
// stops; writing anything needs a word typed out in full, because a flag can be in
// somebody's shell history and a typed word cannot be there by accident.

// confirmation is what somebody has to type before anything is written.
const confirmation = "migrate"

// unproven is said every time, at the top, before anything else.
//
// What this produces has been checked more thoroughly than most things in this
// program: it agrees with the prototype that performed the original migration, and
// every consistency check passes on a full run. None of that is the same as a phone
// having accepted one, and the difference is somebody else's phone. Saying so in the
// release notes is not enough — somebody running a command has not read those.
const unproven = "Note: no backup this command has produced has yet been restored to a phone.\n" +
	"Everything it writes is checked, and the checks pass, but that is not the same\n" +
	"thing. Read what follows, and keep the archived safety backup it asks for.\n"

// Where the command reads and writes. Variables so a test can drive the whole
// command and read what somebody would have seen, which is the only way to check
// that what it says is what it does.
var (
	asked = io.Reader(os.Stdin)
	told  = io.Writer(os.Stdout)
)

func runMigrate(ctx context.Context, args []string) error {
	fs := newFlagSet("migrate", "move an Android history into a copy of an iPhone backup")
	var (
		android  = fs.String("android", "", "the decrypted Android database, usually msgstore.db")
		backup   = fs.String("backup", "", "the iPhone backup folder, named after the device identifier")
		out      = fs.String("out", "", "where to write the changed backup (default: beside the original)")
		bookPath = fs.String("contacts", "", "an address book, so conversations arrive with names")
		waPath   = fs.String("whatsapp-contacts", "", "WhatsApp's own contacts database, usually wa.db")
		pairing  = fs.String("lid-pairs", "", "WhatsApp's LID.sqlite from the same backup; without it people known by a hidden identity on one phone and a number on the other arrive twice")
		country  = fs.String("country", "", "dialling code for numbers saved without one, such as 34")
		groups   = fs.Bool("groups", false, "include group conversations")
		hidden   = fs.Bool("hidden", false, "include conversations that exist only behind a hidden identity")
		only     = fs.String("only", "", "move only this conversation, by address; the way to try one first")
		write    = fs.Bool("write", false, "actually write, after typing the confirmation")
		printed  = fs.Bool("guide", false, "print the restore instructions and stop, for reading away from the screen")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	// The whole guide, on its own. Somebody about to do this on a phone they depend
	// on may reasonably want it on paper, or in front of them on a second screen,
	// before anything has been chosen — and every other way to reach it requires
	// starting a migration first.
	if *printed {
		told := os.Stdout
		for _, stage := range []guide.Stage{guide.Before, guide.Restoring, guide.After, guide.Wrong} {
			show(told, stage, spoken())
		}
		return nil
	}

	if *android == "" || *backup == "" {
		fs.Usage()
		return fmt.Errorf("both --android and --backup are needed")
	}

	fmt.Fprintf(told, "%s\n", unproven)

	ready, err := migrate.Preflight(ctx, *backup, app.WhatsAppDomain, app.ChatStorage)
	if err != nil {
		return err
	}
	reportReadiness(told, ready)
	if !ready.OK() {
		return fmt.Errorf("the checks above have to pass before anything can be moved")
	}

	plan, target, from, err := app.PlanMigration(ctx, app.Planning{
		Android: *android, Backup: *backup, Pairing: *pairing,
		Book: *bookPath, WhatsApp: *waPath, Country: *country,
		Opts: migrate.Options{Groups: *groups, Hidden: *hidden, Only: addresses(*only)},
	})
	if err != nil {
		return err
	}
	defer func() { _ = from.Close() }()

	reportPlan(told, plan)
	if plan.Empty() {
		fmt.Fprintf(told, "\nThere is nothing to move. Everything in this archive is already on the phone.\n")
		return nil
	}

	if !*write {
		fmt.Fprintf(told, "\nNothing has been written. This is what would happen.\n\n")
		fmt.Fprintf(told, "To do it, read what follows, then run the same command again with --write.\n")
		show(told, guide.Before, spoken())
		return nil
	}

	// The plan is free and the writing is not: somebody sees exactly what would
	// happen to their own history before any of this is about money. Asked before
	// the confirmation rather than after, because being made to type a word and
	// then told no would be a poor way to learn the price.
	if err := licence.Allows(licence.Migration); err != nil {
		return err
	}

	show(told, guide.Before, spoken())
	if err := confirm(asked, told); err != nil {
		return err
	}

	into := *out
	if into == "" {
		into = filepath.Join(filepath.Dir(strings.TrimRight(*backup, string(filepath.Separator))),
			filepath.Base(strings.TrimRight(*backup, string(filepath.Separator)))+".amberkeep")
	}
	return carryOut(ctx, from, target, plan, *backup, into)
}

// carryOut does the writing, in the order that keeps it checkable: into a copy of
// the store, checked against the original, then into a copy of the backup.
func carryOut(ctx context.Context, from source.Archive, store string, plan migrate.Plan, backup, into string) error {
	merged := store + ".merged"

	fmt.Fprintf(told, "\nMoving the messages. Nothing is being uploaded.\n")
	result, err := migrate.Apply(ctx, from, store, merged, plan)
	if err != nil {
		return err
	}
	fmt.Fprintf(told, "  moved %s into %s in %s\n",
		app.Plural(result.Added, "message", "messages"),
		app.Plural(result.Merged+result.Created, "conversation", "conversations"),
		result.Took.Round(time.Second))

	fmt.Fprintf(told, "\nChecking what was written.\n")
	report, err := migrate.Verify(ctx, store, merged, plan)
	if err != nil {
		return err
	}
	if !report.OK() {
		for _, c := range report.Failures() {
			fmt.Fprintf(os.Stderr, "  failed: %s%s\n", c.Name, detailOf(c.Detail))
		}
		return fmt.Errorf("%s; nothing has been put into a backup", report.Summary())
	}
	fmt.Fprintf(told, "  %s\n", report.Summary())

	fmt.Fprintf(told, "\nPutting it into a copy of the backup. The original is not touched.\n")
	patched, err := patchBackup(ctx, backup, into, merged)
	if err != nil {
		return err
	}
	fmt.Fprintf(told, "  wrote %s in %s\n", app.Abbreviate(patched.Path), patched.Took.Round(time.Second))
	if patched.LogNeutralised {
		fmt.Fprintf(told, "  emptied the write-ahead log the backup carried, which would have been replayed\n")
	}

	fmt.Fprintf(told, "\nDone. Nothing has touched the phone.\n")
	show(told, guide.Restoring, spoken())
	show(told, guide.After, spoken())
	fmt.Fprintf(told, "\nIf something goes wrong:\n")
	show(told, guide.Wrong, spoken())
	return nil
}

// confirm asks for the word, and accepts nothing else.
//
// Typed out rather than a flag on its own: a flag lives in a shell history and can be
// recalled with an arrow key, and this is the step that ends with somebody's phone
// being written to.
func confirm(in io.Reader, out io.Writer) error {
	fmt.Fprintf(out, "\n%s\n", strings.Repeat("─", 64))
	fmt.Fprintf(out, "Everything above has to be true before going on.\n\n")
	fmt.Fprintf(out, "Type %q to continue, or anything else to stop: ", confirmation)

	said, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && said == "" {
		return fmt.Errorf("nothing was typed, so nothing was done")
	}
	if strings.TrimSpace(said) != confirmation {
		return fmt.Errorf("that is not %q, so nothing was done", confirmation)
	}
	return nil
}

// show prints the steps of one stage.
func show(w io.Writer, stage guide.Stage, lang guide.Language) {
	steps := guide.At(stage, lang)
	if len(steps) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s\n%s\n", strings.ToUpper(guide.Heading(stage, lang)), strings.Repeat("─", 64))
	for i, step := range steps {
		fmt.Fprintf(w, "\n%d. %s\n", i+1, indent(step.Render(spoken())))
	}
}

// indent lines up the lines under a numbered step.
func indent(s string) string {
	return strings.ReplaceAll(s, "\n", "\n   ")
}

// wrap breaks a long sentence into lines somebody can read, keeping any line breaks
// the text already had, because those were put there deliberately.
func wrap(s string, width int, prefix string) string {
	var out []string
	for _, paragraph := range strings.Split(s, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			if len(line)+1+len(word) > width {
				out = append(out, line)
				line = word
				continue
			}
			line += " " + word
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n"+prefix)
}

// reportReadiness prints what could be checked, and what could not.
func reportReadiness(w io.Writer, ready migrate.Readiness) {
	fmt.Fprintf(w, "Checks\n%s\n", strings.Repeat("─", 64))
	for _, f := range ready.Findings {
		mark := "  ok "
		switch {
		case f.Passed:
		case f.Blocking:
			mark = "FAIL "
		default:
			mark = "note "
		}
		fmt.Fprintf(w, "%s %s%s\n", mark, f.Title, detailOf(f.Detail))
	}
	if blockers := ready.Blockers(); len(blockers) > 0 {
		fmt.Fprintf(w, "\nWhat to do:\n")
		for _, f := range blockers {
			if step, ok := guide.Find(f.Step, spoken()); ok {
				fmt.Fprintf(w, "\n%s\n", indent("  "+step.Render(spoken())))
			}
		}
	}
}

// reportPlan prints what would happen, which is the thing somebody agrees to.
func reportPlan(w io.Writer, plan migrate.Plan) {
	fmt.Fprintf(w, "\nWhat would move\n%s\n", strings.Repeat("─", 64))
	fmt.Fprintf(w, "  %s\n", plan.Summary())
	if !plan.Earliest.IsZero() {
		fmt.Fprintf(w, "  from %s to %s\n",
			plan.Earliest.Format("2 January 2006"), plan.Latest.Format("2 January 2006"))
	}
	fmt.Fprintf(w, "  %s would be created, %s merged into, %s left alone\n",
		app.Plural(plan.Creating, "conversation", "conversations"),
		app.Plural(plan.Merging, "conversation", "conversations"),
		app.Plural(plan.Untouched, "conversation", "conversations"))

	// In whatever this terminal reads. The numbers in these sentences are the same
	// either way; the sentence around them is not.
	for _, warning := range plan.Warnings {
		said, ok := guide.Sentence(warning.Note, warning.Values, spoken())
		if !ok {
			said = warning.Text
		}
		fmt.Fprintf(w, "\n  ! %s\n", wrap(said, 68, "    "))
	}
}

// addresses splits a comma-separated list, which is how one conversation is chosen.
func addresses(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func detailOf(s string) string {
	if s == "" {
		return ""
	}
	return " — " + s
}

// patchBackup puts a store into a copy of a backup.
func patchBackup(ctx context.Context, backup, into, store string) (backupfs.Patched, error) {
	return backupfs.Patch(ctx, backup, into, backupfs.Replacement{
		Domain:       app.WhatsAppDomain,
		RelativePath: app.ChatStorage,
		With:         store,
	})
}

// spoken is the language this terminal reads.
//
// Taken from the environment rather than asked for: somebody running a command has
// already told their system what they read, and asking again would be a flag nobody
// sets. LC_ALL wins over LANG, which is the order every other program uses.
func spoken() guide.Language {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if tag := os.Getenv(name); tag != "" {
			return guide.Spoken(tag)
		}
	}
	return guide.English
}
