// Package phone reads an Android phone over adb, and only reads it.
//
// The wizard's fourth screen currently asks somebody to open
// Android/media/com.whatsapp/WhatsApp/Databases on the phone's own storage and copy
// a file off it. That is a sentence a developer writes and a person bounces off:
// the folder is hidden on most launchers, the file beside the one they want is a
// fragment that looks identical, and getting it wrong wastes the twenty minutes the
// backup took.
//
// The phone is usually plugged into the same computer. This asks it.
//
// # What this will not do
//
// Every command is built as a fixed argument list and every one of them reads.
// There is no push, no shell that could remove anything, no root, no install, and
// no way for a caller to reach any of those: the operations are methods on this
// type rather than a string somebody composes. A phone that has somebody's whole
// life on it is not a thing to be clever near.
package phone

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ADB is the Android debug bridge on this machine.
type ADB struct {
	// Binary is what to run. Found on the path rather than bundled: it belongs to
	// whichever Android tooling somebody already installed, and shipping a copy
	// would mean shipping Google's build tools inside a program about privacy.
	Binary string
}

var (
	// ErrNoADB reports that the Android tools are not installed.
	ErrNoADB = errors.New("adb is not installed on this computer")

	// ErrUnusable reports Android tools that are installed and will not run. A
	// half-installed SDK, a quarantined binary, a daemon that cannot start: all of
	// them are the same thing to somebody reading a screen, which is that this way
	// in is not available and the other one still is.
	ErrUnusable = errors.New("the Android tools on this computer would not run")
)

// Find looks for adb where Android's own tooling puts it.
func Find() (ADB, error) {
	if found, err := exec.LookPath("adb"); err == nil {
		return ADB{Binary: found}, nil
	}
	// The usual places when the SDK is installed but never added to a shell profile,
	// which is most of them.
	for _, guess := range guesses() {
		if info, err := exec.LookPath(guess); err == nil {
			return ADB{Binary: info}, nil
		}
	}
	return ADB{}, ErrNoADB
}

// Phone is a device this computer can see.
type Phone struct {
	Serial string `json:"serial"`
	// Name is what to call it on screen: the model, or the serial when the phone
	// will not say until it is trusted.
	Name string `json:"name"`
	// Ready is whether it can actually be read. A phone that is plugged in but has
	// not had the prompt accepted is visible and useless, and saying so is the
	// difference between a person tapping "allow" and concluding it does not work.
	Ready bool `json:"ready"`
	// Trouble says why it is not ready, when it is not.
	Trouble string `json:"trouble,omitempty"`
}

// serials are what a device identifier may contain. Anything else is not put on a
// command line, whatever adb thinks of it.
var serials = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)

// Phones lists the devices this computer can see.
func (a ADB) Phones(ctx context.Context) ([]Phone, error) {
	out, err := a.read(ctx, "devices", "-l")
	if err != nil {
		return nil, err
	}

	var phones []Phone
	scan := bufio.NewScanner(strings.NewReader(out))
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "List of devices") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !serials.MatchString(fields[0]) {
			continue
		}

		p := Phone{Serial: fields[0], Name: fields[0]}
		switch fields[1] {
		case "device":
			p.Ready = true
		case "unauthorized":
			p.Trouble = "unauthorized"
		case "offline":
			p.Trouble = "offline"
		default:
			p.Trouble = fields[1]
		}

		// The model is only readable once the phone trusts this computer.
		if p.Ready {
			if model := a.property(ctx, p.Serial, "ro.product.model"); model != "" {
				p.Name = model
			}
		}
		phones = append(phones, p)
	}
	return phones, scan.Err()
}

// Where WhatsApp keeps its backups. The first is where Android 11 and later put it;
// the second is where everything before that did.
var databases = []string{
	"/sdcard/Android/media/com.whatsapp/WhatsApp/Databases",
	"/sdcard/WhatsApp/Databases",
}

// The two places the files themselves live, beside the two above. Constants for the
// same reason: nothing a caller composes ever reaches adb.
var folders = []string{
	"/sdcard/Android/media/com.whatsapp/WhatsApp/Media",
	"/sdcard/WhatsApp/Media",
}

// Backup is an encrypted message store on the phone.
type Backup struct {
	// Path is where it is on the phone.
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	// When the phone wrote it, when the phone said.
	When time.Time `json:"when,omitzero"`
	// Partial marks a msgstore-increment file: a fragment of a later backup, useless
	// on its own, and sitting in the same folder looking almost identical. Naming
	// them is cheaper than explaining them.
	Partial bool `json:"partial"`
}

