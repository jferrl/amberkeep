package crypt15

import (
	"crypto/hkdf"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// keyLen is the size of the backup root key and of the derived encryption key.
const keyLen = 32

// hkdfInfo is the label WhatsApp uses when deriving the backup encryption key
// from the root key. Changing it changes every derived key, so it is fixed here
// and covered by the golden test.
const hkdfInfo = "backup encryption"

// Key is a WhatsApp backup root key: the 64-digit key the app shows the user, in
// its 32-byte binary form.
//
// Key deliberately hides its bytes. Its String, GoString and MarshalText methods
// all redact, so a key cannot reach a log line, an error message or a crash report
// by accident. Only the package's own decryption path reads the material.
type Key struct {
	material [keyLen]byte
}

// ParseKey reads the 64-digit key as WhatsApp displays it. Spaces, dashes, tabs
// and newlines are ignored, so the eight groups of eight can be pasted verbatim,
// and case does not matter. It returns ErrKeyFormat for anything else.
func ParseKey(s string) (Key, error) {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '\t', '\n', '\r', ':':
			return -1
		}
		return r
	}, s)

	if len(cleaned) != hex.EncodedLen(keyLen) {
		return Key{}, ErrKeyFormat
	}
	decoded, err := hex.DecodeString(cleaned)
	if err != nil {
		return Key{}, ErrKeyFormat.withCause(err)
	}
	return KeyFromBytes(decoded)
}

// KeyFromBytes wraps a raw 32-byte root key, as found in an encrypted_backup.key
// file. It returns ErrKeyFormat if b is not exactly 32 bytes.
func KeyFromBytes(b []byte) (Key, error) {
	if len(b) != keyLen {
		return Key{}, ErrKeyFormat
	}
	var k Key
	copy(k.material[:], b)
	return k, nil
}

// Equal reports whether two keys hold the same material, in constant time.
func (k Key) Equal(other Key) bool {
	return subtle.ConstantTimeCompare(k.material[:], other.material[:]) == 1
}

// String redacts. Keys must never appear in logs, errors or crash reports.
func (k Key) String() string { return "crypt15.Key(redacted)" }

// GoString redacts, so %#v and spew-style dumps stay safe too.
func (k Key) GoString() string { return "crypt15.Key(redacted)" }

// MarshalText redacts, so a key embedded in a struct cannot leak through JSON.
func (k Key) MarshalText() ([]byte, error) { return []byte("redacted"), nil }

// derive returns the AES-256 key WhatsApp uses for the backup payload:
// HKDF-SHA256 over the root key with an empty salt and the fixed info label.
func (k Key) derive() ([]byte, error) {
	return hkdf.Key(sha256.New, k.material[:], nil, hkdfInfo, keyLen)
}
