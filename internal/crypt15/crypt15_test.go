package crypt15

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //nolint:gosec // mirrors the integrity marker the format uses
	"encoding/binary"
	"errors"
	"os"
	"strings"
	"testing"
)

// testKey is a fixed root key. It is not secret: it exists only so the synthetic
// fixtures below are reproducible.
const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestParseKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string // canonical hex, empty when an error is expected
		err   error
	}{
		{
			name:  "plain 64 hex digits",
			input: testKey,
			want:  testKey,
		},
		{
			name:  "grouped as WhatsApp displays it",
			input: "0394 79d3 bf50 0fd8 f523 813f aefc d7ef e9b8 c485 bba4 b36f c9bf 1f9a 8699 5657",
			want:  "039479d3bf500fd8f523813faefcd7efe9b8c485bba4b36fc9bf1f9a86995657",
		},
		{
			name:  "uppercase is accepted",
			input: strings.ToUpper(testKey),
			want:  testKey,
		},
		{
			name:  "dashes and newlines are ignored",
			input: "0123-4567-89ab-cdef\n0123456789abcdef\r\n0123456789abcdef0123456789abcdef",
			want:  testKey,
		},
		{
			name:  "too short is rejected",
			input: testKey[:63],
			err:   ErrKeyFormat,
		},
		{
			name:  "too long is rejected",
			input: testKey + "00",
			err:   ErrKeyFormat,
		},
		{
			name:  "non hexadecimal is rejected",
			input: strings.Repeat("z", 64),
			err:   ErrKeyFormat,
		},
		{
			name:  "empty is rejected",
			input: "",
			err:   ErrKeyFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseKey(tt.input)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("ParseKey() error = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseKey() unexpected error: %v", err)
			}
			want, err := ParseKey(tt.want)
			if err != nil {
				t.Fatalf("ParseKey(want) failed: %v", err)
			}
			if !got.Equal(want) {
				t.Error("ParseKey() produced a different key than the canonical form")
			}
		})
	}
}

// TestKeyNeverLeaks is the guard behind the promise that key material cannot reach
// a log line, an error message or a crash report.
func TestKeyNeverLeaks(t *testing.T) {
	t.Parallel()

	key, err := ParseKey(testKey)
	if err != nil {
		t.Fatalf("ParseKey() failed: %v", err)
	}

	rendered := []string{
		key.String(),
		key.GoString(),
	}
	text, err := key.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText() failed: %v", err)
	}
	rendered = append(rendered, string(text))

	for _, s := range rendered {
		if strings.Contains(strings.ToLower(s), "0123456789abcdef") {
			t.Errorf("key material leaked into %q", s)
		}
	}
}