// Backups lists the message stores on a phone, newest first.
func (a ADB) Backups(ctx context.Context, serial string) ([]Backup, error) {
	if !serials.MatchString(serial) {
		return nil, fmt.Errorf("that is not a device identifier")
	}

	var found []Backup
	for _, dir := range databases {
		// -l for the size and date, and a pattern adb expands on the phone. The
		// directory is one of two constants above, never anything a caller chose.
		out, err := a.read(ctx, "-s", serial, "shell", "ls", "-l", path.Join(dir, "msgstore*.crypt*"))
		if err != nil || strings.Contains(out, "No such file") {
			continue
		}
		found = append(found, parseListing(out, dir)...)
	}
	return found, nil
}

// parseListing reads what `ls -l` said, which is the one thing here that has to cope
// with a format nobody promised. A line it cannot read is skipped rather than
// guessed at: a wrong size is a progress bar that lies, and a wrong name is a file
// that will not pull.
func parseListing(out, dir string) []Backup {
	var found []Backup
	scan := bufio.NewScanner(strings.NewReader(out))
	for scan.Scan() {
		fields := strings.Fields(scan.Text())
		if len(fields) < 5 {
			continue
		}
		name := fields[len(fields)-1]
		if !strings.HasPrefix(path.Base(name), "msgstore") {
			continue
		}

		b := Backup{
			Path:    path.Join(dir, path.Base(name)),
			Name:    path.Base(name),
			Partial: strings.Contains(name, "msgstore-increment"),
		}
		// The size column moves depending on which ls the phone ships. The largest
		// plain number on the line is it, which is true of every layout seen.
		for _, f := range fields {
			if n, err := strconv.ParseInt(f, 10, 64); err == nil && n > b.Size {
				b.Size = n
			}
		}
		found = append(found, b)
	}
	return found
}

// Fetch copies one backup off the phone, and reports how far along it is.
//
// The phone is not touched: this reads the file and writes a copy here. `into` is a
// directory this program already owns, and the name comes from the phone's own
// basename rather than from anything a caller composed.
func (a ADB) Fetch(ctx context.Context, serial, remote, into string, say func(string)) (string, error) {
	if !serials.MatchString(serial) {
		return "", fmt.Errorf("that is not a device identifier")
	}
	// Only from where WhatsApp keeps them. Not a general file-transfer tool.
	if !known(remote) {
		return "", fmt.Errorf("that is not a WhatsApp backup on the phone")
	}

	local := path.Join(into, path.Base(remote))
	cmd := exec.CommandContext(ctx, a.Binary, "-s", serial, "pull", remote, local) //nolint:gosec // a fixed argument list, from a validated serial and a path checked against two constants
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("copying the backup off the phone: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if say != nil {
		say(strings.TrimSpace(string(out)))
	}
	return local, nil
}

// known reports whether a path is one of the two folders WhatsApp keeps backups in.
func known(remote string) bool {
	clean := path.Clean(remote)
	for _, dir := range databases {
		if path.Dir(clean) == dir && strings.HasPrefix(path.Base(clean), "msgstore") {
			return true
		}
	}
	return false
}

