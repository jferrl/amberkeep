package crypt15

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func TestKeyFromBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []byte
		err  error
	}{
		{name: "exactly 32 bytes", in: bytes.Repeat([]byte{0x01}, 32)},
		{name: "31 bytes is rejected", in: bytes.Repeat([]byte{0x01}, 31), err: ErrKeyFormat},
		{name: "33 bytes is rejected", in: bytes.Repeat([]byte{0x01}, 33), err: ErrKeyFormat},
		{name: "nil is rejected", in: nil, err: ErrKeyFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := KeyFromBytes(tt.in)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("KeyFromBytes() error = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("KeyFromBytes() unexpected error: %v", err)
			}
			same, err := KeyFromBytes(tt.in)
			if err != nil {
				t.Fatalf("KeyFromBytes() second call failed: %v", err)
			}
			if !got.Equal(same) {
				t.Error("KeyFromBytes() is not deterministic")
			}
		})
	}
}

func TestParseHeader(t *testing.T) {
	t.Parallel()

	validIV := bytes.Repeat([]byte{0x2a}, ivLen)
	e2eeField := protoBytes(fieldE2EEKeyData, protoBytes(fieldIVInsideE2EEKeyData, validIV))

	tests := []struct {
		name    string
		header  []byte
		wantIV  []byte
		wantErr error
	}{
		{
			name:   "crypt15 nonce is found",
			header: e2eeField,
			wantIV: validIV,
		},
		{
			name:   "unknown varint fields are skipped",
			header: concat(protoVarint(fieldKeyTypeNew, 3), e2eeField),
			wantIV: validIV,
		},
		{
			name:   "unknown length-delimited fields are skipped",
			header: concat(protoBytes(fieldBackupMetadata, []byte{0x08, 0x2a}), e2eeField),
			wantIV: validIV,
		},
		{
			name:   "fixed64 fields are skipped",
			header: concat(protoTag(7, wireFixed64), make([]byte, 8), e2eeField),
			wantIV: validIV,
		},
		{
			name:   "fixed32 fields are skipped",
			header: concat(protoTag(8, wireFixed32), make([]byte, 4), e2eeField),
			wantIV: validIV,
		},
		{
			name:    "crypt14 key data without a nonce",
			header:  protoBytes(fieldCrypt14KeyData, []byte{0x08, 0x01}),
			wantErr: ErrCrypt14,
		},
		{
			name:    "passkey metadata without a nonce",
			header:  protoBytes(fieldPasskeyMetadata, []byte{0x08, 0x01}),
			wantErr: ErrPasskeyProtected,
		},
		{
			name:    "passkey takes priority over crypt14",
			header:  concat(protoBytes(fieldCrypt14KeyData, nil), protoBytes(fieldPasskeyMetadata, nil)),
			wantErr: ErrPasskeyProtected,
		},
		{
			name:    "no recognised fields at all",
			header:  protoVarint(fieldKeyTypeDeprecated, 1),
			wantErr: ErrMalformed,
		},
		{
			name:    "nonce of the wrong length",
			header:  protoBytes(fieldE2EEKeyData, protoBytes(fieldIVInsideE2EEKeyData, []byte{0x01, 0x02})),
			wantErr: ErrMalformed,
		},
		{
			name:    "unsupported wire type",
			header:  protoTag(9, 3), // start-group, removed from proto3
			wantErr: ErrMalformed,
		},
		{
			name:    "field number zero",
			header:  []byte{0x02}, // tag 0, wire type 2
			wantErr: ErrMalformed,
		},
		{
			name:    "length runs past the end",
			header:  concat(protoTag(fieldE2EEKeyData, wireBytes), []byte{0x7f}),
			wantErr: ErrMalformed,
		},
		{
			name:    "truncated varint value",
			header:  concat(protoTag(fieldKeyTypeNew, wireVarint), []byte{0xff}),
			wantErr: ErrMalformed,
		},
		{
			name:    "truncated fixed64",
			header:  concat(protoTag(7, wireFixed64), []byte{0x00}),
			wantErr: ErrMalformed,
		},
		{
			name:    "truncated fixed32",
			header:  concat(protoTag(8, wireFixed32), []byte{0x00}),
			wantErr: ErrMalformed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Frame the header the way a real file does, then add a byte of payload
			// so the length prefix has something to point past.
			var file []byte
			file = binary.AppendUvarint(file, uint64(len(tt.header)))
			file = append(file, tt.header...)
			file = append(file, 0x00)

			h, n, err := parseHeader(file)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("parseHeader() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseHeader() unexpected error: %v", err)
			}
			if !bytes.Equal(h.iv, tt.wantIV) {
				t.Errorf("parseHeader() nonce = %x, want %x", h.iv, tt.wantIV)
			}
			if n != len(file)-1 {
				t.Errorf("parseHeader() consumed %d bytes, want %d", n, len(file)-1)
			}
		})
	}
}

