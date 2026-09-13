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

	_ "modernc.org/sqlite" // registers the pure-Go SQLite driver
)

// File is one entry from a backup's manifest: enough to locate and extract the bytes
// Apple stored for it, without a caller needing to know the on-disk layout.
type File struct {
	// Domain is the app or system component this file belongs to, such as
	// "AppDomainGroup-group.net.whatsapp.WhatsApp.shared".
	Domain string
	// RelativePath is the file's path within Domain, exactly as Apple recorded it.
	RelativePath string
	// Size is the file's size in bytes, read from its archived MBFile metadata.
	// Always 0 for a directory entry.
	Size int64
	// IsDir reports whether this entry is a directory rather than a file. A
	// directory carries no bytes of its own and cannot be extracted.
	IsDir bool

	// id is Manifest.db's fileID for this entry: the lowercase hex SHA-1 that both
	// keys the Files table and names the file's shard directory and filename on disk.
	id string
}

// Archive is one open backup folder. It embeds Backup, so a caller reads the device
// name, product type, iOS version and last-backup date straight off it (a.DeviceName,
// and so on) without a separate accessor.
type Archive struct {
	Backup
	db *sql.DB
}

// Open opens the backup folder at dir: a folder containing Manifest.plist,
// Manifest.db, Info.plist and Status.plist, as Finder, iTunes and the Apple Devices
// app always write. It refuses a folder that is not a backup (ErrNotABackup) and one
// that is encrypted (ErrEncrypted).
//
// The backup is opened read-only; nothing this package does writes to it. The caller
// closes the returned Archive.
func Open(ctx context.Context, dir string) (*Archive, error) {
	backup, err := describeBackup(dir)
	if err != nil {
		return nil, err
	}
	if backup.Encrypted {
		return nil, ErrEncrypted
	}

	dbPath := filepath.Join(dir, "Manifest.db")
	// query_only is belt and braces alongside the read-only mode: neither this code
	// nor the driver's own bookkeeping may write to a backup we were handed.
	dsn := "file:" + url.PathEscape(dbPath) + "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)" +
		manifestMode(dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, ErrUnreadable.withCause(err)
	}
	// One connection is enough for a single-user desktop tool and keeps the read-only
	// pragmas from having to be reapplied per connection.
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		if errors.Is(err, fs.ErrPermission) {
			return nil, ErrPermissionDenied.withCause(err)
		}
		return nil, ErrUnreadable.withCause(err)
	}
	if err := verifyFilesTable(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Archive{Backup: backup, db: db}, nil
}

// manifestMode decides how to open Manifest.db without leaving anything behind.
//
// Read-only is not enough on its own. SQLite creates a -wal and a -shm beside any
// database it opens in write-ahead-log mode, read-only or not, and Apple writes
// this one in that mode. Two files then appear inside somebody's backup that were
// not there before, which is a change to a backup however harmless, and this
// package promises not to make one. A real backup is what found it: every synthetic
// fixture used the default journal mode, where the problem does not arise.
//
// immutable tells SQLite the file cannot change underneath it, so it skips the
// write-ahead machinery entirely and creates nothing. That is only safe when there
// is no write-ahead log to read, because immutable ignores one: a backup Apple
// finished writing has none, and one that does is opened the ordinary way rather
// than read wrongly. Whatever is left behind in that case is the lesser harm.
func manifestMode(path string) string {
	// #nosec G703 -- the path is the backup folder the user named, which is the
	// whole point of the command; nothing here is fetched from elsewhere.
	if info, err := os.Stat(path + "-wal"); err == nil && info.Size() > 0 {
		return ""
	}
	return "&immutable=1"
}

// Close releases Manifest.db.
func (a *Archive) Close() error {
	if err := a.db.Close(); err != nil {
		return ErrUnreadable.withCause(err)
	}
	return nil
}

// Find locates one file by its domain and relative path, exactly as Apple recorded
// them, such as domain "AppDomainGroup-group.net.whatsapp.WhatsApp.shared" and
// relativePath "ChatStorage.sqlite". ok is false when the backup has no such entry,
// which happens whenever the app in question was never opened on the phone, or that
// particular file never existed; that is not an error.
//
// Find takes a context and can itself return an error, unlike a plain map lookup,
// because it queries Manifest.db — an I/O boundary, per this project's principles.
// Only ok == false with err == nil means "not present, and nothing went wrong
// finding that out".
func (a *Archive) Find(ctx context.Context, domain, relativePath string) (File, bool, error) {
	return lookupFile(ctx, a.db, domain, relativePath)
}

// Domain lists every file recorded under one domain, such as
// "AppDomainGroup-group.net.whatsapp.WhatsApp.shared". Only rows for that domain are
// fetched from Manifest.db, never the whole Files table, which can hold entries for
// hundreds of thousands of files across every app on the phone.
func (a *Archive) Domain(ctx context.Context, domain string) ([]File, error) {
	return listFilesInDomain(ctx, a.db, domain)
}

// pathOf returns where f's bytes live on disk inside this backup.
func (a *Archive) pathOf(f File) (string, error) {
	if len(f.id) < 2 {
		return "", ErrCorruptManifest.withCause(
			fmt.Errorf("%s %s has a malformed fileID %q", f.Domain, f.RelativePath, f.id))
	}
	return filepath.Join(a.Path, f.id[:2], f.id), nil
}