func TestDecrypt(t *testing.T) {
	t.Parallel()

	payload := []byte("SQLite format 3\x00 pretend this is a message database")

	tests := []struct {
		name    string
		file    func(t *testing.T) []byte
		key     string
		want    []byte
		wantErr error
	}{
		{
			name: "compressed payload round trips",
			file: func(t *testing.T) []byte { return buildBackup(t, payload, compressed, withFooter) },
			key:  testKey,
			want: payload,
		},
		{
			name: "uncompressed payload round trips",
			file: func(t *testing.T) []byte { return buildBackup(t, payload, uncompressed, withFooter) },
			key:  testKey,
			want: payload,
		},
		{
			name: "chunked backup without the integrity footer",
			file: func(t *testing.T) []byte { return buildBackup(t, payload, compressed, withoutFooter) },
			key:  testKey,
			want: payload,
		},
		{
			name:    "wrong key is reported as such",
			file:    func(t *testing.T) []byte { return buildBackup(t, payload, compressed, withFooter) },
			key:     strings.Repeat("ab", 32),
			wantErr: ErrWrongKey,
		},
		{
			name: "crypt14 backup explains it needs root",
			file: func(t *testing.T) []byte {
				return buildBackup(t, payload, compressed, withFooter, asCrypt14)
			},
			key:     testKey,
			wantErr: ErrCrypt14,
		},
		{
			name: "passkey backup explains it cannot be used",
			file: func(t *testing.T) []byte {
				return buildBackup(t, payload, compressed, withFooter, asPasskey)
			},
			key:     testKey,
			wantErr: ErrPasskeyProtected,
		},
		{
			name:    "empty file is malformed",
			file:    func(t *testing.T) []byte { return nil },
			key:     testKey,
			wantErr: ErrMalformed,
		},
		{
			name:    "random bytes are malformed",
			file:    func(t *testing.T) []byte { return bytes.Repeat([]byte{0xff}, 128) },
			key:     testKey,
			wantErr: ErrMalformed,
		},
		{
			name: "truncated payload fails authentication",
			file: func(t *testing.T) []byte {
				full := buildBackup(t, payload, compressed, withoutFooter)
				return full[:len(full)-8]
			},
			key:     testKey,
			wantErr: ErrWrongKey,
		},
		{
			name: "flipped ciphertext bit fails authentication",
			file: func(t *testing.T) []byte {
				full := buildBackup(t, payload, compressed, withoutFooter)
				full[len(full)-20] ^= 0x01
				return full
			},
			key:     testKey,
			wantErr: ErrWrongKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			key, err := ParseKey(tt.key)
			if err != nil {
				t.Fatalf("ParseKey() failed: %v", err)
			}

			got, err := Decrypt(key, tt.file(t))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Decrypt() error = %v, want %v", err, tt.wantErr)
				}
				var e *Error
				if !errors.As(err, &e) || e.Guidance == "" {
					t.Error("Decrypt() error carries no guidance identifier")
				}
				return
			}
			if err != nil {
				t.Fatalf("Decrypt() unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("Decrypt() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDecryptGolden checks our implementation against a file decrypted by
// wa-crypt-tools, the reference implementation. It is the test that proves the key
// derivation and cipher parameters are right; everything else in the package rests
// on it.
//
// Real backups and real keys never enter the repository, so the test reads their
// paths from the environment and skips when they are absent:
//
//	AMBERKEEP_GOLDEN_CRYPT15  path to a real msgstore.db.crypt15
//	AMBERKEEP_GOLDEN_KEY      path to a file holding the 64-digit key
//	AMBERKEEP_GOLDEN_EXPECTED path to the same backup decrypted by wa-crypt-tools
func TestDecryptGolden(t *testing.T) {
	encPath := os.Getenv("AMBERKEEP_GOLDEN_CRYPT15")
	keyPath := os.Getenv("AMBERKEEP_GOLDEN_KEY")
	expPath := os.Getenv("AMBERKEEP_GOLDEN_EXPECTED")
	if encPath == "" || keyPath == "" || expPath == "" {
		t.Skip("golden inputs not configured; set AMBERKEEP_GOLDEN_CRYPT15, AMBERKEEP_GOLDEN_KEY and AMBERKEEP_GOLDEN_EXPECTED")
	}

	rawKey, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("reading the key: %v", err)
	}
	key, err := ParseKey(string(rawKey))
	if err != nil {
		t.Fatalf("parsing the key: %v", err)
	}

	encrypted, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatalf("reading the backup: %v", err)
	}
	expected, err := os.ReadFile(expPath)
	if err != nil {
		t.Fatalf("reading the expected plaintext: %v", err)
	}

	got, err := Decrypt(key, encrypted)
	if err != nil {
		t.Fatalf("Decrypt() failed on a real backup: %v", err)
	}
	if len(got) != len(expected) {
		t.Fatalf("Decrypt() produced %d bytes, reference produced %d", len(got), len(expected))
	}
	if !bytes.Equal(got, expected) {
		t.Fatal("Decrypt() output differs from the reference implementation")
	}
	if !bytes.HasPrefix(got, []byte("SQLite format 3\x00")) {
		t.Error("Decrypt() output is not a SQLite database")
	}
}

func FuzzDecrypt(f *testing.F) {
	key, err := ParseKey(testKey)
	if err != nil {
		f.Fatalf("ParseKey() failed: %v", err)
	}
	f.Add(buildBackup(f, []byte("seed"), compressed, withFooter))
	f.Add(buildBackup(f, []byte("seed"), uncompressed, withoutFooter))
	f.Add([]byte{})
	f.Add([]byte{0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		// The contract is simply that no input panics and every failure is typed.
		out, err := Decrypt(key, data)
		if err == nil {
			return
		}
		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("Decrypt() returned an untyped error: %v", err)
		}
		if out != nil {
			t.Error("Decrypt() returned both output and an error")
		}
	})
}

