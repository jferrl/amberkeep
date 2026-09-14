package backupfs

import "errors"

// Guidance identifiers, as described in the principles: every user-facing failure
// maps to an explanation rather than to raw text. Identifiers are part of the public
// contract: never renumber or reuse one.
const (
	GuidanceNotABackup       = "backupfs.not-a-backup"
	GuidanceEncrypted        = "backupfs.encrypted"
	GuidancePermissionDenied = "backupfs.permission-denied"
	GuidanceCorruptManifest  = "backupfs.corrupt-manifest"
	GuidanceFileNotFound     = "backupfs.file-not-found"
	GuidanceNotAFile         = "backupfs.not-a-file"
	GuidanceUnreadable       = "backupfs.unreadable"
	GuidanceWouldOverwrite   = "backupfs.would-overwrite"
	GuidanceNotADatabase     = "backupfs.not-a-database"
	GuidanceUnfinished       = "backupfs.unfinished"
)

// Error is what this package returns for problems a user can act on.
type Error struct {
	Guidance string
	msg      string
	err      error
}

func (e *Error) Error() string {
	if e.err != nil {
		return e.msg + ": " + e.err.Error()
	}
	return e.msg
}

func (e *Error) Unwrap() error { return e.err }

// Is matches on the guidance identifier, so a wrapped error still compares equal
// to the sentinel it came from.
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return e.Guidance == t.Guidance
}

func (e *Error) withCause(cause error) *Error {
	return &Error{Guidance: e.Guidance, msg: e.msg, err: cause}
}

var (
	// ErrNotABackup reports a folder that does not have the four files every Finder,
	// iTunes and Apple Devices backup has: Manifest.plist, Manifest.db, Info.plist and
	// Status.plist. Usually this means the wrong folder was chosen, or a backup that is
	// still being written.
	ErrNotABackup = &Error{Guidance: GuidanceNotABackup,
		msg: "this folder does not look like an iPhone backup"}

	// ErrEncrypted reports a backup made with encryption turned on.
	//
	// Decrypting one is out of scope by design, not an oversight. Apple derives the
	// encryption key from a password this package is never given, so reading an
	// encrypted backup would mean reimplementing Apple's key-derivation and keybag
	// format and carrying the user's backup password through this package, for a
	// problem Finder, iTunes and the Apple Devices app already solve on request: they
	// ask for the old password once and write a fresh, unencrypted backup.
	ErrEncrypted = &Error{Guidance: GuidanceEncrypted,
		msg: "this backup is encrypted; turn off encrypted backups in Finder, iTunes " +
			"or the Apple Devices app (it will ask for the current backup password), " +
			"then take a new backup"}

	// ErrPermissionDenied reports an EPERM/EACCES reading the backup, which on macOS
	// almost always means Full Disk Access has not been granted.
	ErrPermissionDenied = &Error{Guidance: GuidancePermissionDenied,
		msg: "the backup could not be read because of a permissions restriction; on " +
			"macOS, grant access in System Settings > Privacy & Security > Full Disk " +
			"Access, or copy the backup folder somewhere else first"}

	// ErrCorruptManifest reports a backup whose Manifest.plist, Info.plist or
	// Manifest.db exists but could not be parsed, which usually means the backup is
	// damaged or was copied incompletely.
	ErrCorruptManifest = &Error{Guidance: GuidanceCorruptManifest,
		msg: "this backup's manifest could not be read, which usually means the " +
			"backup is damaged or incomplete"}

	// ErrFileNotFound reports a domain and relative path this backup never recorded.
	// Apple only backs up what an app has asked to be backed up, so an app that was
	// never opened, or a chat with nothing in it yet, can legitimately be absent.
	ErrFileNotFound = &Error{Guidance: GuidanceFileNotFound,
		msg: "that file is not recorded in this backup"}

	// ErrNotAFile reports a domain and relative path that names a directory entry in
	// the manifest rather than a file; directories carry no bytes of their own to
	// extract.
	ErrNotAFile = &Error{Guidance: GuidanceNotAFile,
		msg: "that entry is a directory in the backup, not a file"}

	// ErrUnreadable reports a failure that is neither "not a backup" nor a permission
	// problem: a truncated database, an I/O error mid-copy, and the like.
	ErrUnreadable = &Error{Guidance: GuidanceUnreadable,
		msg: "the backup could not be read"}

	// ErrWouldOverwrite reports a destination that already holds something.
	//
	// What is most likely to be there is the last attempt, which is several gigabytes
	// somebody waited for and may still need to restore from.
	ErrWouldOverwrite = &Error{Guidance: GuidanceWouldOverwrite,
		msg: "there is already something at that path; choose somewhere new"}

	// ErrNotADatabase reports a replacement file that is not a SQLite database.
	ErrNotADatabase = &Error{Guidance: GuidanceNotADatabase,
		msg: "that file is not a database, so it is not something to put into a backup"}

	// ErrUnfinished reports a database whose write-ahead log has not been folded in.
	//
	// Only the database file goes into a backup, so the changes sitting in its log
	// would be left behind — and those are the most recent ones, which is exactly
	// what somebody is doing this for.
	ErrUnfinished = &Error{Guidance: GuidanceUnfinished,
		msg: "that database still has a write-ahead log beside it, so the most recent " +
			"changes are not in the file yet"}
)
