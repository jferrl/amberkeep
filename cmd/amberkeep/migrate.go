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

	"github.com/jferrl/amberkeep/internal/backupfs"
	"github.com/jferrl/amberkeep/internal/guide"
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
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *android == "" || *backup == "" {
		fs.Usage()
		return fmt.Errorf("both --android and --backup are needed")
	}

	ready, err := migrate.Preflight(ctx, *backup, whatsappDomain, chatStorage)
	if err != nil {
		return err
	}
	reportReadiness(told, ready)
	if !ready.OK() {
		return fmt.Errorf("the checks above have to pass before anything can be moved")
	}

	plan, target, from, err := planMigration(ctx, planning{
		android: *android, backup: *backup, pairing: *pairing,
		book: *bookPath, whatsApp: *waPath, country: *country,
		opts: migrate.Options{Groups: *groups, Hidden: *hidden, Only: addresses(*only)},
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
		show(told, guide.Before)
		return nil
	}

	show(told, guide.Before)
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

// planning is what working out a migration needs.
type planning struct {
	android, backup, pairing string
	book, whatsApp, country  string
	opts                     migrate.Options
}

// planMigration opens both sides and works out what would happen.
func planMigration(ctx context.Context, p planning) (migrate.Plan, string, source.Archive, error) {
	from, err := source.Open(ctx, p.android)
	if err != nil {
		return migrate.Plan{}, "", nil, err
	}
	if _, err := loadNames(ctx, from.Directory(), p.book, p.whatsApp, p.country); err != nil {
		_ = from.Close()
		return migrate.Plan{}, "", nil, err
	}
	mentionPreparation(from, p.android)

	// The store is taken out of the backup into a place of its own. The backup is
	// never opened for writing and never will be.
	work, err := os.MkdirTemp("", "amberkeep-migrate-")
	if err != nil {
		_ = from.Close()
		return migrate.Plan{}, "", nil, fmt.Errorf("making somewhere to work: %w", err)
	}
	store, err := takeStoreOut(ctx, p.backup, work)
	if err != nil {
		_ = from.Close()
		return migrate.Plan{}, "", nil, err
	}

	to, err := migrate.OpenTarget(ctx, store, p.pairing)
	if err != nil {
		_ = from.Close()
		return migrate.Plan{}, "", nil, err
	}
	defer func() { _ = to.Close() }()

	plan, err := migrate.Build(ctx, from, to, p.opts)
	if err != nil {
		_ = from.Close()
		return migrate.Plan{}, "", nil, err
	}
	return plan, store, from, nil
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
		plural(result.Added, "message", "messages"),
		plural(result.Merged+result.Created, "conversation", "conversations"),
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
	fmt.Fprintf(told, "  wrote %s in %s\n", abbreviate(patched.Path), patched.Took.Round(time.Second))
	if patched.LogNeutralised {
		fmt.Fprintf(told, "  emptied the write-ahead log the backup carried, which would have been replayed\n")
	}

	fmt.Fprintf(told, "\nDone. Nothing has touched the phone.\n")
	show(told, guide.Restoring)
	show(told, guide.After)
	fmt.Fprintf(told, "\nIf something goes wrong:\n")
	show(told, guide.Wrong)
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
func show(w io.Writer, stage guide.Stage) {
	steps := guide.At(stage)
	if len(steps) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s\n%s\n", strings.ToUpper(headingFor(stage)), strings.Repeat("─", 64))
	for i, step := range steps {
		fmt.Fprintf(w, "\n%d. %s\n", i+1, indent(step.Render()))
	}
}

// headingFor names a stage the way somebody reading it would.
func headingFor(stage guide.Stage) string {
	switch stage {
	case guide.Before:
		return "Before you restore"
	case guide.Restoring:
		return "Restoring, and what you will see"
	case guide.After:
		return "Once the phone comes back"
	case guide.Wrong:
		return "If it did not work"
	default:
		return string(stage)
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
			if step, ok := guide.Find(f.Step); ok {
				fmt.Fprintf(w, "\n%s\n", indent("  "+step.Render()))
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
		plural(plan.Creating, "conversation", "conversations"),
		plural(plan.Merging, "conversation", "conversations"),
		plural(plan.Untouched, "conversation", "conversations"))

	for _, warning := range plan.Warnings {
		fmt.Fprintf(w, "\n  ! %s\n", wrap(warning, 68, "    "))
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

// takeStoreOut copies WhatsApp's message store out of a backup, into a place of this
// command's own. The backup is opened read-only and is never written to.
func takeStoreOut(ctx context.Context, backup, work string) (string, error) {
	archive, err := backupfs.Open(ctx, backup)
	if err != nil {
		return "", err
	}
	defer func() { _ = archive.Close() }()

	return archive.ExtractDatabase(ctx, work, whatsappDomain, chatStorage)
}

// patchBackup puts a store into a copy of a backup.
func patchBackup(ctx context.Context, backup, into, store string) (backupfs.Patched, error) {
	return backupfs.Patch(ctx, backup, into, backupfs.Replacement{
		Domain:       whatsappDomain,
		RelativePath: chatStorage,
		With:         store,
	})
}
