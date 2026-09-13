package crypt15

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ivLen is the nonce length WhatsApp uses for the backup payload. It is 16 bytes,
// not the 12 that AES-GCM defaults to, which is why the cipher is constructed with
// an explicit nonce size.
const ivLen = 16

// maxHeaderLen bounds the BackupPrefix message. Real headers are a few dozen bytes;
// 64 KiB is generous enough to survive future fields and small enough that a hostile
// length cannot be used to address past the file.
const maxHeaderLen = 1 << 16

// Protobuf field numbers inside WhatsApp's BackupPrefix header message. Only the
// ones that change our behaviour are named; everything else is skipped.
const (
	fieldKeyTypeDeprecated = 1 // varint enum, older builds
	fieldCrypt14KeyData    = 2 // message, present only on crypt12/crypt14
	fieldE2EEKeyData       = 3 // message, holds the IV on crypt15
	fieldBackupMetadata    = 4 // message, version and expiry info
	fieldPasskeyMetadata   = 5 // message, present only on passkey-protected backups
	fieldKeyTypeNew        = 6 // varint enum, current builds

	fieldIVInsideE2EEKeyData = 1 // bytes, the 16-byte nonce
)

// Protobuf wire types.
const (
	wireVarint  = 0
	wireFixed64 = 1
	wireBytes   = 2
	wireFixed32 = 5
)

// errTruncated marks a header that ends mid-field. It is wrapped into ErrMalformed
// by the caller rather than surfaced directly.
var errTruncated = errors.New("truncated protobuf")

// header is what the prefix of a backup file tells us: which flavour of backup it
// is and, for crypt15, the nonce the payload was encrypted with.
type header struct {
	iv         []byte
	isCrypt14  bool
	hasPasskey bool
}

// parseHeader reads the length-prefixed BackupPrefix message at the start of a
// backup file and reports how many bytes of the file it consumed.
//
// It rejects, rather than guesses at, anything it does not understand: this parser
// reads untrusted bytes and is covered by a fuzz test.
func parseHeader(data []byte) (header, int, error) {
	prefixLen, n := binary.Uvarint(data)
	if n <= 0 {
		return header{}, 0, ErrMalformed.withCause(errTruncated)
	}
	// A real header is a few dozen bytes. Bounding it before any conversion keeps a
	// hostile varint from addressing past the slice and makes the int conversion
	// below provably safe on every platform.
	if prefixLen == 0 || prefixLen > maxHeaderLen {
		return header{}, 0, ErrMalformed.withCause(
			fmt.Errorf("header claims to be %d bytes, which is not a backup header", prefixLen))
	}
	headerLen := int(prefixLen) //nolint:gosec // bounded by maxHeaderLen on the line above
	if headerLen > len(data)-n {
		return header{}, 0, ErrMalformed.withCause(
			fmt.Errorf("header of %d bytes does not fit in a %d byte file", headerLen, len(data)))
	}

	h, err := parseBackupPrefix(data[n : n+headerLen])
	if err != nil {
		return header{}, 0, err
	}
	return h, n + headerLen, nil
}

// parseBackupPrefix walks the BackupPrefix message, picking out the fields that
// decide what we can do with this file and skipping the rest.
func parseBackupPrefix(msg []byte) (header, error) {
	var h header

	err := eachField(msg, func(field int, wire int, value []byte, _ uint64) error {
		switch {
		case field == fieldE2EEKeyData && wire == wireBytes:
			iv, err := ivFromE2EEKeyData(value)
			if err != nil {
				return err
			}
			h.iv = iv
		case field == fieldCrypt14KeyData && wire == wireBytes:
			h.isCrypt14 = true
		case field == fieldPasskeyMetadata && wire == wireBytes:
			h.hasPasskey = true
		}
		return nil
	})
	if err != nil {
		return header{}, err
	}

	// Order matters: report the most actionable problem. A passkey backup cannot be
	// decrypted at all, a crypt14 backup needs root, and only then is a missing IV
	// simply a malformed file.
	switch {
	case h.hasPasskey && h.iv == nil:
		return header{}, ErrPasskeyProtected
	case h.isCrypt14 && h.iv == nil:
		return header{}, ErrCrypt14
	case h.iv == nil:
		return header{}, ErrMalformed.withCause(errors.New("no crypt15 nonce in header"))
	}
	return h, nil
}

// ivFromE2EEKeyData pulls the nonce out of the nested C15_IV message.
func ivFromE2EEKeyData(msg []byte) ([]byte, error) {
	var iv []byte
	err := eachField(msg, func(field int, wire int, value []byte, _ uint64) error {
		if field == fieldIVInsideE2EEKeyData && wire == wireBytes {
			iv = value
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(iv) != ivLen {
		return nil, ErrMalformed.withCause(
			fmt.Errorf("nonce is %d bytes, want %d", len(iv), ivLen))
	}
	return iv, nil
}

// eachField walks a protobuf message, calling fn for every field it can decode.
// Length-delimited values arrive in value; varints arrive in num. Unknown wire
// types end the walk with an error rather than being skipped silently, because a
// file we cannot fully parse is a file we should not try to decrypt.
func eachField(msg []byte, fn func(field, wire int, value []byte, num uint64) error) error {
	for len(msg) > 0 {
		tag, n := binary.Uvarint(msg)
		if n <= 0 {
			return ErrMalformed.withCause(errTruncated)
		}
		msg = msg[n:]

		field, wire := int(tag>>3), int(tag&0x7)
		if field == 0 {
			return ErrMalformed.withCause(errors.New("protobuf field number 0"))
		}

		switch wire {
		case wireVarint:
			v, n := binary.Uvarint(msg)
			if n <= 0 {
				return ErrMalformed.withCause(errTruncated)
			}
			msg = msg[n:]
			if err := fn(field, wire, nil, v); err != nil {
				return err
			}
		case wireBytes:
			length, n := binary.Uvarint(msg)
			if n <= 0 {
				return ErrMalformed.withCause(errTruncated)
			}
			msg = msg[n:]
			if length > uint64(len(msg)) {
				return ErrMalformed.withCause(errTruncated)
			}
			value := msg[:length]
			msg = msg[length:]
			if err := fn(field, wire, value, 0); err != nil {
				return err
			}
		case wireFixed64:
			if len(msg) < 8 {
				return ErrMalformed.withCause(errTruncated)
			}
			msg = msg[8:]
		case wireFixed32:
			if len(msg) < 4 {
				return ErrMalformed.withCause(errTruncated)
			}
			msg = msg[4:]
		default:
			return ErrMalformed.withCause(fmt.Errorf("unsupported protobuf wire type %d", wire))
		}
	}
	return nil
}
