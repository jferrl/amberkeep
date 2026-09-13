package ios

import (
	"unicode/utf8"
)

// Recovering replies from an iPhone store.
//
// The obvious place for a reply to point at what it answers is ZWAMESSAGE's
// ZPARENTMESSAGE column, which is what the public tools read. On a real store of
// 161,026 messages that column is empty in every single row. What is actually
// there is a protobuf in ZWAMEDIAITEM.ZMETADATA, attached to the replying message,
// holding the answered message's identifier, who sent it, and a copy of its text.
//
// The field numbers below were established by decoding 161,026 rows and checking
// the identifiers against the messages table: 99 per cent of them name a message
// that is really there, which is the evidence the mapping rests on. It is read
// defensively all the same. A value is accepted only if it has the shape it should
// have, so a field that means something else in another WhatsApp release is
// ignored rather than believed.
//
// The decoder walks the buffer in place and allocates only the strings that are
// kept, because this runs once per message and an archive has a million of them.

// Protobuf field numbers inside the context attached to a message.
const (
	fieldQuotedID     = 5  // the answered message's identifier
	fieldQuotedSender = 6  // who sent the answered message, in a group
	fieldQuotedBody   = 19 // a copy of the answered message
	fieldQuotedText   = 1  // inside the copy, its words
)

// messageContext is what was recovered from the protobuf beside a message.
type messageContext struct {
	// QuotedID is the identifier of the message this one answers.
	QuotedID string
	// QuotedSender is who sent it, when the store recorded anybody. It is absent in
	// a one-to-one conversation, where there is only one person it could be.
	QuotedSender string
	// QuotedText is the copy of the answered message's words that travelled with
	// the reply. It survives even when the answered message itself is gone.
	QuotedText string
}

// IsEmpty reports whether nothing was recovered.
func (c messageContext) IsEmpty() bool {
	return c.QuotedID == "" && c.QuotedText == "" && c.QuotedSender == ""
}

// decodeContext reads what it recognises out of the protobuf beside a message.
//
// It never fails: the blob is untrusted, it changes between WhatsApp releases, and
// a message whose extras cannot be read is still a message. Anything unrecognised
// is skipped.
func decodeContext(blob []byte) messageContext {
	var out messageContext

	for number, value, rest, ok := nextField(blob); ok; number, value, rest, ok = nextField(rest) {
		switch number {
		case fieldQuotedID:
			if looksLikeIdentifier(value) {
				out.QuotedID = string(value)
			}
		case fieldQuotedSender:
			if looksLikeAddress(value) {
				out.QuotedSender = string(value)
			}
		case fieldQuotedBody:
			out.QuotedText = quotedText(value)
		}
	}
	return out
}

// quotedText reads the answered message's words out of the copy that travelled
// with the reply.
func quotedText(body []byte) string {
	for number, value, rest, ok := nextField(body); ok; number, value, rest, ok = nextField(rest) {
		if number == fieldQuotedText && utf8.Valid(value) {
			return string(value)
		}
	}
	return ""
}

// looksLikeIdentifier reports whether these bytes have the shape of a WhatsApp
// message identifier: hexadecimal, long enough to be one, short enough not to be
// something else that happens to be hexadecimal.
func looksLikeIdentifier(value []byte) bool {
	if len(value) < 16 || len(value) > 40 {
		return false
	}
	for _, b := range value {
		switch {
		case b >= '0' && b <= '9':
		case b >= 'a' && b <= 'f':
		case b >= 'A' && b <= 'F':
		default:
			return false
		}
	}
	return true
}

// looksLikeAddress reports whether these bytes are a WhatsApp address rather than
// some other string that happens to sit in the same field.
func looksLikeAddress(value []byte) bool {
	if len(value) < 3 || len(value) > 128 {
		return false
	}
	at := -1
	for i, b := range value {
		if b == '@' {
			if at >= 0 {
				return false
			}
			at = i
		}
	}
	return at > 0 && at < len(value)-1 && utf8.Valid(value)
}

// Just enough protobuf to walk a message.
//
// The generated-code libraries want a schema, and there is no schema for this: the
// shape is WhatsApp's, undocumented, and changes. Walking the wire format needs
// about forty lines, no dependency, and no allocation.

// nextField returns the next field's number and its bytes, along with what is left.
//
// Numeric fields are skipped rather than returned, because nothing recovered here
// is a number. A malformed buffer stops the walk: ok is false and whatever was
// read before it stands.
func nextField(buf []byte) (number int, value, rest []byte, ok bool) {
	for len(buf) > 0 {
		key, after, good := varint(buf)
		if !good {
			return 0, nil, nil, false
		}
		buf = after

		number, wire := int(key>>3), key&7
		switch wire {
		case 2: // length-delimited: strings, bytes, nested messages
			length, after, good := varint(buf)
			if !good || length > uint64(len(after)) {
				return 0, nil, nil, false
			}
			return number, after[:length], after[length:], true
		case 0: // a number
			_, after, good := varint(buf)
			if !good {
				return 0, nil, nil, false
			}
			buf = after
		case 5: // four bytes
			if len(buf) < 4 {
				return 0, nil, nil, false
			}
			buf = buf[4:]
		case 1: // eight bytes
			if len(buf) < 8 {
				return 0, nil, nil, false
			}
			buf = buf[8:]
		default: // a wire type that no longer exists, or nonsense
			return 0, nil, nil, false
		}
	}
	return 0, nil, nil, false
}

// varint reads protobuf's variable-length integer.
func varint(buf []byte) (value uint64, rest []byte, ok bool) {
	var shift uint
	for i, b := range buf {
		if shift >= 64 {
			return 0, nil, false
		}
		value |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return value, buf[i+1:], true
		}
		shift += 7
	}
	return 0, nil, false
}
