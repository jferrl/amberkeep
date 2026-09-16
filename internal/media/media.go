// Package media finds the files a message database refers to and does not contain.
//
// A WhatsApp database records where a photograph was on the phone —
// `Media/WhatsApp Images/IMG-20190614-WA0001.jpg` — and keeps at most a thumbnail of
// it. On a real device 92,941 of 99,041 attachments record such a path and 12,510
// have a thumbnail, so an archive read on its own can show about one picture in
// eight, and the rest of somebody's photographs are a line of text saying a
// photograph was sent.
//
// The files are still on the phone, in the folder WhatsApp keeps them in. This is
// what reads them when somebody brings that folder: the root is the directory
// holding `Media`, which is `Android/media/com.whatsapp/WhatsApp` on the phone, and
// everything under it is opened read-only and never written to.
//
// # Where a path is allowed to lead
//
// The paths come out of a database that came off somebody's phone, and this package
// turns them into files on the reader's own disk. A database carrying
// `../../../../etc/passwd` must therefore not produce /etc/passwd, and one carrying
// an absolute path must not produce that path: both are refused rather than clamped,
// because a path that has to be corrected before it is safe is a path nobody should
// be following.
package media

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrNowhere is that the archive has no files beside it, which is the ordinary
// state of a database copied off a phone on its own.
var ErrNowhere = errors.New("this archive has no files with it")

// ErrNotThere is that the file a message refers to is not in the folder. A folder
// copied from a phone is often partial, and a picture that is not there is a picture
// the archive says nothing about rather than a failure.
var ErrNotThere = errors.New("that file is not in this folder")

// Folder is a directory holding the files a message database refers to.
//
// The zero value is a folder with nothing in it: every lookup answers no, which is
// what an archive read without its files should do.
type Folder struct{ root string }

// In returns the files kept in a directory, and whether there are any.
//
// What is looked for is the `Media` directory WhatsApp keeps everything under, so a
// caller can hand over either the WhatsApp folder itself or the Media folder inside
// it — the two things somebody is equally likely to have copied — and either works.
func In(root string) (Folder, bool) {
	root = strings.TrimSpace(root)
	if root == "" {
		return Folder{}, false
	}
	root = filepath.Clean(root)

	if info, err := os.Stat(filepath.Join(root, "Media")); err == nil && info.IsDir() {
		return Folder{root: through(root)}, true
	}
	// Somebody who copied the Media folder itself rather than the one above it. The
	// recorded paths all begin with Media/, so the root is the parent.
	if filepath.Base(root) == "Media" {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return Folder{root: through(filepath.Dir(root))}, true
		}
	}
	return Folder{}, false
}

// through resolves a directory's own links, so that what is compared against later
// is the place itself rather than a name for it. Temporary directories on macOS are
// reached through one of these, so without it nothing would ever look inside.
func through(dir string) string {
	if followed, err := filepath.EvalSymlinks(dir); err == nil {
		return followed
	}
	return dir
}

// Empty reports whether there is nowhere to look.
func (f Folder) Empty() bool { return f.root == "" }

// Where is the directory this reads from, for a program that has to say so.
func (f Folder) Where() string { return f.root }

// Holds reports whether the file a message recorded is here.
//
// Checked rather than assumed, because a folder copied off a phone is routinely
// partial — somebody copies WhatsApp Images and not WhatsApp Video — and an archive
// that offers a picture it cannot produce is worse than one that offers nothing.
func (f Folder) Holds(recorded string) bool {
	at, ok := f.inside(recorded)
	if !ok {
		return false
	}
	info, err := os.Stat(at)
	return err == nil && info.Mode().IsRegular()
}

// Open returns the file and what kind of thing it is.
//
// Seekable, because what asks for these is an HTTP handler and a video somebody
// drags the middle of is a range request. A reader that could only go forwards would
// mean sending forty megabytes to play the last ten seconds.
func (f Folder) Open(recorded string) (io.ReadSeekCloser, string, error) {
	if f.Empty() {
		return nil, "", ErrNowhere
	}
	at, ok := f.inside(recorded)
	if !ok {
		return nil, "", ErrNotThere
	}

	// #nosec G304 -- the path was resolved against this folder and checked to be
	// inside it by resolve, which is the whole of that function's job.
	file, err := os.Open(at)
	if err != nil {
		return nil, "", ErrNotThere
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, "", ErrNotThere
	}
	return file, KindOf(at), nil
}

// Size is how large the file is, or zero when it is not here.
func (f Folder) Size(recorded string) int64 {
	at, ok := f.inside(recorded)
	if !ok {
		return 0
	}
	info, err := os.Stat(at)
	if err != nil || !info.Mode().IsRegular() {
		return 0
	}
	return info.Size()
}

// inside is the file this path leads to, once its links have been followed, and
// only when that is still inside the folder.
//
// The second check is not the same as the first. A path can be impeccable — no dots,
// no leading slash, squarely under the root — and still be a link to somebody's
// private key, and the operating system will open it without complaint. A folder
// copied off a phone has no business containing links at all, so one that leads out
// is refused rather than reasoned about.
func (f Folder) inside(recorded string) (string, bool) {
	at, ok := f.resolve(recorded)
	if !ok {
		return "", false
	}
	followed, err := filepath.EvalSymlinks(at)
	if err != nil {
		return "", false
	}
	if followed != f.root && !strings.HasPrefix(followed, f.root+string(filepath.Separator)) {
		return "", false
	}
	return followed, true
}

// resolve turns a path a phone recorded into a path on this disk, or says no.
func (f Folder) resolve(recorded string) (string, bool) {
	if f.root == "" {
		return "", false
	}

	// The database writes them with forward slashes whatever the phone was, and the
	// program may be reading them on Windows.
	recorded = strings.TrimSpace(strings.ReplaceAll(recorded, "\\", "/"))
	if recorded == "" || path.IsAbs(recorded) || strings.HasPrefix(recorded, "~") {
		return "", false
	}
	// A Windows path with a drive letter is absolute whatever path.IsAbs thinks.
	if len(recorded) > 1 && recorded[1] == ':' {
		return "", false
	}

	cleaned := path.Clean(recorded)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}

	at := filepath.Join(f.root, filepath.FromSlash(cleaned))
	// Belt and braces: Join cleans, so the check above should have caught anything
	// that climbs out. This is the one that would catch what it did not.
	if at != f.root && !strings.HasPrefix(at, f.root+string(filepath.Separator)) {
		return "", false
	}
	return at, true
}

// KindOf is what a file is, from its name.
//
// From the name rather than the bytes: the database records a media type of its own
// and this is only the fallback, the files number in the tens of thousands, and
// reading the first bytes of each to say what it is would be a disk's worth of work
// to learn what the extension already says.
func KindOf(name string) string {
	if kind := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); kind != "" {
		return kind
	}
	return "application/octet-stream"
}

// Count reports how many of the recorded paths are actually here, for a program
// that has to tell somebody what bringing the folder bought them.
func Count(f Folder, recorded []string) int {
	var found int
	for _, one := range recorded {
		if f.Holds(one) {
			found++
		}
	}
	return found
}

// ErrString is the sentence a caller shows when a folder was named and holds none of
// what the archive refers to.
func ErrString(f Folder) string {
	return fmt.Sprintf("none of the files this archive refers to are in %s", f.Where())
}
