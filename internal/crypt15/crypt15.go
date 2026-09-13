// Package crypt15 decrypts WhatsApp Android "crypt15" backups, the end-to-end
// encrypted format produced when the user turns on encrypted backups and saves a
// 64-digit key.
//
// A crypt15 file is laid out as:
//
//	[varint header length][BackupPrefix protobuf][AES-256-GCM ciphertext][16-byte tag][16-byte MD5]
//
// The trailing MD5 covers everything before it and is present on single-file
// backups only, so it is detected rather than assumed. The payload key is derived
// from the user's root key with HKDF-SHA256, and the plaintext is usually zlib
// compressed.
//
// The package holds the whole file in memory. Backups are typically 200-500 MB,
// which is a fair trade for code with no streaming state to get wrong; a streaming
// path can be added if real files outgrow it.
//
// Nothing here touches the network or writes to disk, and key material never
// reaches a log line or an error message.
package crypt15

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //nolint:gosec // not a security control: WhatsApp appends an MD5 as a plain integrity marker
	"errors"
	"fmt"
	"io"
)

// md5Len is the size of the optional integrity footer.
const md5Len = 16

// gcmTagLen is the AES-GCM authentication tag length.
const gcmTagLen = 16

// zlibMagic is the first byte of a zlib stream at every compression level
// WhatsApp uses. The plaintext is compressed in practice but not always, so the
// first byte decides rather than an assumption.
const zlibMagic = 0x78

// maxDecompressed caps the decompressed payload at 8 GiB. A real msgstore.db is
// far smaller; the cap stops a malicious file from exhausting memory.
const maxDecompressed = 8 << 30

// Decrypt returns the plaintext of a crypt15 backup, decompressing it when needed.
//
// It reports ErrCrypt14 for crypt12/crypt14 backups, ErrPasskeyProtected for
// passkey-protected ones, ErrWrongKey when authentication fails, and ErrMalformed
// for anything that is not a readable backup. Each carries a Guidance identifier
// the caller can turn into an explanation for the user.
func Decrypt(key Key, file []byte) ([]byte, error) {
	body, err := stripIntegrityFooter(file)
	if err != nil {
		return nil, err
	}

	h, headerLen, err := parseHeader(body)
	if err != nil {
		return nil, err
	}

	ciphertext := body[headerLen:]
	if len(ciphertext) < gcmTagLen {
		return nil, ErrMalformed.withCause(
			fmt.Errorf("payload is %d bytes, too short to hold an authentication tag", len(ciphertext)))
	}

	plaintext, err := decryptPayload(key, h.iv, ciphertext)
	if err != nil {
		return nil, err
	}
	return decompress(plaintext)
}

// stripIntegrityFooter removes the trailing MD5 when the file carries one.
//
// The footer is optional: chunked backups omit it. Recomputing the digest is the
// only reliable way to tell, because a tag and a digest are both 16 opaque bytes.
func stripIntegrityFooter(file []byte) ([]byte, error) {
	// A header varint, a minimal prefix, a tag and a footer: anything shorter is
	// definitely not a backup, and the bound keeps the slicing below in range.
	if len(file) < md5Len+gcmTagLen+2 {
		return nil, ErrMalformed.withCause(
			fmt.Errorf("file is %d bytes, too short to be a backup", len(file)))
	}

	body, footer := file[:len(file)-md5Len], file[len(file)-md5Len:]
	sum := md5.Sum(body) //nolint:gosec // integrity marker only, see package comment
	if bytes.Equal(sum[:], footer) {
		return body, nil
	}
	return file, nil
}

// decryptPayload authenticates and decrypts the payload in place, reusing the
// ciphertext buffer so a large backup is not copied twice in memory.
func decryptPayload(key Key, iv, ciphertext []byte) ([]byte, error) {
	derived, err := key.derive()
	if err != nil {
		return nil, ErrMalformed.withCause(fmt.Errorf("deriving the payload key: %w", err))
	}

	block, err := aes.NewCipher(derived)
	if err != nil {
		return nil, ErrMalformed.withCause(fmt.Errorf("preparing the cipher: %w", err))
	}
	// WhatsApp uses a 16-byte nonce rather than the 12-byte default.
	gcm, err := cipher.NewGCMWithNonceSize(block, ivLen)
	if err != nil {
		return nil, ErrMalformed.withCause(fmt.Errorf("preparing the cipher: %w", err))
	}

	plaintext, err := gcm.Open(ciphertext[:0], iv, ciphertext, nil)
	if err != nil {
		// A wrong key and a damaged file are indistinguishable here, and the cause
		// is deliberately dropped: it says nothing useful and this path is reached
		// with attacker-influenced input.
		return nil, ErrWrongKey
	}
	return plaintext, nil
}

// decompress inflates the plaintext when it is a zlib stream and returns it
// unchanged otherwise.
func decompress(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 || plaintext[0] != zlibMagic {
		return plaintext, nil
	}

	r, err := zlib.NewReader(bytes.NewReader(plaintext))
	if err != nil {
		return nil, ErrUnexpectedPlaintext.withCause(fmt.Errorf("opening the compressed payload: %w", err))
	}

	out, err := io.ReadAll(io.LimitReader(r, maxDecompressed))
	if err != nil {
		return nil, ErrUnexpectedPlaintext.withCause(fmt.Errorf("reading the compressed payload: %w", err))
	}
	// Close verifies the stream's checksum, so it is a real correctness check here
	// rather than a resource release, and its result is worth reporting.
	if err := r.Close(); err != nil {
		return nil, ErrUnexpectedPlaintext.withCause(fmt.Errorf("verifying the compressed payload: %w", err))
	}
	if int64(len(out)) == maxDecompressed {
		return nil, ErrUnexpectedPlaintext.withCause(errors.New("payload is implausibly large"))
	}
	return out, nil
}
