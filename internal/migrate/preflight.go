package migrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jferrl/amberkeep/internal/backupfs"
	"github.com/jferrl/amberkeep/internal/guide"
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
	// Check names the sentence saying what was looked at, and Note the one saying
	// what was found. A page looks them up and says them in its reader's language;
	// Title and Detail are the same two sentences in English, for the terminal and
	// for a page meeting a name from a newer program.
	Check string `json:"check,omitempty"`
	Note  string `json:"note,omitempty"`
	// Values fill the holes in either of them: a missing file, a number of days.
	Values map[string]string `json:"values,omitempty"`
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
			r.add("encryption-off", "not-encrypted", false, true, found("is-encrypted"))
			return r, nil
		case errors.Is(err, backupfs.ErrNotABackup):
			r.add("fresh-backup", "is-a-backup", false, true, found("no-backup-files"))
			return r, nil
		default:
			return r, err
		}
	}
	defer func() { _ = archive.Close() }()

	r.add("encryption-off", "not-encrypted", true, true, nothing)

	// The store has to be in it. A backup taken before WhatsApp was ever opened, or
	// of a phone that does not have it, gets this far and no further.
	_, has, err := archive.Find(ctx, domain, relativePath)
	if err != nil {
		return r, err
	}
	r.add("fresh-backup", "holds-messages", has, true,
		detailIf(!has, found("no-store", "file", relativePath)))
	if !has {
		return r, nil
	}

	// Anything said on the phone after the backup was taken is not in the backup,
	// and will be gone after the restore. Not blocking, because somebody who has
	// said nothing since does not need to take another.
	age := time.Since(archive.LastBackup)
	fresh := !archive.LastBackup.IsZero() && age < staleAfter
	r.add("fresh-backup", "is-recent", fresh, false, freshness(archive.LastBackup, age))

	// Room for a second copy of the whole backup. Reported rather than checked:
	// asking the operating system how much space is left is a different question on
	// every one of the three this runs on, and getting a wrong answer here would be
	// worse than giving a number somebody can look at themselves.
	if size, err := folderSize(ctx, backup); err == nil {
		r.Needs = size
	}
	r.add("power-and-space", "has-room", true, false,
		found("needs-space", "size", fmt.Sprintf("%.1f", float64(r.Needs)/(1<<30))))

	// The one that cannot be checked and matters most.
	r.add("safety-backup", "safety-backup", false, false, found("cannot-see-safety"))
	return r, nil
}

// add records one finding.
//
// Everything a person reads here is named rather than written out: the step, so what
// to do about it can be looked up, and both of the finding's own sentences, so they
// can be read in whatever language the reader chose. The English is filled in from
// the same names and the same values, so a check cannot say one thing in the sentence
// it sends and another in the name beside it.
//
// A check's own sentence is a different one from the step's — "the backup is not
// encrypted" against "turn off encrypted backups" — and both are wanted.
func (r *Readiness) add(step, check string, passed, blocking bool, found note) {
	title, _ := guide.Check(check, nil, guide.English)
	f := Finding{Step: step, Check: check, Passed: passed, Blocking: blocking, Title: title}
	if found.name != "" {
		f.Note, f.Values = found.name, found.values
		f.Detail, _ = guide.Check(found.name, found.values, guide.English)
	}
	r.Findings = append(r.Findings, f)
}

// note is what a check found: the name of a sentence and whatever fills its holes.
type note struct {
	name   string
	values map[string]string
}

// found names a sentence, with the things that fill it given as name, value pairs.
func found(name string, pairs ...string) note {
	n := note{name: name}
	if len(pairs) < 2 {
		return n
	}
	n.values = make(map[string]string, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		n.values[pairs[i]] = pairs[i+1]
	}
	return n
}

// nothing is a check that passed and has nothing to add.
var nothing = note{}

func detailIf(when bool, detail note) note {
	if when {
		return detail
	}
	return nothing
}

// freshness says how old a backup is in words somebody would use.
func freshness(at time.Time, age time.Duration) note {
	switch hours, days := int(age.Hours()), int(age.Hours()/24); {
	case at.IsZero():
		return found("no-date")
	case age < time.Hour:
		return found("within-the-hour")
	case hours == 1:
		return found("an-hour-ago")
	case age < staleAfter:
		return found("hours-ago", "hours", strconv.Itoa(hours))
	case days == 1:
		return found("a-day-ago")
	default:
		return found("days-ago", "days", strconv.Itoa(days))
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
