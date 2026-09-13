package backupfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"howett.net/plist"
)

// requiredBackupFiles are what Finder, iTunes and the Apple Devices app always write
// for a finished backup. A folder missing any of these either is not a backup at all,
// or is one that is still in progress.
var requiredBackupFiles = []string{"Manifest.plist", "Manifest.db", "Info.plist", "Status.plist"}

// infoPlist is the subset of Info.plist this package needs. Apple's real file also
// carries the device's serial number, IMEI, ICCID and phone number; none of those are
// read here, because nothing in this package's API surfaces them and there is no
// reason to hold personal data this tool has no use for.
type infoPlist struct {
	DeviceName     string    `plist:"Device Name"`
	DisplayName    string    `plist:"Display Name"`
	ProductType    string    `plist:"Product Type"`
	ProductVersion string    `plist:"Product Version"`
	LastBackupDate time.Time `plist:"Last Backup Date"`
}

// manifestPlist is the subset of Manifest.plist this package needs. The real file
// also describes which applications were backed up and, for an encrypted backup,
// carries Apple's keybag; this package stops at IsEncrypted and never attempts to
// read the keybag, by design (see ErrEncrypted).
type manifestPlist struct {
	IsEncrypted bool `plist:"IsEncrypted"`
}

// readInfoPlist parses path as an Info.plist. Apple writes this file in binary or XML
// property-list format depending on the tool and OS version, and howett.net/plist
// reads either without the caller telling it which.
func readInfoPlist(path string) (infoPlist, error) {
	var info infoPlist
	if err := readPlist(path, &info); err != nil {
		return infoPlist{}, err
	}
	return info, nil
}

// readManifestPlist parses path as a Manifest.plist.
func readManifestPlist(path string) (manifestPlist, error) {
	var manifest manifestPlist
	if err := readPlist(path, &manifest); err != nil {
		return manifestPlist{}, err
	}
	return manifest, nil
}

// readPlist reads and decodes one property list file, translating both I/O and
// parse failures into this package's typed errors: the file is untrusted input, so a
// decode failure is reported as a corrupt manifest rather than propagated raw.
func readPlist(path string, v any) error {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is built from a caller-supplied or discovered backup directory, which is exactly what this package exists to read
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return ErrPermissionDenied.withCause(fmt.Errorf("%s: %w", path, err))
		}
		return ErrCorruptManifest.withCause(fmt.Errorf("%s: %w", path, err))
	}
	if _, err := plist.Unmarshal(data, v); err != nil {
		return ErrCorruptManifest.withCause(fmt.Errorf("%s: %w", path, err))
	}
	return nil
}

// wrapFSError classifies a filesystem error this package did not expect (a missing
// file already has its own handling): a permission problem gets its own guidance,
// because on macOS it almost always means Full Disk Access, and anything else is
// reported as unreadable.
func wrapFSError(context string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fs.ErrPermission) {
		return ErrPermissionDenied.withCause(fmt.Errorf("%s: %w", context, err))
	}
	return ErrUnreadable.withCause(fmt.Errorf("%s: %w", context, err))
}
