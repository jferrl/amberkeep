package android

import (
	"fmt"
	"strings"

	"github.com/jferrl/amberkeep/internal/model"
)

// Turning a notice into a sentence.
//
// The action codes come from IPED's Android extractor, the most complete public
// decoder and the only one verified against real devices, cross-checked with
// WhatsApp-Chat-Exporter, whapa and a decompiled build of the app. Each was then
// checked against a real archive, where the auxiliary table every code accompanies
// matches what those sources predict.
//
// Five codes that appear in real archives are deliberately left unphrased, because
// only one weak source names them and a wrong sentence is worse than an honest
// description. Those become a plain statement of what was recovered.
//
// The wording is English and plain. It exists so an export has something readable
// today; a translated interface should phrase model.Notice itself, which is why the
// facts and the words were kept apart.
const (
	actionSubjectChanged      = 1   // the previous subject is stored; the new one is the message text
	actionAddedToGroup        = 4   // an older encoding of adding someone
	actionLeftGroup           = 5   //
	actionPhotoChanged        = 6   //
	actionRemovedYou          = 7   //
	actionNumberChanged       = 10  //
	actionGroupCreated        = 11  //
	actionAddedToGroupNew     = 12  //
	actionLeftGroupAlt        = 13  //
	actionRemovedFromGroup    = 14  //
	actionYouAreAdmin         = 15  //
	actionYouAreNotAdmin      = 16  //
	actionSecurityCode        = 18  // recurs across nearly every conversation, with no detail table
	actionJoinedViaLink       = 20  //
	actionDescriptionChanged  = 27  //
	actionNumberChangedAlt    = 28  //
	actionEphemeralChanged    = 56  //
	actionDeviceChanged       = 57  //
	actionBlockedContact      = 58  //
	actionEncryptionBanner    = 67  // the "end-to-end encrypted" banner at the head of a chat
	actionBusinessNotice      = 69  //
	actionGroupCallStarted    = 70  //
	actionJoinedFromCommunity = 79  //
	actionCommunityAction     = 110 //
	actionMessagePinned       = 118 //
	actionSenderInContacts    = 129 //
	actionJoinedWhatsApp      = 136 //
)

// unidentifiedActions are the codes that appear in real archives and that no
// reliable source names. They are listed so the reason is written down rather than
// rediscovered, and so a future release can resolve them deliberately.
//
//	2   possibly a group creation, but that contradicts code 11
//	83  possibly a request to join, from one weak source
//	109 and 111 sit in the community-management range, unnamed
//	165 is newer than every published table
var unidentifiedActions = map[int]string{
	2: "", 83: "", 109: "", 111: "", 165: "",
}

// phraseNotice renders what happened, in English, from what was recovered.
func phraseNotice(n model.Notice, dir *model.Directory) string {
	actor := nameOrNothing(dir, n.Actor)
	targets := names(dir, n.Targets)

	switch n.Action {
	case actionSecurityCode:
		if actor != "" {
			return fmt.Sprintf("Your security code with %s changed", actor)
		}
		return "The security code changed"

	case actionEncryptionBanner:
		if n.Business != "" {
			return fmt.Sprintf("%s uses a secure service from Meta to manage this chat", n.Business)
		}
		return "Messages and calls are end-to-end encrypted"

	case actionAddedToGroup, actionAddedToGroupNew:
		// WhatsApp marks a notice about the archive's owner differently, and it
		// reads backwards if that is ignored: the owner appears to have added
		// themselves.
		if n.Joined {
			return withActor(actor, "added you", "You were added")
		}
		if len(targets) > 0 {
			return withActor(actor, "added "+list(targets), list(targets)+" were added")
		}
		return withActor(actor, "added someone", "Someone was added")

	case actionJoinedViaLink:
		if n.Joined {
			return "You joined using an invite link"
		}
		if len(targets) > 0 {
			return list(targets) + " joined using an invite link"
		}
		return "Someone joined using an invite link"

	case actionJoinedFromCommunity:
		who := list(targets)
		if n.Joined {
			who = "You"
		}
		if who == "" {
			who = "Someone"
		}
		if n.Subject != "" {
			return fmt.Sprintf("%s joined this group from the community %q", who, n.Subject)
		}
		return who + " joined this group from the community"

	case actionLeftGroup, actionLeftGroupAlt:
		if len(targets) > 0 {
			return list(targets) + " left"
		}
		return withActor(actor, "left", "Someone left")

	case actionRemovedFromGroup:
		if len(targets) > 0 {
			return withActor(actor, "removed "+list(targets), list(targets)+" were removed")
		}
		return withActor(actor, "removed someone", "Someone was removed")

	case actionRemovedYou:
		return withActor(actor, "removed you", "You were removed")

	case actionYouAreAdmin:
		return "You are now an administrator"
	case actionYouAreNotAdmin:
		return "You are no longer an administrator"

	case actionGroupCreated:
		if n.New != "" {
			return withActor(actor, fmt.Sprintf("created the group %q", n.New), fmt.Sprintf("The group %q was created", n.New))
		}
		return withActor(actor, "created this group", "This group was created")

	case actionSubjectChanged:
		switch {
		case n.Old != "" && n.New != "":
			return withActor(actor,
				fmt.Sprintf("changed the subject from %q to %q", n.Old, n.New),
				fmt.Sprintf("The subject changed from %q to %q", n.Old, n.New))
		case n.New != "":
			return withActor(actor,
				fmt.Sprintf("changed the subject to %q", n.New),
				fmt.Sprintf("The subject changed to %q", n.New))
		default:
			return withActor(actor, "changed the subject", "The subject changed")
		}

	case actionDescriptionChanged:
		if n.New != "" {
			return withActor(actor,
				fmt.Sprintf("changed the group description to %q", n.New),
				fmt.Sprintf("The group description changed to %q", n.New))
		}
		return withActor(actor, "changed the group description", "The group description changed")

	case actionPhotoChanged:
		return withActor(actor, "changed the group photo", "The group photo changed")

	case actionNumberChanged, actionNumberChangedAlt:
		switch {
		case n.Old != "" && n.New != "":
			return fmt.Sprintf("%s changed their phone number, and is now %s", n.Old, n.New)
		case n.Old != "":
			return fmt.Sprintf("%s changed their phone number", n.Old)
		default:
			return "A contact changed their phone number"
		}

	case actionDeviceChanged:
		return phraseDeviceChange(actor, n)

	case actionEphemeralChanged:
		return withActor(actor, "changed the disappearing messages setting",
			"The disappearing messages setting changed")

	case actionBlockedContact:
		if n.Blocked {
			return "You blocked this contact"
		}
		return "You unblocked this contact"

	case actionBusinessNotice:
		if n.Business != "" {
			return fmt.Sprintf("%s uses a secure service from Meta to manage this chat", n.Business)
		}
		return "This business uses a secure service from Meta to manage this chat"

	case actionGroupCallStarted:
		return withActor(actor, "started a call", "A call was started")

	case actionMessagePinned:
		return withActor(actor, "pinned a message", "A message was pinned")

	case actionSenderInContacts:
		if actor != "" {
			return fmt.Sprintf("%s is in your contacts", actor)
		}
		return "This contact is in your address book"

	case actionJoinedWhatsApp:
		if actor != "" {
			return actor + " joined WhatsApp"
		}
		return "A contact joined WhatsApp"

	case actionCommunityAction:
		if n.Subject != "" {
			return fmt.Sprintf("A community change involving %q", n.Subject)
		}
		return "A community was changed"
	}

	return describeUnidentified(n, actor, targets)
}

