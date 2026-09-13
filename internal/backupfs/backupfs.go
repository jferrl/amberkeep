// Package backupfs locates and reads Apple iPhone backups made by Finder, iTunes or
// the Apple Devices app, so a caller can pull WhatsApp's ChatStorage.sqlite (and its
// -wal/-shm sidecars) out of one without knowing anything about Apple's on-disk
// layout.
//
// A backup is a folder, named by the device's UDID, holding four files that mark it
// as one — Manifest.plist, Manifest.db, Info.plist and Status.plist — plus every
// backed-up file's bytes stored under a content address: the first two hex
// characters of fileID name a shard directory, and fileID itself names the file
// inside it. Manifest.db is a SQLite database whose Files table maps each app's
// domain and relative path to that fileID and to an NSKeyedArchiver-encoded blob of
// metadata (see mbfile.go). None of this is guessed; it is documented behaviour of
// Apple's backup format that this package reads and never writes to.
//
// Every function here is read path only. The user's backup is opened read-only and
// is never modified; the one exception, folding a copy's write-ahead log into its
// main file inside ExtractDatabase, only ever touches the private copy a caller asked
// for, never the original. Encrypted backups are detected, not decrypted: see
// ErrEncrypted for why that is a deliberate scope decision.
package backupfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Backup describes one iPhone backup found in a canonical folder, without opening it.
// Everything here comes from Info.plist and Manifest.plist, which are small files
// read once; nothing about Manifest.db or the backed-up files themselves is touched
// until Open.
type Backup struct {
	// Path is the backup's own folder, named by the device's UDID. Pass it to Open.
	Path string
	// UDID is the device identifier the backup folder is named after.
	UDID string
	// DeviceName is what the phone was called at backup time: Info.plist's
	// "Device Name", falling back to "Display Name" when that is empty, which happens
	// for some Apple Devices app backups.
	DeviceName string
	// ProductType is Apple's internal model identifier, such as "iPhone14,2".
	ProductType string
	// IOSVersion is the OS version the phone was running when this backup was made.
	IOSVersion string
	// LastBackup is when this backup finished.
	LastBackup time.Time
	// Encrypted reports whether the backup was made with encryption turned on.
	// Backups lists an encrypted backup like any other; only Open refuses one.
	Encrypted bool
}

// canonicalDirsFunc points at the platform-specific implementation in
// locate_darwin.go, locate_windows.go or locate_other.go. Tests reassign it to a
// synthetic root instead of the real Application Support or Apple folder, which is
// the one piece of this package that genuinely cannot be exercised any other way: the
// real locations are fixed by the operating system, not passed as a parameter.
var canonicalDirsFunc = canonicalDirs

// Backups reports every backup found in the canonical folders for this platform,
// most recently backed up first. A missing canonical folder (no Apple software has
// ever run on this machine, or its backup feature has never been used) is not an
// error and yields no entries for that location.
//
// A permission error reading a canonical folder — on macOS, almost always missing
// Full Disk Access — is reported rather than swallowed, but scanning continues past
// it: on Windows there are two candidate folders, and one being unreadable should not
// hide backups found via the other. Check the returned error with errors.Is against
// ErrPermissionDenied even when the returned slice is non-empty.
func Backups() ([]Backup, error) {
	var (
		found []Backup
		errs  []error
	)

	for _, root := range canonicalDirsFunc() {
		entries, err := os.ReadDir(root)
		switch {
		case err == nil:
		case errors.Is(err, fs.ErrNotExist):
			continue
		default:
			errs = append(errs, wrapFSError(root, err))
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			b, err := describeBackup(filepath.Join(root, entry.Name()))
			if err != nil {
				// Not every subdirectory of a MobileSync root is a backup — Apple
				// Devices and iTunes also keep their own bookkeeping files there —
				// so one that does not parse as one is skipped rather than failing
				// the whole scan.
				continue
			}
			found = append(found, b)
		}
	}

	sort.Slice(found, func(i, j int) bool { return found[i].LastBackup.After(found[j].LastBackup) })
	return found, errors.Join(errs...)
}

// describeBackup reads one backup folder's metadata without opening Manifest.db. It
// is used both by Backups, which never opens the database at all, and by Open, which
// does so only after confirming the folder is a genuine, non-encrypted backup.
func describeBackup(dir string) (Backup, error) {
	// #nosec G703 -- the folder is the one the user named or one this package found
	// in a canonical location. Looking at it is the command.
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Backup{}, ErrNotABackup.withCause(err)
	case err != nil:
		return Backup{}, wrapFSError("opening "+dir, err)
	case !info.IsDir():
		return Backup{}, ErrNotABackup.withCause(errors.New(dir + " is not a directory"))
	}

	for _, name := range requiredBackupFiles {
		// #nosec G703 -- a fixed file name inside the folder above.
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return Backup{}, ErrNotABackup.withCause(errors.New("missing " + name))
			}
			return Backup{}, wrapFSError("checking "+name, err)
		}
	}

	manifest, err := readManifestPlist(filepath.Join(dir, "Manifest.plist"))
	if err != nil {
		return Backup{}, err
	}
	device, err := readInfoPlist(filepath.Join(dir, "Info.plist"))
	if err != nil {
		return Backup{}, err
	}

	return Backup{
		Path:        dir,
		UDID:        filepath.Base(dir),
		DeviceName:  firstNonEmpty(device.DeviceName, device.DisplayName),
		ProductType: device.ProductType,
		IOSVersion:  device.ProductVersion,
		LastBackup:  device.LastBackupDate,
		Encrypted:   manifest.IsEncrypted,
	}, nil
}

// firstNonEmpty returns the first non-empty string, or "" if all of them are.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