// property reads one of the phone's own settings. Empty when it will not say.
func (a ADB) property(ctx context.Context, serial, name string) string {
	out, err := a.read(ctx, "-s", serial, "shell", "getprop", name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// read runs one adb command and returns what it said.
//
// Every caller passes a fixed argument list. Nothing here takes a shell string,
// which is the whole reason there is no way to ask this package to delete anything.
func (a ADB) read(ctx context.Context, args ...string) (string, error) {
	if a.Binary == "" {
		return "", ErrNoADB
	}
	ctx, stop := context.WithTimeout(ctx, 20*time.Second)
	defer stop()

	out, err := exec.CommandContext(ctx, a.Binary, args...).Output() //nolint:gosec // a fixed argument list; see the package comment
	if err != nil {
		// Not the raw failure. adb reports a broken install as "signal: abort trap"
		// and a missing one as a path error, and neither is a sentence anybody can
		// act on — least of all somebody whose phone has just died. What they need
		// to know is that this way in is unavailable, which the screen then acts on
		// by not offering it.
		return "", fmt.Errorf("%w: %s", ErrUnusable, a.Binary)
	}
	return string(out), nil
}

// Media is the phone's folder of photographs, videos and recordings.
//
// A database records where every one of them was and holds at most a thumbnail, so
// this folder is the difference between an archive that shows one picture in eight
// and one that shows all of them. It is also, on a real device, 5.7 GB — which is
// why its size is asked for before anything is copied rather than after.
type Media struct {
	// Path is where it is on the phone.
	Path string `json:"path"`
	// Bytes is how much is in it, as the phone counts it. Zero when the phone would
	// not say, which is not a reason to refuse: it makes the wait unpredictable
	// rather than impossible.
	Bytes int64 `json:"bytes"`
	// Kinds are the folders inside it — WhatsApp Images, WhatsApp Voice Notes — in
	// the phone's own order. These are what get copied, one at a time.
	Kinds []string `json:"kinds"`
}

// Files finds the phone's WhatsApp folder and says how large it is.
//
// Nothing is copied here. A person about to wait twenty minutes for several
// gigabytes should be told that first, and a person whose phone keeps its files
// somewhere this does not know about should be told that instead of watching an
// empty progress bar.
func (a ADB) Files(ctx context.Context, serial string) (Media, error) {
	if !serials.MatchString(serial) {
		return Media{}, fmt.Errorf("that is not a device identifier")
	}

	for _, dir := range folders {
		// -1 so each name is a line of its own: these hold spaces, without exception.
		out, err := a.read(ctx, "-s", serial, "shell", "ls", "-1", dir)
		if err != nil || strings.Contains(out, "No such file") {
			continue
		}
		kinds := lines(out)
		if len(kinds) == 0 {
			continue
		}
		return Media{Path: dir, Bytes: a.sizeOf(ctx, serial, dir), Kinds: kinds}, nil
	}
	return Media{}, ErrNoMediaOnPhone
}

// ErrNoMediaOnPhone reports a phone with no WhatsApp folder where this looks.
//
// Not a failure of the phone or of this program: WhatsApp on a phone that has never
// received a picture has no such folder, and a phone that keeps it somewhere else
// is a shape worth hearing about rather than guessing at.
var ErrNoMediaOnPhone = errors.New("this phone has no WhatsApp media folder where Amberkeep looks")

// sizeOf asks the phone how much is in a folder. Zero when it will not say.
//
// `du` is not on every Android, and on the ones that have it a folder of five
// thousand files takes a moment. Both are survivable: the number is used to set
// expectations, and a missing one only makes the wait unpredictable.
func (a ADB) sizeOf(ctx context.Context, serial, dir string) int64 {
	ctx, stop := context.WithTimeout(ctx, 2*time.Minute)
	defer stop()

	out, err := exec.CommandContext(ctx, a.Binary, "-s", serial, "shell", "du", "-s", "-k", dir).Output() //nolint:gosec // a fixed argument list, from a validated serial and one of two constants
	if err != nil {
		return 0
	}
	blocks, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\t")
	kb, err := strconv.ParseInt(strings.TrimSpace(blocks), 10, 64)
	if err != nil {
		return 0
	}
	return kb * 1024
}

// FetchFiles copies the phone's WhatsApp folder into a directory here.
//
// One kind at a time — WhatsApp Images, then WhatsApp Video, and so on — rather than
// the whole folder in one call, for two reasons. A person watching gigabytes copy
// should be told which part is copying, and a copy that fails or is stopped halfway
// leaves the kinds that finished where they are: running it again skips them instead
// of starting five gigabytes over.
//
// What lands is `<into>/Media/...`, the shape the paths inside the database already
// use, so the folder is readable beside the archive with nothing else done to it.
func (a ADB) FetchFiles(ctx context.Context, serial, into string, say func(string, int, int)) (string, error) {
	found, err := a.Files(ctx, serial)
	if err != nil {
		return "", err
	}

	media := filepath.Join(into, "Media")
	if err := os.MkdirAll(media, 0o700); err != nil {
		return "", fmt.Errorf("making room for the files: %w", err)
	}

	for at, kind := range found.Kinds {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if say != nil {
			say(kind, at, len(found.Kinds))
		}
		// Already here from an earlier run, which is the whole point of copying one
		// kind at a time. adb pull would copy every byte again.
		if there, err := os.Stat(filepath.Join(media, kind)); err == nil && there.IsDir() {
			continue
		}
		// The name came off the phone's own listing, and it is joined to one of two
		// constant directories. Nothing a caller composed reaches this.
		remote := path.Join(found.Path, kind)
		cmd := exec.CommandContext(ctx, a.Binary, "-s", serial, "pull", remote, media) //nolint:gosec // a fixed argument list; see above
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("copying %s off the phone: %w: %s", kind, err, strings.TrimSpace(string(out)))
		}
	}
	return into, nil
}

// lines are the non-empty lines of some output, trimmed.
func lines(out string) []string {
	var found []string
	scan := bufio.NewScanner(strings.NewReader(out))
	for scan.Scan() {
		if line := strings.TrimSpace(scan.Text()); line != "" {
			found = append(found, line)
		}
	}
	return found
}
