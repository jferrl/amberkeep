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
	"os/exec"
	"path"
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
