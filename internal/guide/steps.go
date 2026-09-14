package guide

import "time"

// What somebody has to be told, in the order they need it.
//
// Every one of these was learned by doing it. The identifiers are stable and are
// never reused: a translation, a screenshot, or somebody quoting one in a support
// conversation all have to keep meaning the same step a year from now.

var steps = []Step{
	// ---------------------------------------------------------------- before
	{
		ID:       "safety-backup",
		Stage:    Before,
		Critical: true,
		Title:    "Make a safety backup, and archive it",
		Body: "Connect the phone, open Finder, choose the phone in the sidebar, and back it\n" +
			"up to this Mac with \"Encrypt local backup\" switched ON. Set a password you\n" +
			"will not lose.\n\n" +
			"Then right-click that backup in Finder's list and choose Archive.\n\n" +
			"Archiving is the part people skip and the part that matters. Finder keeps one\n" +
			"backup per phone and writes over it the next time. Without archiving, the very\n" +
			"next backup — the one this process is about to make — replaces the only copy\n" +
			"of everything as it was. Archiving puts it somewhere Finder will leave alone.\n\n" +
			"Encrypted, because only an encrypted backup carries Health data, saved\n" +
			"passwords and Wi-Fi networks. This is the copy you would go back to.",
		Expect: "the backup appears in Finder's list with a padlock and a date. Check the padlock\n" +
			"is there before going on: no padlock means it was not encrypted, and Health data\n" +
			"and passwords are not in it.",
		Takes: 30 * time.Minute,
	},
	{
		ID:       "health-data-goes",
		Stage:    Before,
		Critical: true,
		Title:    "Know what an unencrypted backup does not carry",
		Body: "The backup this restores from has to be unencrypted, because an encrypted one\n" +
			"is sealed with a key that never leaves the phone and nothing on a computer can\n" +
			"open it.\n\n" +
			"An unencrypted backup does not contain Health data, saved passwords, Wi-Fi\n" +
			"networks, or the keychain. Restoring one means losing those, and there is no\n" +
			"way around it: it is Apple's design, not a limitation of this program.\n\n" +
			"If Health data matters more than the message history, stop here. The archived\n" +
			"encrypted backup from the previous step still has everything, and restoring\n" +
			"that puts the phone back exactly as it is now.",
		Expect: "nothing yet. This is a decision, not an action.",
	},
	{
		ID:    "encryption-off",
		Stage: Before,
		Title: "Turn off encrypted backups",
		Body: "In Finder, with the phone selected, untick \"Encrypt local backup\".\n\n" +
			"It will ask for the password you set a moment ago. This does not remove the\n" +
			"archived backup — that one stays encrypted and stays where it is.",
		Expect: "the tick disappears. Finder may start a new backup on its own; let it finish.",
		Check:  "backup-not-encrypted",
	},
	{
		ID:       "find-my-off",
		Stage:    Before,
		Critical: false,
		Title:    "Turn off Find My iPhone",
		Body: "On the phone: Settings, your name at the top, Find My, Find My iPhone, and\n" +
			"turn it off. It asks for the Apple ID password.\n\n" +
			"A restore with Find My still on stops at the very end with \"error 211\" and\n" +
			"nothing else. Nothing is written and nothing is lost, but it is an hour spent\n" +
			"for a number instead of a sentence.\n\n" +
			"Write down that you turned it off. It has to go back on afterwards, and by\n" +
			"then it is easy to forget it was ever off.",
		Expect: "the switch goes grey and the phone confirms it.",
	},
	{
		ID:    "fresh-backup",
		Stage: Before,
		Title: "Back the phone up again, unencrypted this time",
		Body: "Back up to this Mac once more. This is the backup that will be changed and\n" +
			"restored, and it must be recent: anything said on the phone after this point\n" +
			"will not be in it and will be gone after the restore.\n\n" +
			"Say nothing on the phone from here until the restore has finished.",
		Expect: "Finder shows a new date with no padlock beside it.",
		Takes:  20 * time.Minute,
		Check:  "backup-is-recent",
	},
	{
		ID:    "power-and-space",
		Stage: Before,
		Title: "Plug both in, and check there is room",
		Body: "The phone on its cable and the computer on mains power. A restore that is\n" +
			"interrupted by a flat battery is the one way this goes badly wrong.\n\n" +
			"The computer needs free space for a second copy of the backup, which is as\n" +
			"large as the first.",
		Check: "enough-space",
	},

	// ------------------------------------------------------------- restoring
	{
		ID:    "start-restore",
		Stage: Restoring,
		Title: "Restore the changed backup",
		Body: "In Finder, with the phone selected, choose Restore Backup and pick the one\n" +
			"this program made. Its date is today's.\n\n" +
			"Nothing you do on the computer from here will change the result. The phone is\n" +
			"being written to; leave it connected and leave it alone.",
		Expect: "a progress bar on the computer first, then \"Restore in progress\" on the phone.",
		Takes:  45 * time.Minute,
	},
	{
		ID:    "looks-stalled",
		Stage: Restoring,
		Title: "The part that looks like it has frozen",
		Body: "Partway through, the phone shows the Apple logo with a progress bar that does\n" +
			"not move for a long time. This is normal and it is the longest part.\n\n" +
			"Do not unplug it. Do not hold the power button. A phone interrupted here is a\n" +
			"phone that has to be restored again from the beginning.",
		Expect: "the bar eventually moves, the phone restarts a second time, and Setup\n" +
			"Assistant appears asking about language and Wi-Fi.",
		Takes: 25 * time.Minute,
	},

	// ----------------------------------------------------------------- after
	{
		ID:       "decline-icloud",
		Stage:    After,
		Critical: true,
		Title:    "Say no to restoring chats from iCloud",
		Body: "When WhatsApp opens it may offer to restore your chat history from iCloud.\n\n" +
			"Decline it.\n\n" +
			"Accepting replaces the entire message history — including everything just\n" +
			"merged in — with whatever iCloud happens to hold. This is the single way to\n" +
			"undo the whole exercise in one tap, and it is offered as though it were\n" +
			"helpful.",
		Expect: "WhatsApp opens on the conversation list with the merged history in it.",
	},
	{
		ID:    "reverify-number",
		Stage: After,
		Title: "WhatsApp may ask for the phone number again",
		Body: "It sometimes asks to verify the number by text message. Keep the SIM in the\n" +
			"phone until this is done, or the message cannot arrive.\n\n" +
			"Verifying the number does not touch the messages already there.",
	},
	{
		ID:    "apps-and-passcode",
		Stage: After,
		Title: "The phone will finish itself",
		Body: "Apps are downloaded again from the App Store rather than restored, so the\n" +
			"phone needs the Apple ID password and Wi-Fi, and will look half-empty for a\n" +
			"while. The passcode is not set; set it again.\n\n" +
			"Saved passwords, Wi-Fi networks and Health data are not there, for the reason\n" +
			"given before any of this started.",
		Takes: time.Hour,
	},
	{
		ID:    "find-my-back-on",
		Stage: After,
		Title: "Turn Find My iPhone back on",
		Body: "Settings, your name, Find My, Find My iPhone, on.\n\n" +
			"A phone without it is a phone that cannot be found or wiped if it is lost. It\n" +
			"was turned off for one hour and for one reason, and this is that reason ending.",
	},
	{
		ID:    "check-it-worked",
		Stage: After,
		Title: "Check it actually worked",
		Body: "Four things, in a minute:\n\n" +
			"  * The number of conversations looks right.\n" +
			"  * Open the oldest conversation you moved and look at its first message.\n" +
			"    The date should be the one the report said.\n" +
			"  * Names appear rather than phone numbers, in the conversations that had them.\n" +
			"  * Pictures that came across appear as a line of text saying what was sent.\n" +
			"    That is expected: the files themselves are not in the backup.\n\n" +
			"If any of those is wrong, do not take another backup. The archived one is\n" +
			"still the way back, and taking a backup now is the thing that would spoil it.",
	},

	// ----------------------------------------------------------------- wrong
	{
		// Not marked as losing something if skipped: it is the remedy rather than a
		// precaution, and the whole of this stage is shown together anyway. Marking
		// everything important would leave the mark meaning nothing.
		ID:    "go-back",
		Stage: Wrong,
		Title: "Going back",
		Body: "The archived encrypted backup from the first step is the way back, and it is\n" +
			"complete: everything as it was, Health data and passwords included.\n\n" +
			"In Finder, with the phone selected, choose Restore Backup and pick the\n" +
			"archived one. It will ask for the password set at the start.\n\n" +
			"Do this before taking any new backup. A new backup does not overwrite an\n" +
			"archived one, but it is not worth finding out otherwise while upset.",
		Takes: 45 * time.Minute,
	},
	{
		ID:    "error-211",
		Stage: Wrong,
		Title: "If the restore stopped with error 211",
		Body: "Find My iPhone was still on. Nothing was written to the phone and nothing has\n" +
			"been lost.\n\n" +
			"Turn it off and start the restore again.",
	},
	{
		ID:    "wrong-backup",
		Stage: Wrong,
		Title: "If Finder will not show the changed backup",
		Body: "Finder lists backups by device and date. The changed one is a copy of the\n" +
			"original with today's date, in the same place Finder keeps the rest.\n\n" +
			"If it is not listed, Finder has not noticed it. Quit Finder and open it again\n" +
			"with the phone connected.",
	},
}
