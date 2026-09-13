package android

import (
	"errors"
	"strings"
	"testing"

	"github.com/jferrl/amberkeep/internal/model"
)

func TestKindOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sourceType int
		origin     int
		want       model.Kind
	}{
		{name: "text", sourceType: typeText, want: model.KindText},
		{name: "photo", sourceType: typeImage, want: model.KindImage},
		{name: "video", sourceType: typeVideo, want: model.KindVideo},
		{name: "an attached audio file", sourceType: typeAudio, want: model.KindAudio},
		{name: "a recorded voice message", sourceType: typeAudio, origin: originVoiceNote, want: model.KindVoice},
		{name: "one contact card", sourceType: typeContact, want: model.KindContact},
		{name: "several contact cards", sourceType: typeContactArray, want: model.KindContact},
		{name: "a place", sourceType: typeLocation, want: model.KindLocation},
		{name: "a place shared live", sourceType: typeLiveLocation, want: model.KindLocation},
		{name: "a notice", sourceType: typeSystem, want: model.KindSystem},
		{name: "a document", sourceType: typeDocument, want: model.KindDocument},
		{name: "a missed call", sourceType: typeMissedCall, want: model.KindCall},
		{name: "a call", sourceType: typeCall, want: model.KindCall},
		{name: "a call log entry", sourceType: typeCallLog, want: model.KindCall},
		{name: "an animation", sourceType: typeGIF, want: model.KindGIF},
		{name: "deleted by its sender", sourceType: typeDeleted, want: model.KindDeleted},
		{name: "deleted by an administrator", sourceType: typeDeletedAdmin, want: model.KindDeleted},
		{name: "a sticker", sourceType: typeSticker, want: model.KindSticker},
		{name: "a photo that could be opened once", sourceType: typeViewOnceImg, want: model.KindViewOnce},
		{name: "a video that could be opened once", sourceType: typeViewOnceVid, want: model.KindViewOnce},
		{name: "a poll", sourceType: typePoll, want: model.KindPoll},
		{name: "an event", sourceType: typeEvent, want: model.KindEvent},
		{
			// A code from a future WhatsApp release must not stop an archive from
			// opening; it is carried through as unrecognised so the schema report
			// can surface it.
			name:       "a code nobody has seen before",
			sourceType: 250,
			want:       model.KindUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := kindOf(tt.sourceType, tt.origin); got != tt.want {
				t.Errorf("kindOf(%d, %d) = %v, want %v", tt.sourceType, tt.origin, got, tt.want)
			}
		})
	}
}

func TestLayoutNames(t *testing.T) {
	t.Parallel()

	tests := map[layout]string{
		layoutModern:  "modern",
		layoutLegacy:  "legacy",
		layoutUnknown: "unknown",
		layout(99):    "unknown",
	}
	for l, want := range tests {
		if got := l.String(); got != want {
			t.Errorf("layout(%d).String() = %q, want %q", l, got, want)
		}
	}
}

func TestPlaceholders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "NULL"},
		{n: 1, want: "?"},
		{n: 3, want: "?,?,?"},
	}
	for _, tt := range tests {
		if got := placeholders(tt.n); got != tt.want {
			t.Errorf("placeholders(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}

	t.Run("arguments match the placeholders", func(t *testing.T) {
		t.Parallel()
		ids := []int64{1, 2, 3}
		args := asArgs(ids)
		if len(args) != len(ids) {
			t.Fatalf("asArgs() produced %d arguments for %d identifiers", len(args), len(ids))
		}
		if strings.Count(placeholders(len(ids)), "?") != len(args) {
			t.Error("the number of placeholders does not match the number of arguments")
		}
	})
}

func TestErrorBehaviour(t *testing.T) {
	t.Parallel()

	cause := errors.New("the underlying reason")

	t.Run("a bare error reads on its own", func(t *testing.T) {
		t.Parallel()
		if msg := ErrNotAMessageDatabase.Error(); msg == "" {
			t.Error("the message is empty")
		}
	})

	t.Run("a cause is appended and recoverable", func(t *testing.T) {
		t.Parallel()

		wrapped := ErrUnreadable.withCause(cause)
		if !strings.Contains(wrapped.Error(), cause.Error()) {
			t.Errorf("Error() = %q, want it to mention the cause", wrapped.Error())
		}
		if !errors.Is(wrapped, cause) {
			t.Error("the cause cannot be recovered with errors.Is")
		}
		if !errors.Is(errors.Unwrap(wrapped), cause) {
			t.Error("Unwrap() did not return the cause")
		}
	})

	t.Run("wrapping does not change which failure it is", func(t *testing.T) {
		t.Parallel()

		if !errors.Is(ErrLegacyUnsupported.withCause(cause), ErrLegacyUnsupported) {
			t.Error("a wrapped error no longer matches its sentinel")
		}
		if errors.Is(ErrLegacyUnsupported, ErrUnreadable) {
			t.Error("two different failures compare equal")
		}
		if errors.Is(ErrUnreadable, cause) {
			t.Error("an unrelated error compares equal")
		}
	})

	t.Run("the sentinels keep their guidance identifiers", func(t *testing.T) {
		t.Parallel()

		tests := map[*Error]string{
			ErrNotAMessageDatabase: GuidanceNotAMessageDatabase,
			ErrLegacyUnsupported:   GuidanceLegacyUnsupported,
			ErrUnreadable:          GuidanceUnreadable,
		}
		for err, want := range tests {
			if err.Guidance != want {
				t.Errorf("guidance = %q, want %q", err.Guidance, want)
			}
		}
	})
}
