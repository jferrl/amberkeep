package backupfs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// Putting a changed file back into a backup, without touching the backup.
//
// This is the last thing that happens before somebody restores, and it is the point
// at which a mistake stops being recoverable: what comes out of here is what a phone
// will be told is its own history. So it works entirely on a copy, changes exactly
// two things inside that copy, and checks the result before returning it.
//
// It does not restore anything. Putting the result onto a phone is Finder's job, and
// telling somebody how is the wizard's. Nothing in this package has ever spoken to a
// device and nothing in it is going to.

// Replacement is one file to put into the copy.
type Replacement struct {
	// Domain and RelativePath say which file, in Apple's own terms.
	Domain       string
	RelativePath string
	// With is the file whose contents should take its place.
	With string
}

// Patched is what was done.
type Patched struct {
	// Path is the copy. The original is where it was, as it was.
	Path string
	// Was and Now are the replaced file's size before and after.
	Was, Now int64
	// Files is how many entries the copy's index holds, which must be what the
	// original held: this adds nothing and removes nothing.
	Files int
	// LogNeutralised reports whether the backup carried a write-ahead log for the
	// replaced file, which had to be emptied so the phone would not replay it over
	// the contents just put there.
	LogNeutralised bool
	// Took is how long it ran, nearly all of it copying.
	Took time.Duration
}

// Patch writes a copy of a backup with one file replaced.
//
// from is opened read-only and never written to. into must not exist. What comes back
// is a backup Finder will restore exactly as it would restore the original, with one
// file's contents different and its recorded size and times to match.
func Patch(ctx context.Context, from, into string, replace Replacement) (Patched, error) {
	started := time.Now()

	// Refuses a folder that is not a backup, and one that is encrypted: an encrypted
	// backup's payloads are wrapped with a key this program is never given, so a file
	// put into one in the clear is a file the phone cannot read.
	archive, err := Open(ctx, from)
	if err != nil {
		return Patched{}, err
	}
	original, found, err := archive.Find(ctx, replace.Domain, replace.RelativePath)
	if err != nil {
		_ = archive.Close()
		return Patched{}, err
	}
	if !found {
		_ = archive.Close()
		return Patched{}, ErrFileNotFound.withCause(
			fmt.Errorf("%s %s", replace.Domain, replace.RelativePath))
	}
	log, hasLog, err := archive.Find(ctx, replace.Domain, replace.RelativePath+"-wal")
	if err != nil {
		_ = archive.Close()
		return Patched{}, err
	}
	before, err := countFiles(ctx, filepath.Join(from, "Manifest.db"))
	if err != nil {
		_ = archive.Close()
		return Patched{}, err
	}
	if err := archive.Close(); err != nil {
		return Patched{}, err
	}

	if err := usable(replace.With); err != nil {
		return Patched{}, err
	}
	if _, err := os.Stat(into); err == nil { // #nosec G703 -- the destination the caller named
		return Patched{}, ErrWouldOverwrite.withCause(errors.New(into))
	}

	if err := copyTree(ctx, from, into); err != nil {
		_ = os.RemoveAll(into) // #nosec G703 -- the half-made copy this call just made
		return Patched{}, err
	}

	result := Patched{Path: into, Files: before, Was: original.Size}
	if err := put(ctx, into, replace.With, original, &result); err != nil {
		_ = os.RemoveAll(into) // #nosec G703 -- the copy this call is abandoning
		return Patched{}, err
	}
	if hasLog {
		if err := empty(ctx, into, log, &result); err != nil {
			_ = os.RemoveAll(into) // #nosec G703
			return Patched{}, err
		}
	}
	if err := checkPatched(ctx, into, original, result); err != nil {
		_ = os.RemoveAll(into) // #nosec G703
		return Patched{}, err
	}

	result.Took = time.Since(started)
	return result, nil
}

// sqliteHeader is what every SQLite file begins with.
const sqliteHeader = "SQLite format 3\x00"

