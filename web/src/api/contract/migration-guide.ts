// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/migration/guide — the words, served rather than copied into the page,
 * so that correcting a sentence corrects it everywhere.
 */
export default {
  "sentences": {
    "a-day-ago": "taken a day ago; anything said on the phone since is not in it",
    "all-already-there": "every message is already on the iPhone",
    "an-hour-ago": "taken an hour ago",
    "cannot-see-safety": "nothing here can see this; it is the only way back and it has to be done by hand",
    "days-ago": "taken {days} days ago; anything said on the phone since is not in it",
    "groups-not-included": "groups were not included",
    "has-room": "There is room for a copy of the backup",
    "hidden-identity": "it has only a hidden identity, so it cannot be matched to a conversation on the iPhone",
    "holds-messages": "The backup holds WhatsApp's messages",
    "hours-ago": "taken {hours} hours ago",
    "is-a-backup": "That folder is a backup",
    "is-encrypted": "it is encrypted, and an encrypted backup is sealed with a key that never leaves the phone",
    "is-recent": "The backup is recent",
    "it-is-empty": "it is empty",
    "needs-space": "it needs about {size} GB free",
    "no-backup-files": "it has none of the four files every Finder, iTunes and Apple Devices backup has",
    "no-date": "it does not say when it was taken",
    "no-store": "there is no {file} in it",
    "not-a-conversation": "only conversations and groups can be moved",
    "not-encrypted": "The backup is not encrypted",
    "nothing-carries": "nothing in it can be carried across",
    "safety-backup": "A safety backup exists and has been archived",
    "thanks-ask": "If this helped, you can buy me a coffee at {where}. It is voluntary, and nothing here depends on it.",
    "warn-folded": "{conversations} conversations turned out to be the same person the iPhone already has under another name, and will be written into the conversation that is already there rather than added beside it.",
    "warn-folded-one": "One conversation turned out to be the same person the iPhone already has under another name, and will be written into the conversation that is already there rather than added beside it.",
    "warn-groups-left-out": "{groups} groups were left out because groups were not included.",
    "warn-groups-left-out-one": "One group was left out because groups were not included.",
    "warn-hidden-left-out": "{conversations} conversations could not be matched to anyone, because WhatsApp hides some people behind an identifier this archive has no phone number for. They were left out rather than added as somebody new.",
    "warn-hidden-left-out-one": "One conversation could not be matched to anyone, because WhatsApp hides some people behind an identifier this archive has no phone number for. It was left out rather than added as somebody new.",
    "warn-no-pairings": "The iPhone files {conversations} conversations under a hidden identity rather than a phone number, and WhatsApp's own record of which is which was not supplied. Anybody in that position who is known by their number on the Android will arrive as a second conversation rather than joining the one already there. Supply LID.sqlite from the same backup to avoid it.",
    "warn-no-pairings-one": "The iPhone files one conversation under a hidden identity rather than a phone number, and WhatsApp's own record of which is which was not supplied. Anybody in that position who is known by their number on the Android will arrive as a second conversation rather than joining the one already there. Supply LID.sqlite from the same backup to avoid it.",
    "warn-placeholders": "{messages} messages will arrive as a line of text saying what was sent, not as the picture, recording or file itself. Those files are not in the backup this reads, and nothing here can invent them.",
    "warn-placeholders-one": "One message will arrive as a line of text saying what was sent, not as the picture, recording or file itself. That file is not in the backup this reads, and nothing here can invent it.",
    "warn-untranslatable": "{entries} entries will not come across at all: call history, and the notices WhatsApp writes into a conversation about itself. Nothing anybody typed is in that number.",
    "warn-untranslatable-one": "One entry will not come across at all: call history, or a notice WhatsApp wrote into a conversation about itself. Nothing anybody typed is in that number.",
    "within-the-hour": "taken within the hour"
  },
  "stages": [
    {
      "heading": "Before you restore",
      "stage": "before",
      "steps": [
        {
          "body": "Connect the phone, open Finder, choose the phone in the sidebar, and back it\nup to this Mac with \"Encrypt local backup\" switched ON. Set a password you\nwill not lose.\n\nThen right-click that backup in Finder's list and choose Archive.\n\nArchiving is the part people skip and the part that matters. Finder keeps one\nbackup per phone and writes over it the next time. Without archiving, the very\nnext backup — the one this process is about to make — replaces the only copy\nof everything as it was. Archiving puts it somewhere Finder will leave alone.\n\nEncrypted, because only an encrypted backup carries Health data, saved\npasswords and Wi-Fi networks. This is the copy you would go back to.",
          "critical": true,
          "expect": "the backup appears in Finder's list with a padlock and a date. Check the padlock\nis there before going on: no padlock means it was not encrypted, and Health data\nand passwords are not in it.",
          "id": "safety-backup",
          "minutes": 30,
          "title": "Make a safety backup, and archive it"
        },
        {
          "body": "The backup this restores from has to be unencrypted, because an encrypted one\nis sealed with a key that never leaves the phone and nothing on a computer can\nopen it.\n\nAn unencrypted backup does not contain Health data, saved passwords, Wi-Fi\nnetworks, or the keychain. Restoring one means losing those, and there is no\nway around it: it is Apple's design, not a limitation of this program.\n\nIf Health data matters more than the message history, stop here. The archived\nencrypted backup from the previous step still has everything, and restoring\nthat puts the phone back exactly as it is now.",
          "critical": true,
          "expect": "nothing yet. This is a decision, not an action.",
          "id": "health-data-goes",
          "title": "Know what an unencrypted backup does not carry"
        },
        {
          "body": "In Finder, with the phone selected, untick \"Encrypt local backup\".\n\nIt will ask for the password you set a moment ago. This does not remove the\narchived backup — that one stays encrypted and stays where it is.",
          "expect": "the tick disappears. Finder may start a new backup on its own; let it finish.",
          "id": "encryption-off",
          "title": "Turn off encrypted backups"
        },
        {
          "body": "On the phone: Settings, your name at the top, Find My, Find My iPhone, and\nturn it off. It asks for the Apple ID password.\n\nA restore with Find My still on stops at the very end with \"error 211\" and\nnothing else. Nothing is written and nothing is lost, but it is an hour spent\nfor a number instead of a sentence.\n\nWrite down that you turned it off. It has to go back on afterwards, and by\nthen it is easy to forget it was ever off.",
          "expect": "the switch goes grey and the phone confirms it.",
          "id": "find-my-off",
          "title": "Turn off Find My iPhone"
        },
        {
          "body": "Back up to this Mac once more. This is the backup that will be changed and\nrestored, and it must be recent: anything said on the phone after this point\nwill not be in it and will be gone after the restore.\n\nSay nothing on the phone from here until the restore has finished.",
          "expect": "Finder shows a new date with no padlock beside it.",
          "id": "fresh-backup",
          "minutes": 20,
          "title": "Back the phone up again, unencrypted this time"
        },
        {
          "body": "The phone on its cable and the computer on mains power. A restore that is\ninterrupted by a flat battery is the one way this goes badly wrong.\n\nThe computer needs free space for a second copy of the backup, which is as\nlarge as the first.",
          "id": "power-and-space",
          "title": "Plug both in, and check there is room"
        }
      ]
    },
    {
      "heading": "Restoring, and what you will see",
      "stage": "restoring",
      "steps": [
        {
          "body": "In Finder, with the phone selected, choose Restore Backup and pick the one\nthis program made. Its date is today's.\n\nNothing you do on the computer from here will change the result. The phone is\nbeing written to; leave it connected and leave it alone.",
          "expect": "a progress bar on the computer first, then \"Restore in progress\" on the phone.",
          "id": "start-restore",
          "minutes": 45,
          "title": "Restore the changed backup"
        },
        {
          "body": "Partway through, the phone shows the Apple logo with a progress bar that does\nnot move for a long time. This is normal and it is the longest part.\n\nDo not unplug it. Do not hold the power button. A phone interrupted here is a\nphone that has to be restored again from the beginning.",
          "expect": "the bar eventually moves, the phone restarts a second time, and Setup\nAssistant appears asking about language and Wi-Fi.",
          "id": "looks-stalled",
          "minutes": 25,
          "title": "The part that looks like it has frozen"
        }
      ]
    },
    {
      "heading": "Once the phone comes back",
      "stage": "after",
      "steps": [
        {
          "body": "When WhatsApp opens it may offer to restore your chat history from iCloud.\n\nDecline it.\n\nAccepting replaces the entire message history — including everything just\nmerged in — with whatever iCloud happens to hold. This is the single way to\nundo the whole exercise in one tap, and it is offered as though it were\nhelpful.",
          "critical": true,
          "expect": "WhatsApp opens on the conversation list with the merged history in it.",
          "id": "decline-icloud",
          "title": "Say no to restoring chats from iCloud"
        },
        {
          "body": "It sometimes asks to verify the number by text message. Keep the SIM in the\nphone until this is done, or the message cannot arrive.\n\nVerifying the number does not touch the messages already there.",
          "id": "reverify-number",
          "title": "WhatsApp may ask for the phone number again"
        },
        {
          "body": "Apps are downloaded again from the App Store rather than restored, so the\nphone needs the Apple ID password and Wi-Fi, and will look half-empty for a\nwhile. The passcode is not set; set it again.\n\nSaved passwords, Wi-Fi networks and Health data are not there, for the reason\ngiven before any of this started.",
          "id": "apps-and-passcode",
          "minutes": 60,
          "title": "The phone will finish itself"
        },
        {
          "body": "Settings, your name, Find My, Find My iPhone, on.\n\nA phone without it is a phone that cannot be found or wiped if it is lost. It\nwas turned off for one hour and for one reason, and this is that reason ending.",
          "id": "find-my-back-on",
          "title": "Turn Find My iPhone back on"
        },
        {
          "body": "Four things, in a minute:\n\n  * The number of conversations looks right.\n  * Open the oldest conversation you moved and look at its first message.\n    The date should be the one the report said.\n  * Names appear rather than phone numbers, in the conversations that had them.\n  * Pictures that came across appear as a line of text saying what was sent.\n    That is expected: the files themselves are not in the backup.\n\nIf any of those is wrong, do not take another backup. The archived one is\nstill the way back, and taking a backup now is the thing that would spoil it.",
          "id": "check-it-worked",
          "title": "Check it actually worked"
        }
      ]
    },
    {
      "heading": "If it did not work",
      "stage": "wrong",
      "steps": [
        {
          "body": "The archived encrypted backup from the first step is the way back, and it is\ncomplete: everything as it was, Health data and passwords included.\n\nIn Finder, with the phone selected, choose Restore Backup and pick the\narchived one. It will ask for the password set at the start.\n\nDo this before taking any new backup. A new backup does not overwrite an\narchived one, but it is not worth finding out otherwise while upset.",
          "id": "go-back",
          "minutes": 45,
          "title": "Going back"
        },
        {
          "body": "Find My iPhone was still on. Nothing was written to the phone and nothing has\nbeen lost.\n\nTurn it off and start the restore again.",
          "id": "error-211",
          "title": "If the restore stopped with error 211"
        },
        {
          "body": "Finder lists backups by device and date. The changed one is a copy of the\noriginal with today's date, in the same place Finder keeps the rest.\n\nIf it is not listed, Finder has not noticed it. Quit Finder and open it again\nwith the phone connected.",
          "id": "wrong-backup",
          "title": "If Finder will not show the changed backup"
        }
      ]
    }
  ]
} as const;
