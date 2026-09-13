package backupfs

import (
	"errors"
	"fmt"
	"testing"
)

// TestErrorFormatting protects the two shapes Error.Error() must produce: a bare
// message when there is no underlying cause, and the message plus cause when there
// is one, which is what every caller further up sees in a log line or a UI.
func TestErrorFormatting(t *testing.T) {
	t.Parallel()

	t.Run("without a cause", func(t *testing.T) {
		t.Parallel()
		e := &Error{Guidance: "test.bare", msg: "something went wrong"}
		if got, want := e.Error(), "something went wrong"; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
		if e.Unwrap() != nil {
			t.Error("Unwrap() is non-nil for an error with no cause")
		}
	})

	t.Run("with a cause", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("underlying failure")
		e := &Error{Guidance: "test.cause", msg: "something went wrong", err: cause}
		if got, want := e.Error(), "something went wrong: underlying failure"; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
		if !errors.Is(e.Unwrap(), cause) {
			t.Error("Unwrap() does not return the cause")
		}
	})
}

// TestErrorIsMatchesOnGuidanceNotIdentity protects errors.Is against the whole point
// of carrying a guidance identifier: a wrapped copy of a sentinel, with its own
// distinct cause, must still compare equal to that sentinel, while a different
// sentinel and a foreign error type must not.
func TestErrorIsMatchesOnGuidanceNotIdentity(t *testing.T) {
	t.Parallel()

	wrapped := ErrUnreadable.withCause(errors.New("disk exploded"))
	if !errors.Is(wrapped, ErrUnreadable) {
		t.Error("a wrapped Error does not match its own sentinel")
	}
	if errors.Is(wrapped, ErrNotABackup) {
		t.Error("a wrapped Error matched an unrelated sentinel")
	}
	if errors.Is(wrapped, errors.New("disk exploded")) {
		t.Error("a wrapped Error matched a plain error with the same text")
	}

	// Is must also work through fmt.Errorf's %w, the way real call sites wrap it.
	further := fmt.Errorf("extracting a file: %w", wrapped)
	if !errors.Is(further, ErrUnreadable) {
		t.Error("errors.Is does not see through an additional %w wrap")
	}
}

// TestFirstNonEmpty protects the small helper describeBackup uses to prefer a
// non-empty device name: the first non-empty argument wins, and when every argument
// is empty the result is empty too, rather than panicking on an empty slice.
func TestFirstNonEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "first is non-empty", values: []string{"a", "b"}, want: "a"},
		{name: "first is empty, second wins", values: []string{"", "b"}, want: "b"},
		{name: "all empty", values: []string{"", ""}, want: ""},
		{name: "no arguments at all", values: nil, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := firstNonEmpty(tt.values...); got != tt.want {
				t.Errorf("firstNonEmpty(%v) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}