// usable refuses a replacement that is not finished.
//
// A database with a write-ahead log beside it has not been written out: the log holds
// changes the file does not, and only the file goes into the backup. Restoring that
// is restoring a database missing its most recent part, which is exactly the data
// somebody is doing this for.
func usable(path string) error {
	f, err := os.Open(path) // #nosec G304,G703 -- the file the caller asked to put in
	if err != nil {
		return ErrUnreadable.withCause(err)
	}
	defer func() { _ = f.Close() }()

	header := make([]byte, len(sqliteHeader))
	if _, err := io.ReadFull(f, header); err != nil || string(header) != sqliteHeader {
		return ErrNotADatabase.withCause(errors.New(filepath.Base(path)))
	}
	if info, err := os.Stat(path + "-wal"); err == nil && info.Size() > 0 { // #nosec G703
		return ErrUnfinished.withCause(errors.New(filepath.Base(path)))
	}
	return nil
}

// put replaces one payload inside the copy and records the new size in its index.
func put(ctx context.Context, into, from string, original File, result *Patched) error {
	payload, err := payloadPath(into, original)
	if err != nil {
		return err
	}
	if _, err := os.Stat(payload); err != nil { // #nosec G703 -- a path this package built
		return ErrUnreadable.withCause(fmt.Errorf("the copy does not hold %s: %w", original.RelativePath, err))
	}
	if err := copyFile(ctx, payload, from); err != nil {
		return err
	}

	// Readable by its owner and by the process that restores it, which is what every
	// other payload in a backup is.
	if err := os.Chmod(payload, 0o600); err != nil { // #nosec G703 -- a path this package built
		return ErrUnreadable.withCause(err)
	}
	info, err := os.Stat(payload) // #nosec G703 -- a path this package built
	if err != nil {
		return ErrUnreadable.withCause(err)
	}
	result.Now = info.Size()

	return record(ctx, into, original.id, result.Now)
}

// empty neutralises a write-ahead log the backup carries for the replaced file.
//
// A backup records the log as a file of its own. Left as it was, the phone opens the
// new contents and replays a log written against the old ones over the top, which is
// how a restore that reports success loses the thing it was done for. The log becomes
// an empty file, which is what SQLite treats as nothing to replay.
func empty(ctx context.Context, into string, log File, result *Patched) error {
	payload, err := payloadPath(into, log)
	if err != nil {
		return err
	}
	// Recorded in the index but not present on disk, which a real backup does: there
	// is nothing to neutralise, and that is not a failure.
	//nolint:nilerr // see above
	if _, err := os.Stat(payload); err != nil { // #nosec G703 -- a path this package built
		return nil
	}
	if log.Size == 0 {
		return nil
	}

	if err := os.WriteFile(payload, nil, 0o600); err != nil { // #nosec G703
		return ErrUnreadable.withCause(err)
	}
	if err := record(ctx, into, log.id, 0); err != nil {
		return err
	}
	result.LogNeutralised = true
	return nil
}

// payloadPath is where a backup keeps a file's contents: under the first two
// characters of its identifier, named by the whole of it.
func payloadPath(root string, f File) (string, error) {
	if len(f.id) < 2 {
		return "", ErrCorruptManifest.withCause(fmt.Errorf("file identifier %q is too short", f.id))
	}
	return filepath.Join(root, f.id[:2], f.id), nil
}