// phraseDeviceChange reports how many linked devices changed.
func phraseDeviceChange(actor string, n model.Notice) string {
	var parts []string
	if n.DevicesAdded > 0 {
		parts = append(parts, "added "+count(n.DevicesAdded, "device", "devices"))
	}
	if n.DevicesRemoved > 0 {
		parts = append(parts, "removed "+count(n.DevicesRemoved, "device", "devices"))
	}
	if len(parts) == 0 {
		return withActor(actor, "changed their linked devices", "Linked devices changed")
	}
	joined := strings.Join(parts, " and ")
	return withActor(actor, joined, "Linked devices were "+joined)
}

// describeUnidentified states the facts of a notice this build cannot phrase.
//
// It reads as a description rather than as something somebody said, so nobody
// mistakes it for a claim about what happened. Five codes reach this deliberately;
// anything else reaching it is a new WhatsApp release worth investigating.
func describeUnidentified(n model.Notice, actor string, targets []string) string {
	var detail []string
	if len(targets) > 0 {
		detail = append(detail, list(targets))
	}
	if n.Subject != "" {
		detail = append(detail, fmt.Sprintf("group %q", n.Subject))
	}
	if n.Business != "" {
		detail = append(detail, n.Business)
	}
	switch {
	case n.Old != "" && n.New != "":
		detail = append(detail, fmt.Sprintf("%q became %q", n.Old, n.New))
	case n.New != "":
		detail = append(detail, fmt.Sprintf("%q", n.New))
	case n.Old != "":
		detail = append(detail, fmt.Sprintf("was %q", n.Old))
	}

	head := fmt.Sprintf("Unrecognised notice, code %d", n.Action)
	if actor != "" {
		head = fmt.Sprintf("Unrecognised notice from %s, code %d", actor, n.Action)
	}
	if len(detail) == 0 {
		return head
	}
	return head + ": " + strings.Join(detail, "; ")
}

// IsIdentifiedNotice reports whether this build can phrase a notice code, so a
// report can say honestly how much of an archive it understood.
func IsIdentifiedNotice(action int) bool {
	if _, unresolved := unidentifiedActions[action]; unresolved {
		return false
	}
	return phraseNotice(model.Notice{Action: action}, nil) != describeUnidentified(
		model.Notice{Action: action}, "", nil)
}

// withActor picks between a sentence that names who acted and one that does not.
func withActor(actor, withName, withoutName string) string {
	if actor == "" {
		return withoutName
	}
	return actor + " " + withName
}

// list renders several names as "a, b and c".
func list(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// nameOrNothing resolves a person to the best label available, which may be a
// phone number rather than a name.
//
// It returns an empty string only when there is genuinely nobody to name: the
// message was the owner's own, or no directory was supplied. Dropping somebody
// from a sentence merely because they were never saved as a contact would lose
// information the archive actually has, and in a real database most people in a
// group are never saved.
func nameOrNothing(dir *model.Directory, j model.JID) string {
	if j.IsZero() || dir == nil {
		return ""
	}
	return dir.NameOf(j)
}

// names resolves several people for a list.
func names(dir *model.Directory, jids []model.JID) []string {
	if dir == nil {
		return nil
	}
	out := make([]string, 0, len(jids))
	for _, j := range jids {
		out = append(out, dir.NameOf(j))
	}
	return out
}

// count renders a number with the right form of its noun.
func count(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}