func TestParseHeaderFraming(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file []byte
	}{
		{name: "empty input", file: nil},
		{name: "length prefix only", file: []byte{0x20}},
		{name: "length larger than the file", file: []byte{0x7f, 0x01, 0x02}},
		{name: "zero length header", file: []byte{0x00, 0x01, 0x02}},
		{name: "incomplete length varint", file: []byte{0xff, 0xff}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, _, err := parseHeader(tt.file); !errors.Is(err, ErrMalformed) {
				t.Fatalf("parseHeader() error = %v, want %v", err, ErrMalformed)
			}
		})
	}
}

// TestDecryptRejectsUndecompressablePayload covers the case where the key was
// right and the file intact, but the plaintext claims to be compressed and is not.
// That is a format we do not understand rather than user error, and it must say so.
func TestDecryptRejectsUndecompressablePayload(t *testing.T) {
	t.Parallel()

	// Starts with the zlib marker so decompression is attempted, then garbage.
	payload := append([]byte{zlibMagic}, make([]byte, 32)...)
	file := buildBackup(t, payload, uncompressed, withFooter)

	key, err := ParseKey(testKey)
	if err != nil {
		t.Fatalf("ParseKey() failed: %v", err)
	}
	if _, err := Decrypt(key, file); !errors.Is(err, ErrUnexpectedPlaintext) {
		t.Fatalf("Decrypt() error = %v, want %v", err, ErrUnexpectedPlaintext)
	}
}

// TestErrorGuidanceIsStable pins the guidance identifiers, which the user interface
// and the knowledge base both key off. Changing one silently would break the link
// between an error and its explanation.
func TestErrorGuidanceIsStable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  *Error
		want string
	}{
		{ErrCrypt14, "crypt15.crypt14-needs-root"},
		{ErrPasskeyProtected, "crypt15.passkey-not-supported"},
		{ErrWrongKey, "crypt15.wrong-key"},
		{ErrMalformed, "crypt15.malformed-file"},
		{ErrKeyFormat, "crypt15.key-format"},
		{ErrUnexpectedPlaintext, "crypt15.unexpected-plaintext"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()

			if tt.err.Guidance != tt.want {
				t.Errorf("guidance = %q, want %q", tt.err.Guidance, tt.want)
			}
			if tt.err.Error() == "" {
				t.Error("error message is empty")
			}
			// A wrapped error keeps matching its sentinel.
			wrapped := tt.err.withCause(errors.New("cause"))
			if !errors.Is(wrapped, tt.err) {
				t.Error("a wrapped error no longer matches its sentinel")
			}
			if !errors.Is(wrapped, wrapped) {
				t.Error("a wrapped error no longer matches itself")
			}
		})
	}
}

// --- protobuf helpers used only by the header tests -----------------------

func protoTag(field, wire int) []byte {
	return binary.AppendUvarint(nil, uint64(field)<<3|uint64(wire))
}

func protoVarint(field int, v uint64) []byte {
	return binary.AppendUvarint(protoTag(field, wireVarint), v)
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}