// record writes a file's new size into the copy's index.
func record(ctx context.Context, into, id string, size int64) error {
	manifest := filepath.Join(into, "Manifest.db")
	db, err := sql.Open("sqlite", "file:"+url.PathEscape(manifest)+"?_pragma=busy_timeout(10000)")
	if err != nil {
		return ErrCorruptManifest.withCause(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	var blob []byte
	if err := db.QueryRowContext(ctx, "SELECT file FROM Files WHERE fileID = :id",
		sql.Named("id", id)).Scan(&blob); err != nil {
		return ErrCorruptManifest.withCause(err)
	}
	patched, err := resize(blob, size, time.Now())
	if err != nil {
		return ErrCorruptManifest.withCause(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE Files SET file = :file WHERE fileID = :id",
		sql.Named("file", patched), sql.Named("id", id)); err != nil {
		return ErrCorruptManifest.withCause(err)
	}

	// The copy has to carry a single Manifest.db, as the original does. A backup with
	// a log beside its index is one Finder may read differently than intended.
	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return ErrCorruptManifest.withCause(err)
	}
	if err := db.Close(); err != nil {
		return ErrCorruptManifest.withCause(err)
	}
	for _, side := range []string{"-wal", "-shm"} {
		if err := os.Remove(manifest + side); err != nil && !os.IsNotExist(err) { // #nosec G703
			return ErrCorruptManifest.withCause(err)
		}
	}
	return nil
}

// countFiles is how many entries an index holds.
func countFiles(ctx context.Context, manifest string) (int, error) {
	db, err := sql.Open("sqlite", "file:"+url.PathEscape(manifest)+"?mode=ro&immutable=1")
	if err != nil {
		return 0, ErrCorruptManifest.withCause(err)
	}
	defer func() { _ = db.Close() }()

	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM Files").Scan(&n); err != nil {
		return 0, ErrCorruptManifest.withCause(err)
	}
	return n, nil
}

// checkPatched looks at what was produced before handing it back.
//
// Cheap, and the last chance anybody has. A backup that is wrong here is a backup
// somebody restores.
func checkPatched(ctx context.Context, into string, original File, result Patched) error {
	manifest := filepath.Join(into, "Manifest.db")
	db, err := sql.Open("sqlite", "file:"+url.PathEscape(manifest)+"?mode=ro&immutable=1")
	if err != nil {
		return ErrCorruptManifest.withCause(err)
	}
	defer func() { _ = db.Close() }()

	var said string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&said); err != nil || said != "ok" {
		return ErrCorruptManifest.withCause(fmt.Errorf("the copy's index is damaged: %s", said))
	}

	var after int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM Files").Scan(&after); err != nil {
		return ErrCorruptManifest.withCause(err)
	}
	if after != result.Files {
		return ErrCorruptManifest.withCause(
			fmt.Errorf("the copy's index holds %d files and the original held %d", after, result.Files))
	}

	var blob []byte
	if err := db.QueryRowContext(ctx, "SELECT file FROM Files WHERE fileID = :id",
		sql.Named("id", original.id)).Scan(&blob); err != nil {
		return ErrCorruptManifest.withCause(err)
	}
	recorded, err := parseMBFile(blob)
	if err != nil {
		return ErrCorruptManifest.withCause(err)
	}
	if recorded.Size != result.Now {
		return ErrCorruptManifest.withCause(fmt.Errorf(
			"the copy's index says %d bytes and the file is %d", recorded.Size, result.Now))
	}
	return nil
}

// copyTree copies a whole backup folder.
//
// Plainly, file by file. A backup is several gigabytes and Apple's own tooling would
// clone it on APFS in no time at all, which is worth doing later and is not worth
// getting wrong now: a copy that is merely slow is a copy.
func copyTree(ctx context.Context, from, into string) error {
	// #nosec G703 -- the backup folder the caller named, walked as it stands.
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return wrapFSError("reading "+path, err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		relative, err := filepath.Rel(from, path)
		if err != nil {
			return wrapFSError("reading "+path, err)
		}
		destination := filepath.Join(into, relative)

		switch {
		case entry.IsDir():
			return wrapFSError("creating "+destination, os.MkdirAll(destination, 0o700))
		case !entry.Type().IsRegular():
			// A backup holds ordinary files and directories. Anything else is not
			// something to reproduce blindly inside a folder about to be restored.
			return nil
		default:
			return copyFile(ctx, destination, path)
		}
	})
}