// --- fixture construction -------------------------------------------------

type buildOption func(*buildConfig)

type buildConfig struct {
	compress bool
	footer   bool
	crypt14  bool
	passkey  bool
}

func compressed(c *buildConfig)    { c.compress = true }
func uncompressed(c *buildConfig)  { c.compress = false }
func withFooter(c *buildConfig)    { c.footer = true }
func withoutFooter(c *buildConfig) { c.footer = false }
func asCrypt14(c *buildConfig)     { c.crypt14 = true }
func asPasskey(c *buildConfig)     { c.passkey = true }

// buildBackup assembles a crypt15 file the way WhatsApp does, so the tests
// exercise the real parsing path rather than a mock.
func buildBackup(tb testing.TB, payload []byte, opts ...buildOption) []byte {
	tb.Helper()

	cfg := buildConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	key, err := ParseKey(testKey)
	if err != nil {
		tb.Fatalf("ParseKey() failed: %v", err)
	}
	derived, err := key.derive()
	if err != nil {
		tb.Fatalf("derive() failed: %v", err)
	}

	body := payload
	if cfg.compress {
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		if _, err := w.Write(payload); err != nil {
			tb.Fatalf("compressing the fixture: %v", err)
		}
		if err := w.Close(); err != nil {
			tb.Fatalf("closing the compressor: %v", err)
		}
		body = buf.Bytes()
	}

	iv := bytes.Repeat([]byte{0x2a}, ivLen)
	block, err := aes.NewCipher(derived)
	if err != nil {
		tb.Fatalf("aes.NewCipher() failed: %v", err)
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, ivLen)
	if err != nil {
		tb.Fatalf("cipher.NewGCMWithNonceSize() failed: %v", err)
	}
	ciphertext := gcm.Seal(nil, iv, body, nil)

	header := protoBytes(fieldE2EEKeyData, protoBytes(fieldIVInsideE2EEKeyData, iv))
	if cfg.crypt14 {
		// A crypt14 file carries the key data field and no crypt15 nonce.
		header = protoBytes(fieldCrypt14KeyData, []byte{0x08, 0x01})
	}
	if cfg.passkey {
		header = protoBytes(fieldPasskeyMetadata, []byte{0x08, 0x01})
	}

	var file []byte
	file = binary.AppendUvarint(file, uint64(len(header)))
	file = append(file, header...)
	file = append(file, ciphertext...)

	if cfg.footer {
		sum := md5.Sum(file) //nolint:gosec // mirrors the format
		file = append(file, sum[:]...)
	}
	return file
}

// protoBytes encodes one length-delimited protobuf field.
func protoBytes(field int, value []byte) []byte {
	var out []byte
	out = binary.AppendUvarint(out, uint64(field)<<3|wireBytes)
	out = binary.AppendUvarint(out, uint64(len(value)))
	return append(out, value...)
}

