package android

import (
	"strings"
	"testing"

	"github.com/jferrl/amberkeep/internal/model"
)

// phrasingDirectory returns a directory where one person has a saved name and
// another has only a number, which is what a real archive looks like.
func phrasingDirectory() *model.Directory {
	d := model.NewDirectory()
	d.Add(model.Contact{JID: model.ParseJID("34600111222@s.whatsapp.net"), Name: "Alice"})
	d.Add(model.Contact{JID: model.ParseJID("34600333444@s.whatsapp.net")})
	return d
}

var (
	alice   = model.ParseJID("34600111222@s.whatsapp.net")
	unnamed = model.ParseJID("34600333444@s.whatsapp.net")
)

func TestPhraseNotice(t *testing.T) {
	t.Parallel()

	dir := phrasingDirectory()

	tests := []struct {
		name   string
		notice model.Notice
		want   string
	}{
		{
			name:   "a security code change names the other person",
			notice: model.Notice{Action: actionSecurityCode, Actor: alice},
			want:   "Your security code with Alice changed",
		},
		{
			name:   "a security code change with nobody to name",
			notice: model.Notice{Action: actionSecurityCode},
			want:   "The security code changed",
		},
		{
			name:   "the encryption banner",
			notice: model.Notice{Action: actionEncryptionBanner},
			want:   "Messages and calls are end-to-end encrypted",
		},
		{
			name:   "the encryption banner for a business",
			notice: model.Notice{Action: actionEncryptionBanner, Business: "A shop"},
			want:   "A shop uses a secure service from Meta to manage this chat",
		},
		{
			name:   "one person added",
			notice: model.Notice{Action: actionAddedToGroupNew, Actor: alice, Targets: []model.JID{unnamed}},
			want:   "Alice added +34600333444",
		},
		{
			name:   "two people added are joined with 'and'",
			notice: model.Notice{Action: actionAddedToGroup, Actor: alice, Targets: []model.JID{unnamed, alice}},
			want:   "Alice added +34600333444 and Alice",
		},
		{
			name: "the archive owner being added reads the right way round",
			// Without honouring this flag the owner appears to have added themselves.
			notice: model.Notice{Action: actionAddedToGroupNew, Actor: alice, Joined: true},
			want:   "Alice added you",
		},
		{
			name:   "somebody joined using a link",
			notice: model.Notice{Action: actionJoinedViaLink, Targets: []model.JID{unnamed}},
			want:   "+34600333444 joined using an invite link",
		},
		{
			name:   "the owner joined using a link",
			notice: model.Notice{Action: actionJoinedViaLink, Joined: true},
			want:   "You joined using an invite link",
		},
		{
			name:   "somebody joined from a community",
			notice: model.Notice{Action: actionJoinedFromCommunity, Targets: []model.JID{unnamed}, Subject: "Neighbours"},
			want:   `+34600333444 joined this group from the community "Neighbours"`,
		},
		{
			name:   "somebody left",
			notice: model.Notice{Action: actionLeftGroup, Targets: []model.JID{alice}},
			want:   "Alice left",
		},
		{
			name:   "somebody was removed",
			notice: model.Notice{Action: actionRemovedFromGroup, Actor: alice, Targets: []model.JID{unnamed}},
			want:   "Alice removed +34600333444",
		},
		{
			name:   "the owner was removed",
			notice: model.Notice{Action: actionRemovedYou, Actor: alice},
			want:   "Alice removed you",
		},
		{
			name:   "the owner became an administrator",
			notice: model.Notice{Action: actionYouAreAdmin},
			want:   "You are now an administrator",
		},
		{
			name:   "the owner stopped being an administrator",
			notice: model.Notice{Action: actionYouAreNotAdmin},
			want:   "You are no longer an administrator",
		},
		{
			name:   "a group was created with a name",
			notice: model.Notice{Action: actionGroupCreated, Actor: alice, New: "Book club"},
			want:   `Alice created the group "Book club"`,
		},
		{
			name:   "a group was created by nobody we can name",
			notice: model.Notice{Action: actionGroupCreated},
			want:   "This group was created",
		},
		{
			name:   "a subject changed, showing both values",
			notice: model.Notice{Action: actionSubjectChanged, Actor: alice, Old: "Before", New: "After"},
			want:   `Alice changed the subject from "Before" to "After"`,
		},
		{
			name:   "a subject changed with only the new value",
			notice: model.Notice{Action: actionSubjectChanged, Actor: alice, New: "After"},
			want:   `Alice changed the subject to "After"`,
		},
		{
			name:   "a subject changed with nothing recorded",
			notice: model.Notice{Action: actionSubjectChanged},
			want:   "The subject changed",
		},
		{
			name:   "a description changed",
			notice: model.Notice{Action: actionDescriptionChanged, Actor: alice, New: "Read the rules"},
			want:   `Alice changed the group description to "Read the rules"`,
		},
		{
			name:   "a group photo changed",
			notice: model.Notice{Action: actionPhotoChanged, Actor: alice},
			want:   "Alice changed the group photo",
		},
		{
			name:   "somebody changed their number",
			notice: model.Notice{Action: actionNumberChanged, Old: "Alice", New: "+34600999888"},
			want:   "Alice changed their phone number, and is now +34600999888",
		},
		{
			name:   "somebody changed their number, with only the old one known",
			notice: model.Notice{Action: actionNumberChangedAlt, Old: "Alice"},
			want:   "Alice changed their phone number",
		},
		{
			name:   "linked devices were added and removed",
			notice: model.Notice{Action: actionDeviceChanged, Actor: alice, DevicesAdded: 2, DevicesRemoved: 1},
			want:   "Alice added 2 devices and removed 1 device",
		},
		{
			name:   "linked devices changed with no counts",
			notice: model.Notice{Action: actionDeviceChanged, Actor: alice},
			want:   "Alice changed their linked devices",
		},
		{
			name:   "a contact was blocked",
			notice: model.Notice{Action: actionBlockedContact, Blocked: true},
			want:   "You blocked this contact",
		},
		{
			name:   "a contact was unblocked",
			notice: model.Notice{Action: actionBlockedContact},
			want:   "You unblocked this contact",
		},
		{
			name:   "the disappearing messages setting changed",
			notice: model.Notice{Action: actionEphemeralChanged, Actor: alice},
			want:   "Alice changed the disappearing messages setting",
		},
		{
			name:   "a business notice",
			notice: model.Notice{Action: actionBusinessNotice, Business: "A shop"},
			want:   "A shop uses a secure service from Meta to manage this chat",
		},
		{
			name:   "a call was started",
			notice: model.Notice{Action: actionGroupCallStarted, Actor: alice},
			want:   "Alice started a call",
		},
		{
			name:   "a message was pinned",
			notice: model.Notice{Action: actionMessagePinned, Actor: alice},
			want:   "Alice pinned a message",
		},
		{
			name:   "a contact is in the address book",
			notice: model.Notice{Action: actionSenderInContacts, Actor: alice},
			want:   "Alice is in your contacts",
		},
		{
			name:   "a contact joined WhatsApp",
			notice: model.Notice{Action: actionJoinedWhatsApp, Actor: alice},
			want:   "Alice joined WhatsApp",
		},
		{
			name:   "a community changed",
			notice: model.Notice{Action: actionCommunityAction, Subject: "Neighbours"},
			want:   `A community change involving "Neighbours"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := phraseNotice(tt.notice, dir); got != tt.want {
				t.Errorf("phraseNotice() =\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}

// TestUnidentifiedNoticesAreDescribedNotInvented guards the decision that matters
// most for honesty. Five action codes appear in real archives that no reliable
// source names, and a plausible-sounding wrong sentence would mislabel real events.
func TestUnidentifiedNoticesAreDescribedNotInvented(t *testing.T) {
	t.Parallel()

	dir := phrasingDirectory()

	tests := []struct {
		name   string
		notice model.Notice
		want   string
	}{
		{
			name:   "nothing but a code",
			notice: model.Notice{Action: 165},
			want:   "Unrecognised notice, code 165",
		},
		{
			name:   "a code with somebody behind it",
			notice: model.Notice{Action: 109, Actor: alice},
			want:   "Unrecognised notice from Alice, code 109",
		},
		{
			name:   "a code with people affected",
			notice: model.Notice{Action: 111, Targets: []model.JID{alice, unnamed}},
			want:   "Unrecognised notice, code 111: Alice and +34600333444",
		},
		{
			name:   "a code with a value that changed",
			notice: model.Notice{Action: 2, Old: "before", New: "after"},
			want:   `Unrecognised notice, code 2: "before" became "after"`,
		},
		{
			name:   "a code with only a new value",
			notice: model.Notice{Action: 83, New: "after"},
			want:   `Unrecognised notice, code 83: "after"`,
		},
		{
			name:   "a code with only an old value",
			notice: model.Notice{Action: 83, Old: "before"},
			want:   `Unrecognised notice, code 83: was "before"`,
		},
		{
			name:   "a code naming a group and a business",
			notice: model.Notice{Action: 999, Subject: "Neighbours", Business: "A shop"},
			want:   `Unrecognised notice, code 999: group "Neighbours"; A shop`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := phraseNotice(tt.notice, dir)
			if got != tt.want {
				t.Errorf("phraseNotice() =\n  %q\nwant\n  %q", got, tt.want)
			}
			// The wording must read as a description, never as a claim about what
			// somebody did.
			if !strings.HasPrefix(got, "Unrecognised notice") {
				t.Errorf("an unidentified notice was phrased as though it were understood: %q", got)
			}
		})
	}
}

func TestIsIdentifiedNotice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action int
		want   bool
	}{
		{name: "a code with a confirmed meaning", action: actionSecurityCode, want: true},
		{name: "another confirmed code", action: actionSubjectChanged, want: true},
		{name: "a code only one weak source names", action: 165, want: false},
		{name: "a community code nobody names", action: 109, want: false},
		{name: "a code nobody has ever seen", action: 9999, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsIdentifiedNotice(tt.action); got != tt.want {
				t.Errorf("IsIdentifiedNotice(%d) = %v, want %v", tt.action, got, tt.want)
			}
		})
	}
}

func TestListJoinsNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		names []string
		want  string
	}{
		{name: "nobody", names: nil, want: ""},
		{name: "one", names: []string{"Alice"}, want: "Alice"},
		{name: "two", names: []string{"Alice", "Bob"}, want: "Alice and Bob"},
		{name: "three", names: []string{"Alice", "Bob", "Carol"}, want: "Alice, Bob and Carol"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := list(tt.names); got != tt.want {
				t.Errorf("list() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCountUsesTheRightNoun(t *testing.T) {
	t.Parallel()

	if got := count(1, "device", "devices"); got != "1 device" {
		t.Errorf("count(1) = %q", got)
	}
	if got := count(3, "device", "devices"); got != "3 devices" {
		t.Errorf("count(3) = %q", got)
	}
}

// TestPhrasingSurvivesAMissingDirectory checks that phrasing never panics when no
// directory is available, which is the case when a notice is phrased in isolation.
func TestPhrasingSurvivesAMissingDirectory(t *testing.T) {
	t.Parallel()

	for _, action := range []int{
		actionSecurityCode, actionAddedToGroupNew, actionSubjectChanged,
		actionDeviceChanged, actionGroupCreated, 165,
	} {
		notice := model.Notice{Action: action, Actor: alice, Targets: []model.JID{unnamed}}
		if got := phraseNotice(notice, nil); got == "" {
			t.Errorf("phraseNotice(%d) with no directory produced nothing", action)
		}
	}
}
