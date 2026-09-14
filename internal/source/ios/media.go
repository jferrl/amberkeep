package ios

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The pictures an iPhone store does not contain.
//
// This is the largest difference between the two platforms and the one a reader
// notices. An Android database keeps a small copy of a photograph inside itself, so
// a decade-old archive still shows what was sent even though the file is long gone.
// An iPhone store keeps only a path: `Media/<conversation>/5/e/<name>.thumb`, and
// the bytes are a file in the backup.
//
// So an iPhone archive shows no photographs at all unless somebody goes and gets
// them. On a real device that is 9,422 pictures and 13.9 MB, which is nothing to
// carry and everything to a person reading their own history.
//
// The full-size files are there too, and are not fetched: the same device holds
// 5.7 GB of them, they are already in the phone's own gallery, and copying
// gigabytes to say what is already said helps nobody.

// Media supplies the files a store refers to but does not hold.
//
// It is an interface so that this package knows nothing about Apple's backup
// format. What it has is a path the store recorded; where that path leads is
// somebody else's business.
type Media interface {
	// ReadFile returns the contents of one file, named as the store names it.
	// A file that is not there is not an error: a backup can be incomplete, and a
	// picture that cannot be found is a picture the archive says nothing about.
	ReadFile(path string) ([]byte, bool)
}

// FolderMedia reads the files from a directory laid out as the store expects.
//
// That is what `amberkeep extract` produces: the store and the pictures beside it,
// in the shape the paths inside the store already use, so the folder is readable on
// its own long after the backup it came from is gone.
type FolderMedia struct {
	root string
}

// MediaIn returns the pictures kept in a directory, or nothing when there are none.
//
// A caller passes the directory holding the store. Having no pictures is the
// ordinary case for a store that was copied out on its own, and is not a failure.
func MediaIn(root string) (Media, bool) {
	for _, shape := range []string{filepath.Join(root, "Media"), root} {
		if info, err := os.Stat(shape); err == nil && info.IsDir() {
			return FolderMedia{root: root}, true
		}
	}
	return nil, false
}

// ReadFile returns one picture.
//
// The path comes from the database rather than from a person, and a database is
// untrusted input: one carrying `../../` in a path would otherwise read whatever it
// liked off the disk. So the path is cleaned and refused unless it stays inside the
// directory it is supposed to be in.
func (m FolderMedia) ReadFile(path string) ([]byte, bool) {
	clean := filepath.Clean(filepath.FromSlash(path))
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return nil, false
	}

	full := filepath.Join(m.root, clean)
	if !strings.HasPrefix(full, m.root+string(filepath.Separator)) {
		return nil, false
	}

	// A picture is tens of kilobytes. Anything enormous under a name the store
	// chose is not one, and reading it would be the bug rather than the point.
	info, err := os.Stat(full)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxPictureBytes {
		return nil, false
	}

	// #nosec G304 -- the path was cleaned and confined to the directory above.
	contents, err := os.ReadFile(full)
	if err != nil {
		return nil, false
	}
	return contents, true
}

// maxPictureBytes is the most a preview may be.
//
// The largest on a real device is well under a megabyte; four is room to spare and
// still small enough that a malformed store cannot make an archive enormous.
const maxPictureBytes = 4 << 20

// fsMedia adapts any file system, which is what the tests use.
type fsMedia struct{ files fs.FS }

// MediaFrom returns the pictures in a file system.
func MediaFrom(files fs.FS) Media { return fsMedia{files: files} }

// ReadFile returns one picture, refusing a path that leaves the file system or
// names something too large to be one.
func (m fsMedia) ReadFile(path string) ([]byte, bool) {
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if !fs.ValidPath(clean) {
		return nil, false
	}
	contents, err := fs.ReadFile(m.files, clean)
	if err != nil || len(contents) > maxPictureBytes {
		return nil, false
	}
	return contents, true
}
