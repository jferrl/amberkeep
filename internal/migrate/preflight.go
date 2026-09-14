package migrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jferrl/amberkeep/internal/backupfs"
)

// What can be checked before anybody commits to anything.
//
// Most of what has to be true cannot be checked from here: nothing in this program
// can see whether Find My is off, whether the safety backup was archived, or whether
// the phone is plugged in. Those are told rather than tested, which is what the guide
// is for. What is left is small and worth doing properly, because each of these has
// a specific way of wasting an hour.

// Finding is one thing that was checked.
type Finding struct {
	// Step is the guided step this bears on, so a failure can point at what to do.
	Step string `json:"step"`
	// Title is what was checked, in a few words.
	Title string `json:"title"`
	// Passed is whether it is as it needs to be.
	Passed bool `json:"passed"`
	// Blocking marks a finding that must be put right before going on, as against
	// one worth knowing.
	Blocking bool `json:"blocking"`
	// Detail says what was found.
	Detail string `json:"detail,omitempty"`
}

// Readiness is everything that was checked.
type Readiness struct {
	Findings []Finding `json:"findings"`
	// Needs is how much room the copy of the backup will take.
	Needs int64 `json:"needs"`
}

// OK reports whether anything blocking is outstanding.
func (r Readiness) OK() bool {
	for _, f := range r.Findings {
		if f.Blocking && !f.Passed {
			return false
		}
	}
	return len(r.Findings) > 0
}

// Blockers are the findings that have to be put right.
func (r Readiness) Blockers() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Blocking && !f.Passed {
			out = append(out, f)
		}
	}
	return out
}

// staleAfter is when a backup stops being recent enough to restore without losing
// something. Anything said on the phone after the backup was taken is not in it.
const staleAfter = 24 * time.Hour

// Preflight checks what can be checked about a backup before a migration.
//
// It opens the backup read-only and writes nothing. A blocking finding means going
// on would waste an hour or lose something; anything else is worth knowing and is
// the person's own decision.
func Preflight(ctx context.Context, backup, domain, relativePath string) (Readiness, error) {
	var r Readiness

	archive, err := backupfs.Open(ctx, backup)
	if err != nil {
		// The two that have their own advice are reported as findings rather than as
		// failures, because they are things to go and fix rather than things that
		// went wrong.
		switch {
		case errors.Is(err, backupfs.ErrEncrypted):
			r.add("encryption-off", "The backup is not encrypted", false, true,
				"it is encrypted, and an encrypted backup is sealed with a key that never "+
					"leaves the phone")
			return r, nil
		case errors.Is(err, backupfs.ErrNotABackup):
			r.add("fresh-backup", "That folder is a backup", false, true,
				"it has none of the four files every Finder, iTunes and Apple Devices backup has")
			return r, nil
		default:
			return r, err
		}
	}
	defer func() { _ = archive.Close() }()

	r.add("encryption-off", "The backup is not encrypted", true, true, "")

	// The store has to be in it. A backup taken before WhatsApp was ever opened, or
	// of a phone that does not have it, gets this far and no further.
	_, found, err := archive.Find(ctx, domain, relativePath)
	if err != nil {
		return r, err
	}
	r.add("fresh-backup", "The backup holds WhatsApp's messages", found, true,
		detailIf(!found, "there is no "+relativePath+" in it"))
	if !found {
		return r, nil
	}

	// Anything said on the phone after the backup was taken is not in the backup,
	// and will be gone after the restore. Not blocking, because somebody who has
	// said nothing since does not need to take another.
	age := time.Since(archive.LastBackup)
	fresh := !archive.LastBackup.IsZero() && age < staleAfter
	r.add("fresh-backup", "The backup is recent", fresh, false,
		freshness(archive.LastBackup, age))

	// Room for a second copy of the whole backup. Reported rather than checked:
	// asking the operating system how much space is left is a different question on
	// every one of the three this runs on, and getting a wrong answer here would be
	// worse than giving a number somebody can look at themselves.
	if size, err := folderSize(ctx, backup); err == nil {
		r.Needs = size
	}
	r.add("power-and-space", "There is room for a copy of the backup", true, false,
		fmt.Sprintf("it needs about %.1f GB free", float64(r.Needs)/(1<<30)))

	// The one that cannot be checked and matters most.
	r.add("safety-backup", "A safety backup exists and has been archived", false, false,
		"nothing here can see this; it is the only way back and it has to be done by hand")
	return r, nil
}

// add records one finding.
//
// The step is named rather than described: what to do about it is the guide's to
// say, and whoever is showing this looks it up. A check's own title is a different
// sentence from the step's — "the backup is not encrypted" against "turn off
// encrypted backups" — and both are wanted.
func (r *Readiness) add(step, title string, passed, blocking bool, detail string) {
	r.Findings = append(r.Findings,
		Finding{Step: step, Title: title, Passed: passed, Blocking: blocking, Detail: detail})
}

func detailIf(when bool, detail string) string {
	if when {
		return detail
	}
	return ""
}

// freshness says how old a backup is in words somebody would use.
func freshness(at time.Time, age time.Duration) string {
	if at.IsZero() {
		return "it does not say when it was taken"
	}
	switch {
	case age < time.Hour:
		return "taken within the hour"
	case age < staleAfter:
		return "taken " + plural(int(age.Hours()), "hour", "hours") + " ago"
	default:
		return "taken " + plural(int(age.Hours()/24), "day", "days") +
			" ago; anything said on the phone since is not in it"
	}
}

// folderSize adds up a backup, so somebody can be told what a copy will cost.
func folderSize(ctx context.Context, root string) (int64, error) {
	var total int64
	err := walkFiles(ctx, root, func(info os.FileInfo) {
		total += info.Size()
	})
	return total, err
}

// walkFiles visits every ordinary file under a folder.
func walkFiles(ctx context.Context, root string, each func(os.FileInfo)) error {
	entries, err := os.ReadDir(root) // #nosec G703 -- the folder the caller named
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			if err := walkFiles(ctx, path, each); err != nil {
				return err
			}
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		each(info)
	}
	return nil
}