// TestDecryptToMatchesDecrypt is the guarantee behind the streaming path: the
// database written out must be byte for byte what holding the whole thing in
// memory would have produced. It exists only to use less memory, so producing
// anything different would be a straight loss.
func TestDecryptToMatchesDecrypt(t *testing.T) {
	t.Parallel()

	key, err := ParseKey(testKey)
	if err != nil {
		t.Fatalf("ParseKey() failed: %v", err)
	}

	tests := []struct {
		name    string
		payload []byte
		opts    []buildOption
	}{
		{name: "an ordinary database", payload: []byte("SQLite format 3\x00and then some contents")},
		{name: "nothing at all", payload: []byte{}},
		{name: "one byte", payload: []byte{0x42}},
		{name: "without the integrity footer", payload: []byte("a payload"), opts: []buildOption{withoutFooter}},
		{
			name: "something large enough to cross the writer's buffer",
			payload: func() []byte {
				out := make([]byte, 1<<20)
				for i := range out {
					out[i] = byte(i * 7)
				}
				return out
			}(),
		},
		{
			name: "something that compresses to almost nothing",
			payload: func() []byte {
				return make([]byte, 1<<20)
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// A fresh file for each, because decryption works in place.
			whole, err := Decrypt(key, buildBackup(t, tt.payload, tt.opts...))
			if err != nil {
				t.Fatalf("Decrypt() failed: %v", err)
			}

			var streamed bytes.Buffer
			written, err := DecryptTo(key, buildBackup(t, tt.payload, tt.opts...), &streamed)
			if err != nil {
				t.Fatalf("DecryptTo() failed: %v", err)
			}

			if !bytes.Equal(whole, streamed.Bytes()) {
				t.Errorf("streaming produced %d bytes, holding it produced %d, and they differ",
					streamed.Len(), len(whole))
			}
			if written != int64(streamed.Len()) {
				t.Errorf("DecryptTo() reported %d bytes written, wrote %d", written, streamed.Len())
			}
		})
	}
}

// TestDecryptToRefusesWhatDecryptRefuses: the two paths must agree about what a
// backup is, or one of them would open something the other calls forged.
func TestDecryptToRefusesWhatDecryptRefuses(t *testing.T) {
	t.Parallel()

	key, err := ParseKey(testKey)
	if err != nil {
		t.Fatalf("ParseKey() failed: %v", err)
	}
	other, err := ParseKey(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("ParseKey() failed: %v", err)
	}

	tests := []struct {
		name string
		key  Key
		file []byte
		want error
	}{
		{name: "the wrong key", key: other, file: buildBackup(t, []byte("hello")), want: ErrWrongKey},
		{name: "far too short", key: key, file: []byte{1, 2, 3}, want: ErrMalformed},
		{name: "nothing at all", key: key, file: nil, want: ErrMalformed},
		{
			name: "an older format",
			key:  key,
			file: buildBackup(t, []byte("a payload long enough to be recognised at all"), compressed, withFooter, asCrypt14),
			want: ErrCrypt14,
		},
		{
			name: "locked with a passkey",
			key:  key,
			file: buildBackup(t, []byte("a payload long enough to be recognised at all"), compressed, withFooter, asPasskey),
			want: ErrPasskeyProtected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			written, err := DecryptTo(tt.key, tt.file, &out)
			if !errors.Is(err, tt.want) {
				t.Fatalf("DecryptTo() error = %v, want %v", err, tt.want)
			}
			if written != 0 || out.Len() != 0 {
				t.Errorf("DecryptTo() wrote %d bytes of a backup it refused", out.Len())
			}
		})
	}
}

// TestDecryptToReportsAFailingWriter checks the path where the disk fills up
// halfway through, which is the one that leaves a file looking complete.
func TestDecryptToReportsAFailingWriter(t *testing.T) {
	t.Parallel()

	key, err := ParseKey(testKey)
	if err != nil {
		t.Fatalf("ParseKey() failed: %v", err)
	}

	payload := make([]byte, 1<<20)
	for i := range payload {
		payload[i] = byte(i)
	}

	_, err = DecryptTo(key, buildBackup(t, payload), failingWriter{after: 4096})
	if err == nil {
		t.Fatal("DecryptTo() succeeded against a writer that refused to write")
	}
}

// failingWriter accepts a little and then refuses, as a full disk does.
type failingWriter struct{ after int }

func (w failingWriter) Write(p []byte) (int, error) {
	if len(p) > w.after {
		return w.after, errors.New("no space left on device")
	}
	return len(p), nil
}
