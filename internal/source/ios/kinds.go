package ios

import (
	"sort"
	"strings"

	"github.com/jferrl/amberkeep/internal/model"
)

// What an iPhone's numeric message types mean.
//
// The mapping below was derived from a real store of 161,026 messages by joining
// each type to the media type recorded beside it, which is evidence rather than
// folklore: type 1 is an image because every one of its 6,658 rows carries
// image/jpeg, type 15 is a sticker because all 4,309 carry image/webp, type 8 is a
// document because its rows carry PDF and Office types. Types whose meaning no
// such evidence settles are deliberately absent; see kindOf.
var kinds = map[int]model.Kind{
	0:  model.KindText,
	1:  model.KindImage,
	2:  model.KindVideo,
	3:  model.KindVoice,
	4:  model.KindContact,
	5:  model.KindLocation,
	6:  model.KindSystem, // a group event
	7:  model.KindText,   // text whose link was given a preview
	8:  model.KindDocument,
	10: model.KindSystem, // an account or security notice
	14: model.KindDeleted,
	15: model.KindSticker,
}

// kindOf decides what a message is.
//
// The numeric type is tried first, and when it is one this build does not know the
// media type recorded beside the message answers instead. That second path is what
// keeps a new WhatsApp release from turning a photograph into an unrecognised row:
// the number may be new, but image/jpeg has not changed.
func kindOf(messageType int, mediaType string) model.Kind {
	if kind, known := kinds[messageType]; known {
		return kind
	}
	if kind := kindOfMedia(mediaType); kind != model.KindUnknown {
		return kind
	}
	return model.KindUnknown
}

// kindOfMedia reads the kind out of a media type.
func kindOfMedia(mediaType string) model.Kind {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == "" {
		return model.KindUnknown
	}
	// The parameters after a semicolon carry the codec, which changes far more often
	// than the type does: "audio/ogg; codecs=opus" is still audio.
	if cut := strings.IndexByte(mediaType, ';'); cut >= 0 {
		mediaType = strings.TrimSpace(mediaType[:cut])
	}

	switch {
	case mediaType == "image/webp":
		// Stickers are the only thing WhatsApp sends as WebP.
		return model.KindSticker
	case strings.HasPrefix(mediaType, "image/"):
		return model.KindImage
	case strings.HasPrefix(mediaType, "video/"):
		return model.KindVideo
	case strings.HasPrefix(mediaType, "audio/"):
		return model.KindVoice
	case strings.HasPrefix(mediaType, "application/"), strings.HasPrefix(mediaType, "text/"):
		return model.KindDocument
	default:
		return model.KindUnknown
	}
}

// isSystem reports whether a type is one of the two the store uses for its own
// notices, which is the only case where the group event code means anything.
func isSystem(messageType int) bool {
	return messageType == 6 || messageType == 10
}

// Known is every type code this build has a meaning for from the number alone.
//
// Exported for the canary. A code that is not here is not necessarily a message this
// build cannot read: an iPhone store records a media type beside the message, and
// kindOf falls back to that, which is what keeps a new WhatsApp release from turning
// a photograph into an unrecognised row. What a code missing from this list means is
// that the number carries no meaning here, which is the thing worth reporting.
func Known() []int {
	codes := make([]int, 0, len(kinds))
	for code := range kinds {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	return codes
}
