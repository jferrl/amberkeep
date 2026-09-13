package android

import "github.com/jferrl/amberkeep/internal/model"

// Android stores what a message is as a small integer.
//
// The meanings below come from the tools that have tracked this format for years:
// IPED's Android extractor, which is the most complete public decoder and is
// verified against real devices, cross-checked with WhatsApp-Chat-Exporter, whapa
// and a decompiled build of the app itself. They were then checked against a real
// 1.1 million message database, where the frequency of each code matches what those
// sources predict.
//
// A code that no source identifies is left unrecognised on purpose. Guessing would
// mislabel real messages, and a wrong label is worse than an honest one: the
// message is carried through with its original number either way, and the report
// surfaces it so the mapping can be extended from real archives rather than hope.
const (
	typeText          = 0
	typeImage         = 1
	typeAudio         = 2
	typeVideo         = 3
	typeContact       = 4
	typeLocation      = 5
	typeSystem        = 7
	typeDocument      = 9
	typeMissedCall    = 10
	typeCall          = 11
	typeGIF           = 13
	typeContactArray  = 14
	typeDeleted       = 15
	typeLiveLocation  = 16
	typeSticker       = 20
	typeGroupInvite   = 24 // an invitation card carrying the group's name and expiry
	typeBizList       = 25 // a business message offering a list to choose from
	typeBizButtons    = 27 // a business message with buttons, including one-time codes
	typeOfficial      = 28 // an announcement from WhatsApp's own account
	typeTemplateQuote = 32 // a reply quoting one of those
	typeEphemeralSet  = 36 // the disappearing-messages timer was set in a direct chat
	typeViewOnceImg   = 42
	typeViewOnceVid   = 43
	typeInteractive   = 45 // a business message built from interface elements
	typeInteractiveQ  = 46 // a reply quoting one. On iPhone this same number means a poll
	typeInteractiveR  = 49 // another form of reply to interface elements
	typeCarousel      = 55 // a business message showing a carousel of cards
	typeDeletedAdmin  = 64
	typePoll          = 66 // on iPhone this number means an album, so never share this table
	typeViewOnceAudio = 82
	typeCallLog       = 90
	typeEvent         = 92
	typeAlbum         = 99  // a container; the pictures themselves are separate messages
	typePollAlt       = 106 // a second encoding of a poll
	typePrivacyState  = 112 // advanced chat privacy was turned on or off
	typeNotDisplayed  = 116 // a row the app itself never shows
)

// originVoiceNote marks a recording made in the app rather than an audio file that
// was attached. Nothing else in the row distinguishes the two, and the difference
// matters: a voice message reads very differently from a shared song.
const originVoiceNote = 1

// kindOf translates a source type code, using the message origin to tell a voice
// message from an audio file.
func kindOf(sourceType, origin int) model.Kind {
	switch sourceType {
	case typeText:
		return model.KindText
	case typeImage:
		return model.KindImage
	case typeAudio:
		if origin == originVoiceNote {
			return model.KindVoice
		}
		return model.KindAudio
	case typeVideo:
		return model.KindVideo
	case typeContact, typeContactArray:
		return model.KindContact
	case typeLocation, typeLiveLocation:
		return model.KindLocation
	case typeSystem, typeEphemeralSet, typePrivacyState:
		return model.KindSystem
	case typeDocument:
		return model.KindDocument
	case typeMissedCall, typeCall, typeCallLog:
		return model.KindCall
	case typeGIF:
		return model.KindGIF
	case typeDeleted, typeDeletedAdmin:
		return model.KindDeleted
	case typeSticker:
		return model.KindSticker
	case typeViewOnceImg, typeViewOnceVid, typeViewOnceAudio:
		return model.KindViewOnce
	case typePoll, typePollAlt:
		return model.KindPoll
	case typeEvent:
		return model.KindEvent
	case typeGroupInvite:
		return model.KindInvite
	case typeAlbum:
		return model.KindAlbum
	case typeNotDisplayed:
		return model.KindIgnored
	case typeBizList, typeBizButtons, typeOfficial, typeTemplateQuote,
		typeInteractive, typeInteractiveQ, typeInteractiveR, typeCarousel:
		// These carry readable words, which is the part worth keeping. They are
		// marked as interactive rather than as plain text so an export can say what
		// they were instead of pretending somebody typed them.
		return model.KindInteractive
	default:
		return model.KindUnknown
	}
}
