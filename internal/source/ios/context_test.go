package ios

import (
	"strings"
	"testing"
)

// TestDecodeContext covers the recovery that public tools miss, and the refusals
// that keep a field meaning something else in a future release from being believed.
func TestDecodeContext(t *testing.T) {
	t.Parallel()

	const (
		stanza = "3EB0ABCD1234567890AB"
		sender = "34600111222@s.whatsapp.net"
	)

	tests := []struct {
		name  string
		blob  []byte
		want  messageContext
		empty bool
	}{
		{
			name: "a reply, whole",
			blob: replyContext(stanza, sender, "are you up?"),
			want: messageContext{QuotedID: stanza, QuotedSender: sender, QuotedText: "are you up?"},
		},
		{
			name: "a reply in a one-to-one conversation names no sender",
			blob: appendBytes(appendString(nil, fieldQuotedID, stanza),
				fieldQuotedBody, appendString(nil, fieldQuotedText, "hola")),
			want: messageContext{QuotedID: stanza, QuotedText: "hola"},
		},
		{
			name: "what was answered is kept even when it carried no words",
			blob: appendString(nil, fieldQuotedID, stanza),
			want: messageContext{QuotedID: stanza},
		},
		{
			name:  "a message that is not a reply",
			blob:  appendString(nil, 68, strings.Repeat("k", 32)),
			empty: true,
		},
		{
			name:  "nothing at all",
			blob:  nil,
			empty: true,
		},
		{
			name:  "a number where the identifier should be is not an identifier",
			blob:  appendString(nil, fieldQuotedID, "12"),
			empty: true,
		},
		{
			name:  "a sentence where the identifier should be is refused",
			blob:  appendString(nil, fieldQuotedID, "this is not an identifier at all"),
			empty: true,
		},
		{
			name: "a sentence where the address should be is refused",
			blob: appendBytes(appendString(nil, fieldQuotedID, stanza),
				fieldQuotedSender, []byte("not an address")),
			want: messageContext{QuotedID: stanza},
		},
		{
			name: "text that is not text is refused",
			blob: appendBytes(appendString(nil, fieldQuotedID, stanza),
				fieldQuotedBody, appendBytes(nil, fieldQuotedText, []byte{0xff, 0xfe, 0xfd})),
			want: messageContext{QuotedID: stanza},
		},
		{
			name: "fields this build does not know are skipped, not mistaken for others",
			blob: func() []byte {
				var b []byte
				b = appendVarint(b, 17<<3) // a number, wire type zero
				b = appendVarint(b, 42)
				b = appendString(b, fieldQuotedID, stanza)
				b = appendVarint(b, 50<<3|5) // four fixed bytes
				b = append(b, 1, 2, 3, 4)
				b = appendVarint(b, 51<<3|1) // eight fixed bytes
				b = append(b, 1, 2, 3, 4, 5, 6, 7, 8)
				b = appendBytes(b, fieldQuotedBody, appendString(nil, fieldQuotedText, "after"))
				return b
			}(),
			want: messageContext{QuotedID: stanza, QuotedText: "after"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := decodeContext(tt.blob)
			if tt.empty {
				if !got.IsEmpty() {
					t.Fatalf("decodeContext() = %+v, want nothing recovered", got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("decodeContext() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestDecodeContextSurvivesRubbish: the blob is untrusted, undocumented and changes
// between WhatsApp releases. A message whose extras cannot be read is still a
// message, so nothing here may panic or hang.
func TestDecodeContextSurvivesRubbish(t *testing.T) {
	t.Parallel()

	good := replyContext("3EB0ABCD1234567890AB", "34600111222@s.whatsapp.net", "hola")

	tests := []struct {
		name string
		blob []byte
	}{
		{name: "cut in half", blob: good[:len(good)/2]},
		{name: "one byte", blob: good[:1]},
		{name: "a length that runs off the end", blob: []byte{0x2a, 0x7f}},
		{name: "a varint that never ends", blob: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{name: "a wire type that does not exist", blob: []byte{0x2f, 0x01}},
		{name: "an enormous declared length", blob: []byte{0x2a, 0xff, 0xff, 0xff, 0xff, 0x0f}},
		{name: "all zeroes", blob: make([]byte, 64)},
		{name: "all ones", blob: []byte{0xff, 0xff, 0xff, 0xff}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Reaching the end without panicking is the whole assertion.
			_ = decodeContext(tt.blob)
		})
	}
}

func FuzzDecodeContext(f *testing.F) {
	f.Add(replyContext("3EB0ABCD1234567890AB", "34600111222@s.whatsapp.net", "hola"))
	f.Add(appendString(nil, fieldQuotedID, "3EB0ABCD1234567890AB"))
	f.Add([]byte{})
	f.Add([]byte{0x2a, 0x7f})
	f.Add(make([]byte, 128))

	f.Fuzz(func(t *testing.T, blob []byte) {
		got := decodeContext(blob)

		// Whatever comes back must have the shape the reader relies on, or a future
		// WhatsApp release could put an arbitrary string where an address goes and
		// have it believed.
		if got.QuotedID != "" && !looksLikeIdentifier([]byte(got.QuotedID)) {
			t.Errorf("an identifier that is not one was accepted: %q", got.QuotedID)
		}
		if got.QuotedSender != "" && !looksLikeAddress([]byte(got.QuotedSender)) {
			t.Errorf("an address that is not one was accepted: %q", got.QuotedSender)
		}
	})
}

func TestKindOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		code      int
		mediaType string
		want      string
	}{
		{name: "text", code: 0, want: "text"},
		{name: "an image by its number", code: 1, mediaType: "image/jpeg", want: "image"},
		{name: "a sticker", code: 15, mediaType: "image/webp", want: "sticker"},
		{name: "a voice note", code: 3, mediaType: "audio/ogg; codecs=opus", want: "voice"},
		{name: "a document", code: 8, mediaType: "application/pdf", want: "document"},
		{name: "a notice", code: 6, want: "system"},
		{name: "a deletion", code: 14, want: "deleted"},
		{name: "an unknown number with an image beside it", code: 91, mediaType: "image/png", want: "image"},
		{name: "an unknown number with a video beside it", code: 92, mediaType: "video/mp4", want: "video"},
		{name: "an unknown number with a sticker beside it", code: 93, mediaType: "image/webp", want: "sticker"},
		{name: "an unknown number with nothing beside it", code: 94, want: "unknown"},
		{name: "an unknown number with an unknown media type", code: 95, mediaType: "chemical/x-pdb", want: "unknown"},
		{name: "a codec parameter does not change the kind", code: 96, mediaType: "audio/mp4; codecs=mp4a", want: "voice"},
		{name: "spacing and case are forgiven", code: 97, mediaType: "  IMAGE/JPEG  ", want: "image"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := kindOf(tt.code, tt.mediaType).String(); got != tt.want {
				t.Errorf("kindOf(%d, %q) = %q, want %q", tt.code, tt.mediaType, got, tt.want)
			}
		})
	}
}