// Extract copies one file's bytes out of the backup to dst, which is created or
// truncated. The backup is only ever opened read-only for this; dst is the only file
// written.
//
// The copy is streamed through a bounded buffer rather than read into memory at once:
// a backed-up file — a WhatsApp media attachment, say — can be large enough that
// holding the whole thing in memory would be wasteful for what is fundamentally a
// copy.
func (a *Archive) Extract(ctx context.Context, dst string, f File) error {
	if f.IsDir {
		return ErrNotAFile.withCause(fmt.Errorf("%s %s", f.Domain, f.RelativePath))
	}
	src, err := a.pathOf(f)
	if err != nil {
		return err
	}
	return copyFile(ctx, dst, src)
}

// copyFile streams src to dst through a fixed-size buffer, so memory use does not
// grow with the size of the file being copied. src is opened read-only; dst is
// created or truncated, and is the only file this ever opens for writing.
func copyFile(ctx context.Context, dst, src string) error {
	in, err := os.Open(src) //nolint:gosec // G304: src is a path this package computed from the backup it was asked to read
	if err != nil {
		return wrapFSError("opening "+src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // G304: dst is the caller's own extraction target, not attacker input
	if err != nil {
		return wrapFSError("creating "+dst, err)
	}

	// A fixed buffer, rather than plain io.Copy, is what keeps this bounded: without
	// it, io.Copy would still avoid reading the whole file into one slice, but the
	// buffer size then depends on whichever fast path the two *os.File values happen
	// to take, which is not something this package wants to leave implicit.
	buf := make([]byte, 256*1024)
	if _, err := io.CopyBuffer(out, ctxReader{ctx: ctx, r: in}, buf); err != nil {
		_ = out.Close()
		return wrapFSError("copying "+src, err)
	}
	if err := out.Close(); err != nil {
		return wrapFSError("closing "+dst, err)
	}
	return nil
}

// ctxReader makes a plain io.Reader stop with the context's error once it is done,
// rather than running a large copy to completion after its caller has already given
// up on it.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// walSuffixes are the sidecar files SQLite keeps next to a database that is using a
// write-ahead log: the log itself, and the shared-memory index into it.
var walSuffixes = [...]string{"-wal", "-shm"}

// ExtractDatabase copies a SQLite database, and any -wal/-shm sidecars recorded for
// it, out of the backup and into dstDir, checkpoints the copy so its write-ahead log
// is folded into the main file, and returns the path to that main file.
//
// A copy is made rather than reading the backup in place for two reasons. First,
// folding a write-ahead log into its main file is itself a write, and the user's
// backup is opened read-only everywhere in this package — only the copy in dstDir is
// ever opened read-write, and only inside this function. Second, SQLite finds a
// -wal/-shm sidecar by looking next to the main file by name, but a backup stores
// every file at a content-addressed path with no such relationship visible on disk;
// extracting only ChatStorage.sqlite and leaving its -wal behind would silently drop
// whatever had been written since the last checkpoint, which for a chat database in
// daily use is often the most recent messages.
func (a *Archive) ExtractDatabase(ctx context.Context, dstDir, domain, relativePath string) (string, error) {
	main, ok, err := a.Find(ctx, domain, relativePath)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrFileNotFound.withCause(fmt.Errorf("%s %s", domain, relativePath))
	}
	if main.IsDir {
		return "", ErrNotAFile.withCause(fmt.Errorf("%s %s", domain, relativePath))
	}

	if err := os.MkdirAll(dstDir, 0o700); err != nil {
		return "", wrapFSError("creating "+dstDir, err)
	}

	dstMain := filepath.Join(dstDir, filepath.Base(relativePath))
	if err := a.Extract(ctx, dstMain, main); err != nil {
		return "", err
	}

	for _, suffix := range walSuffixes {
		side, ok, err := a.Find(ctx, domain, relativePath+suffix)
		if err != nil {
			return "", err
		}
		if !ok {
			continue // not every database has a pending write-ahead log to carry over
		}
		if err := a.Extract(ctx, dstMain+suffix, side); err != nil {
			return "", err
		}
	}

	if err := checkpointDatabase(ctx, dstMain); err != nil {
		return "", err
	}
	return dstMain, nil
}

// checkpointDatabase folds a database's write-ahead log back into its main file, in
// place, so the extracted copy can be opened read-only afterwards without losing
// whatever had not yet been checkpointed at backup time.
//
// path must always be the private copy ExtractDatabase just made, never the backup
// itself: this is the one place in this package that opens a file read-write.
func checkpointDatabase(ctx context.Context, path string) error {
	dsn := "file:" + url.PathEscape(path) + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("opening the extracted copy: %w", err))
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("opening the extracted copy: %w", err))
	}
	// TRUNCATE both checkpoints the log into the main file and removes the -wal file
	// afterwards, so the copy behaves like an ordinary, complete SQLite file rather
	// than one that still depends on a sidecar to be read in full.
	if _, err := db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return ErrUnreadable.withCause(fmt.Errorf("checkpointing the extracted copy: %w", err))
	}
	return nil
}
