package backupfs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestDescribeBackupRejectsACorruptPlist protects describeBackup, and therefore Open,
// against a Manifest.plist or Info.plist that exists but is not a property list at
// all — a partially written or truncated backup, most likely — reporting it as a
// corrupt manifest rather than propagating a raw decode error.
func TestDescribeBackupRejectsACorruptPlist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file string
	}{
		{name: "Manifest.plist is not a plist", file: "Manifest.plist"},
		{name: "Info.plist is not a plist", file: "Info.plist"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := buildBackup(t)
			if err := os.WriteFile(filepath.Join(dir, tt.file), []byte("not a property list"), 0o600); err != nil {
				t.Fatalf("corrupting the fixture: %v", err)
			}

			_, err := describeBackup(dir)
			if !errors.Is(err, ErrCorruptManifest) {
				t.Fatalf("describeBackup() error = %v, want %v", err, ErrCorruptManifest)
			}
		})
	}
}

// TestDescribeBackupReportsAnUnreadableFile protects the Full Disk Access case: a
// marker file that exists but cannot be opened must be reported as a permission
// problem, not as "not a backup" or a generic failure, because the fix (grant access,
// or copy the backup elsewhere) is completely different from either of those.
func TestDescribeBackupReportsAnUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not simulate a permission denial on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root can read anything, so a permission denial can't be produced")
	}
	t.Parallel()

	dir := buildBackup(t)
	target := filepath.Join(dir, "Info.plist")
	if err := os.Chmod(target, 0o000); err != nil {
		t.Fatalf("revoking read access on the fixture: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(target, 0o600) })

	_, err := describeBackup(dir)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("describeBackup() error = %v, want %v", err, ErrPermissionDenied)
	}
}

// withCanonicalDirs points Backups at synthetic roots instead of the real
// Application Support or Apple folder, restoring the original when the test ends.
// canonicalDirsFunc exists for exactly this: the real canonical locations are fixed
// by the operating system, not passed as a parameter, so this is the one seam that
// lets Backups be exercised without touching this machine's actual MobileSync
// folder.
func withCanonicalDirs(t *testing.T, dirs ...string) {
	t.Helper()
	original := canonicalDirsFunc
	canonicalDirsFunc = func() []string { return dirs }
	t.Cleanup(func() { canonicalDirsFunc = original })
}

// TestBackupsFindsBackupsAndSkipsEverythingElse protects the scan Backups does over a
// canonical root: a genuine backup is found and described, an encrypted one is found
// too (only Open refuses those), and a subdirectory that is not a backup at all —
// which is normal; iTunes and the Apple Devices app keep other bookkeeping in the
// same parent folder — is skipped rather than failing the whole scan.
func TestBackupsFindsBackupsAndSkipsEverythingElse(t *testing.T) {
	root := t.TempDir()

	plainDir := filepath.Join(root, "00008030-001A2D3E1E28002E")
	if err := os.MkdirAll(plainDir, 0o700); err != nil {
		t.Fatalf("creating a fixture backup directory: %v", err)
	}
	buildBackupIn(t, plainDir)

	encDir := filepath.Join(root, "11112222333344445555666677778888")
	if err := os.MkdirAll(encDir, 0o700); err != nil {
		t.Fatalf("creating a fixture backup directory: %v", err)
	}
	buildBackupIn(t, encDir, withEncrypted())

	if err := os.MkdirAll(filepath.Join(root, "Locking"), 0o700); err != nil {
		t.Fatalf("creating a non-backup fixture directory: %v", err)
	}

	withCanonicalDirs(t, root)

	backups, err := Backups()
	if err != nil {
		t.Fatalf("Backups() unexpected error: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("Backups() found %d entries, want 2 (got %+v)", len(backups), backups)
	}

	byUDID := make(map[string]Backup, len(backups))
	for _, b := range backups {
		byUDID[b.UDID] = b
	}
	plain, ok := byUDID[filepath.Base(plainDir)]
	if !ok {
		t.Fatal("Backups() did not include the plain backup")
	}
	if plain.Encrypted {
		t.Error("the plain backup was reported as encrypted")
	}
	if plain.DeviceName == "" {
		t.Error("the plain backup has no DeviceName")
	}

	enc, ok := byUDID[filepath.Base(encDir)]
	if !ok {
		t.Fatal("Backups() did not include the encrypted backup")
	}
	if !enc.Encrypted {
		t.Error("the encrypted backup was not reported as encrypted; Backups must list it, only Open refuses it")
	}
}

// TestBackupsToleratesAMissingCanonicalRoot protects the common case where a
// canonical folder simply does not exist: no Apple software has ever run on this
// machine, or its backup feature has never been used. That is not an error.
func TestBackupsToleratesAMissingCanonicalRoot(t *testing.T) {
	withCanonicalDirs(t, filepath.Join(t.TempDir(), "does-not-exist"))

	backups, err := Backups()
	if err != nil {
		t.Fatalf("Backups() unexpected error: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("Backups() = %d entries, want 0", len(backups))
	}
}

// TestBackupsReportsPermissionDeniedButKeepsScanning protects the Windows case with
// two canonical roots: one being unreadable must not hide backups found via the
// other, but the permission problem must still be reported so a caller can act on it
// (see ErrPermissionDenied). Unreliable to simulate as root, and on Windows chmod
// does not model a real access denial, so both are skipped there.
func TestBackupsReportsPermissionDeniedButKeepsScanning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not simulate a permission denial on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root can read anything, so a permission denial can't be produced")
	}

	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.Mkdir(blocked, 0o000); err != nil {
		t.Fatalf("creating an unreadable fixture directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })

	readableRoot := t.TempDir()
	okDir := filepath.Join(readableRoot, "00008030-001A2D3E1E28002E")
	if err := os.MkdirAll(okDir, 0o700); err != nil {
		t.Fatalf("creating a fixture backup directory: %v", err)
	}
	buildBackupIn(t, okDir)

	withCanonicalDirs(t, blocked, readableRoot)

	backups, err := Backups()
	if len(backups) != 1 {
		t.Fatalf("Backups() found %d entries, want 1 from the readable root", len(backups))
	}
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("Backups() error = %v, want %v", err, ErrPermissionDenied)
	}
}

// TestDescribeBackupRejectsWhatIsNotOne protects Open (which calls describeBackup
// first) against the folders it must refuse: one missing the four marker files
// entirely, and one that exists but is a plain file rather than a directory.
func TestDescribeBackupRejectsWhatIsNotOne(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func(t *testing.T) string
	}{
		{
			name:  "a folder that does not exist",
			build: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent") },
		},
		{
			name: "an empty folder",
			build: func(t *testing.T) string {
				return t.TempDir()
			},
		},
		{
			name: "a path that is a file, not a directory",
			build: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "notadir")
				if err := os.WriteFile(path, []byte("hi"), 0o600); err != nil {
					t.Fatalf("writing the fixture: %v", err)
				}
				return path
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := describeBackup(tt.build(t))
			if !errors.Is(err, ErrNotABackup) {
				t.Fatalf("describeBackup() error = %v, want %v", err, ErrNotABackup)
			}
		})
	}
}
