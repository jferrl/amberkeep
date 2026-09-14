package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jferrl/amberkeep/internal/api"
	"github.com/jferrl/amberkeep/internal/backupfs"
	"github.com/jferrl/amberkeep/internal/migrate"
)

// The migration, for the page rather than the terminal.
//
// Every method here is a few lines calling what the command already calls. The two
// must not become two implementations: a browser and a terminal disagreeing about
// what a migration does is the sort of difference nobody finds until it matters.

// Migrator moves a history onto a phone.
type Migrator struct {
	// Me and Country are how the archive is read, the same as everywhere else.
	Me      string
	Country string
}

// Check looks at a backup before anything else happens.
func (m Migrator) Check(ctx context.Context, backup string) (migrate.Readiness, error) {
	return migrate.Preflight(ctx, backup, WhatsAppDomain, ChatStorage)
}

// Plan works out what would move. It writes nothing.
func (m Migrator) Plan(ctx context.Context, ask api.MigrationRequest, say api.Progress) (migrate.Plan, error) {
	say(api.StepOpening, "Reading the archive and the phone's own messages.")

	book, whatsApp := addressBook(ask.Contacts)
	plan, _, from, err := PlanMigration(ctx, Planning{
		Android: ask.Android, Backup: ask.Backup, Pairing: ask.Pairing,
		Book: book, WhatsApp: whatsApp, Country: m.Country,
		Opts: migrate.Options{Groups: ask.Groups, Hidden: ask.Hidden, Only: ask.Only},
	})
	if err != nil {
		return migrate.Plan{}, err
	}
	// The plan is a description and holds nothing open. What carries it out opens
	// both sides again for itself.
	_ = from.Close()
	return plan, nil
}

// Carry does it: into a copy of the store, checked against the original, then into a
// copy of the backup. Nothing it touches is an original.
func (m Migrator) Carry(ctx context.Context, ask api.MigrationRequest, plan migrate.Plan,
	say api.Progress,
) (api.Migrated, error) {
	book, whatsApp := addressBook(ask.Contacts)

	say(api.StepOpening, "Taking the phone's messages out of the backup.")
	freshPlan, store, from, err := PlanMigration(ctx, Planning{
		Android: ask.Android, Backup: ask.Backup, Pairing: ask.Pairing,
		Book: book, WhatsApp: whatsApp, Country: m.Country,
		Opts: migrate.Options{Groups: ask.Groups, Hidden: ask.Hidden, Only: ask.Only},
	})
	if err != nil {
		return api.Migrated{}, err
	}
	defer func() { _ = from.Close() }()

	// The plan somebody read is the one that gets carried out. Working it out again
	// here is not duplication: it is how a change to the archive between reading and
	// agreeing is noticed rather than quietly acted on.
	if freshPlan.Adding != plan.Adding {
		return api.Migrated{}, fmt.Errorf(
			"the archive has changed since you read the plan: it said %d messages and now says %d. "+
				"Nothing has been written; ask for the plan again",
			plan.Adding, freshPlan.Adding)
	}

	say(api.StepPreparing, fmt.Sprintf("Moving %s. Nothing is being uploaded.",
		Plural(plan.Adding, "message", "messages")))
	merged := store + ".merged"
	result, err := migrate.Apply(ctx, from, store, merged, freshPlan)
	if err != nil {
		return api.Migrated{}, err
	}

	say(api.StepPreparing, "Checking every message that was written.")
	report, err := migrate.Verify(ctx, store, merged, freshPlan)
	if err != nil {
		return api.Migrated{}, err
	}
	if !report.OK() {
		return api.Migrated{}, fmt.Errorf("%s: %s", report.Summary(), firstFailure(report))
	}

	say(api.StepPreparing, "Putting it into a copy of the backup. The original is not touched.")
	into := ask.Into
	if into == "" {
		into = besideTheBackup(ask.Backup)
	}
	patched, err := backupfs.Patch(ctx, ask.Backup, into, backupfs.Replacement{
		Domain: WhatsAppDomain, RelativePath: ChatStorage, With: merged,
	})
	if err != nil {
		return api.Migrated{}, err
	}

	return api.Migrated{
		Backup: patched.Path, Added: result.Added,
		Merged: result.Merged, Created: result.Created,
		Checks: len(report.Checks), Files: patched.Files,
	}, nil
}

// firstFailure is the one thing to say about a result that did not pass.
func firstFailure(report migrate.Report) string {
	failures := report.Failures()
	if len(failures) == 0 {
		return ""
	}
	said := failures[0].Name
	if failures[0].Detail != "" {
		said += " (" + failures[0].Detail + ")"
	}
	return said
}

// besideTheBackup is where a changed backup goes when nobody says otherwise: next to
// the original, named after it and after today, so the two are never confused.
func besideTheBackup(backup string) string {
	clean := strings.TrimRight(backup, string(filepath.Separator))
	return filepath.Join(filepath.Dir(clean),
		filepath.Base(clean)+".amberkeep-"+time.Now().Format("20060102"))
}
