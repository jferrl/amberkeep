package crypt15

import "errors"

// Guidance identifiers. Every user-facing error carries one so the UI can look up
// a plain-language explanation and the matching step in the guided knowledge base.
// They are part of the public contract: never renumber or reuse one.
const (
	GuidanceCrypt14NeedsRoot    = "crypt15.crypt14-needs-root"
	GuidancePasskeyNotSupported = "crypt15.passkey-not-supported"
	GuidanceWrongKey            = "crypt15.wrong-key"
	GuidanceMalformedFile       = "crypt15.malformed-file"
	GuidanceKeyFormat           = "crypt15.key-format"
	GuidanceUnexpectedPlaintext = "crypt15.unexpected-plaintext"
)

var (
	// ErrCrypt14 reports a crypt12/crypt14 backup. Those are encrypted with a key
	// that WhatsApp keeps inside its private app storage, so decrypting one needs
	// root access to the Android device. There is no offline path for most users.
	ErrCrypt14 = &Error{Guidance: GuidanceCrypt14NeedsRoot,
		msg: "this is a crypt14 backup, which needs a key file only reachable with root access; use an end-to-end encrypted backup with a 64-digit key instead"}

	// ErrPasskeyProtected reports a backup protected by a passkey rather than a
	// 64-digit key. WhatsApp releases that key only to the app after the passkey is
	// verified on the device, so the backup cannot be decrypted on a computer.
	ErrPasskeyProtected = &Error{Guidance: GuidancePasskeyNotSupported,
		msg: "this backup is protected by a passkey, which cannot be used outside WhatsApp; switch the backup to a 64-digit key and back up again"}

	// ErrWrongKey reports that authenticated decryption failed. Either the key does
	// not belong to this backup or the file is damaged; the two are indistinguishable.
	ErrWrongKey = &Error{Guidance: GuidanceWrongKey,
		msg: "the backup could not be decrypted with this key; check that the 64-digit key belongs to this backup and that the file is complete"}

	// ErrMalformed reports a file that is not a readable crypt15 backup at all.
	ErrMalformed = &Error{Guidance: GuidanceMalformedFile,
		msg: "this file is not a readable crypt15 backup"}

	// ErrKeyFormat reports a key that is not 64 hexadecimal digits or 32 raw bytes.
	ErrKeyFormat = &Error{Guidance: GuidanceKeyFormat,
		msg: "the key must be 64 hexadecimal digits, as WhatsApp shows it in eight groups of eight"}

	// ErrUnexpectedPlaintext reports a payload that authenticated correctly but does
	// not decompress. That means the key was right and the file intact, so the cause
	// is a format we do not understand rather than anything the user did wrong.
	ErrUnexpectedPlaintext = &Error{Guidance: GuidanceUnexpectedPlaintext,
		msg: "the backup was decrypted but its contents are not in a format this version understands; the file may come from a newer WhatsApp release"}
)

// Error is the error type this package returns. It carries a Guidance identifier
// so callers can show the user the right explanation instead of raw text, and it
// never contains key material.
type Error struct {
	// Guidance identifies the explanation to show the user.
	Guidance string

	msg string
	err error
}

func (e *Error) Error() string {
	if e.err != nil {
		return e.msg + ": " + e.err.Error()
	}
	return e.msg
}

func (e *Error) Unwrap() error { return e.err }

// Is reports whether target is the same guidance case, so callers can compare
// against the sentinels above even when a cause has been attached.
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return e.Guidance == t.Guidance
}

// withCause returns a copy of e carrying cause, keeping the sentinels immutable.
func (e *Error) withCause(cause error) *Error {
	return &Error{Guidance: e.Guidance, msg: e.msg, err: cause}
}
