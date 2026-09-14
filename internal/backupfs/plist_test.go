package backupfs

import (
	"errors"
	"io/fs"
	"testing"
)

// TestBlameSaysWhereOnce covers the sentence a macOS user meets on first run.
//
// Full Disk Access is not granted until somebody grants it, so the permission
// failure is the most-read error this package produces, and it used to read
// "/long/path: open /long/path: operation not permitted".
func TestBlameSaysWhereOnce(t *testing.T) {
	t.Parallel()

	// fs.ErrPermission rather than syscall.EPERM: the constant does not exist on
	// every platform this is built for, and what is under test is the shape of the
	// sentence, not which errno produced it.
	const path = "/Users/someone/Library/Application Support/MobileSync/Backup"
	denied := &fs.PathError{Op: "open", Path: path, Err: fs.ErrPermission}

	tests := []struct {
		name    string
		context string
		err     error
		want    string
	}{
		{
			name:    "a context that is the path does not repeat it",
			context: path,
			err:     denied,
			want:    path + ": permission denied",
		},
		{
			name:    "nor does one that names the path in a phrase",
			context: "opening " + path,
			err:     denied,
			want:    "opening " + path + ": permission denied",
		},
		{
			// Here the path is the only thing saying which file, so it stays.
			name:    "a context that names no path keeps the one in the error",
			context: "clearing an unfinished copy",
			err:     denied,
			want:    "clearing an unfinished copy: open " + path + ": permission denied",
		},
		{
			name:    "an error that is not about a path is left alone",
			context: "putting the finished copy in place",
			err:     errors.New("invalid cross-device link"),
			want:    "putting the finished copy in place: invalid cross-device link",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := blame(tt.context, tt.err)
			if got.Error() != tt.want {
				t.Errorf("blame() = %q, want %q", got.Error(), tt.want)
			}
			// Whatever it says, the original has to stay reachable: callers match on
			// fs.ErrPermission to decide what advice to give.
			if !errors.Is(got, tt.err) && !errors.Is(got, fs.ErrPermission) {
				t.Error("the cause can no longer be matched")
			}
		})
	}
}
