// Package export writes an archive out in formats people can read, search and
// keep: plain text, structured data, and a self-contained web page.
//
// Everything here streams. A single conversation in a real archive reaches ninety
// thousand messages, so nothing is collected in memory before being written.
//
// Files are written atomically, to a temporary name alongside the target and then
// moved into place, so an interrupted export never leaves a half-written archive
// that looks complete. Nothing is overwritten unless the caller says so.
package export

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/jferrl/amberkeep/internal/model"
)

// Options say how an archive should be written.
type Options struct {
	// Directory is where the files go. It is created if it does not exist.
	Directory string

	// Names resolves addresses to people. Without it an archive is a list of phone
	// numbers, so a caller should load whatever address book the user has first.
	Names *model.Directory

	// Location is the time zone timestamps are shown in. Messages are stored in
	// universal time, and reading them back in the wrong zone silently shifts every
	// conversation by hours. Defaults to the machine's own zone.
	Location *time.Location

	// Me is what to call the archive's owner. The owner's own name is often not
	// recorded anywhere, and every language has a different word for it, so the
	// caller supplies one.
	Me string

	// Words are the exported page's own chrome — its heading, the noun for a
	// conversation, and what the search box says before anything is typed. Supplied
	// by the caller for the same reason Me is: every language has different ones,
	// and this package has no business holding a catalogue. English when absent.
	Words Words

	// NoticeIdentified answers whether a system notice's action code is one this
	// build understands, so an archive can admit what it could not phrase instead
	// of leaving a consumer to guess. The reader that produced the archive knows;
	// this package does not, and asking it directly would invert the dependency.
	NoticeIdentified func(action int) bool

	// IncludeNotices writes the things WhatsApp did as well as the things people
	// said: who joined a group, when a security code changed. They are left out by
	// default because they add noise to a conversation without adding anybody's words.
	IncludeNotices bool

	// Overwrite permits replacing files that already exist. Without it an export
	// into a directory that already holds one stops rather than destroying it.
	Overwrite bool
}

// withDefaults fills in what the caller left out.
func (o Options) withDefaults() Options {
	if o.Location == nil {
		o.Location = time.Local
	}
	if o.Me == "" {
		o.Me = "You"
	}
	if o.NoticeIdentified == nil {
		// Without an answer, claim nothing.
		o.NoticeIdentified = func(int) bool { return false }
	}
	return o
}

// writeBuffer is how much of a file is held before it reaches the disk.
//
// The default is four kilobytes, which on a large export costs a third of the time
// in write calls that do almost nothing each. Sixty-four is where the gain flattens
// out and is a rounding error against the file being written.
const writeBuffer = 1 << 16

// ErrExists reports that a file is already there and Overwrite was not set.
var ErrExists = errors.New("the file already exists")

// Words are the exported page's own few words.
type Words struct {
	Title       string
	Noun        string
	Placeholder string
}

// Or fills in English for anything the caller did not say.
func (w Words) Or() Words {
	if w.Title == "" {
		w.Title = "Archive"
	}
	if w.Noun == "" {
		w.Noun = "conversations"
	}
	if w.Placeholder == "" {
		w.Placeholder = "Search for a person or group"
	}
	return w
}

// Conversation is one chat and the messages that belong to it.
//
// Messages arrive as a function rather than a slice because a conversation is far
// too large to hold: the writers pull them through one at a time.
type Conversation struct {
	Chat model.Chat
	// Messages yields the conversation in order. It may be called more than once,
	// and must produce the same messages each time.
	Messages func(yield func(model.Message, error) bool)
}

// Result counts what an export produced, for telling the user what happened.
type Result struct {
	Conversations int
	Messages      int
	Skipped       int // notices and hidden rows left out
	Files         []string
	Bytes         int64
}

// atomicWrite creates a file, hands it to write, and moves it into place only if
// that succeeded.
//
// The temporary file sits in the same directory as the target so the move is a
// rename rather than a copy across filesystems, which is what makes it atomic.
func atomicWrite(path string, overwrite bool, write func(io.Writer) error) (int64, error) {
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return 0, fmt.Errorf("%s: %w", filepath.Base(path), ErrExists)
		}
	}
	// An archive is somebody's private history, so the directory holding it is
	// theirs alone rather than world readable.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return 0, fmt.Errorf("preparing the export directory: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".amberkeep-*")
	if err != nil {
		return 0, fmt.Errorf("creating the export file: %w", err)
	}
	tmpName := tmp.Name()
	// An error anywhere below leaves nothing behind.
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	counter := &countingWriter{w: tmp}
	if err := write(counter); err != nil {
		return 0, err
	}
	if err := tmp.Sync(); err != nil {
		return 0, fmt.Errorf("writing the export file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("writing the export file: %w", err)
	}
	// An archive is somebody's private history; it should not be world readable.
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return 0, fmt.Errorf("securing the export file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return 0, fmt.Errorf("finishing the export file: %w", err)
	}
	return counter.n, nil
}

// countingWriter records how much was written, so a result can report it.
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// FileName is what to call the file holding one conversation.
//
// It has to survive being copied between a Mac, a Windows machine and a phone, so
// it avoids every character any of them reserves. The conversation's address is
// appended because two people can share a name, and two groups certainly can.
func FileName(chat model.Chat, extension string) string {
	name := sanitize(chat.Title())
	if name == "" {
		name = "conversation"
	}

	// The address disambiguates, but only the part that identifies somebody: the
	// server half is the same for everybody and wastes room in the name.
	suffix := sanitize(chat.JID.User)
	if suffix != "" && !strings.Contains(name, suffix) {
		name = name + " (" + suffix + ")"
	}
	return trimToLength(name, 120) + extension
}

// reservedNames are the file names Windows refuses to create, whatever the
// extension. A conversation with somebody saved as "CON" would otherwise fail to
// export on one platform and not the other.
var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// sanitize turns anything into something every filesystem accepts.
func sanitize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r < 0x20 || r == 0x7f:
			// Control characters, including the direction marks WhatsApp scatters
			// through its own exports.
		case strings.ContainsRune(`<>:"/\|?*`, r):
			b.WriteRune('-')
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}

	// A name may not end in a dot or a space on Windows, and leading dots hide the
	// file on everything else.
	out := strings.Trim(strings.Join(strings.Fields(b.String()), " "), ". ")
	if reservedNames[strings.ToUpper(out)] {
		out += "_"
	}
	return out
}

// trimToLength shortens a name without splitting a character in half.
func trimToLength(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	runes := []rune(s)
	for len(string(runes)) > limit {
		runes = runes[:len(runes)-1]
	}
	return strings.TrimRight(string(runes), ". ")
}
