package android

import "github.com/jferrl/amberkeep/internal/model"

// Android stores what a message is as a small integer. The meanings below were
// confirmed against a real database from WhatsApp 2.26 by counting how many rows of
// each code carried text, an attachment or a system action, and cross-checked with
// the open-source parsers that have tracked this format for years.
//
// Codes not listed here become model.KindUnknown and keep their number, which is
// what the schema report surfaces so a new code is noticed rather than guessed at.
const (
	typeText         = 0
	typeImage        = 1
	typeAudio        = 2
	typeVideo        = 3
	typeContact      = 4
	typeLocation     = 5
	typeSystem       = 7
	typeDocument     = 9
	typeMissedCall   = 10
	typeCall         = 11
	typeGIF          = 13
	typeContactArray = 14
	typeDeleted      = 15
	typeLiveLocation = 16
	typeSticker      = 20
	typeViewOnceImg  = 42
	typeViewOnceVid  = 43
	typeDeletedAdmin = 64
	typePoll         = 66
	typeCallLog      = 90
	typeEvent        = 92
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
	case typeSystem:
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
	case typeViewOnceImg, typeViewOnceVid:
		return model.KindViewOnce
	case typePoll:
		return model.KindPoll
	case typeEvent:
		return model.KindEvent
	default:
		return model.KindUnknown
	}
}
