package migrate

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jferrl/amberkeep/internal/model"
)

func TestAConversationThePhoneHasNeverSeen(t *testing.T) {
	t.Parallel()

	from := history(said(1, "K1", 0, "one"), said(2, "K2", 1, "two"), said(3, "K3", 2, "three"))
	plan, err := Build(context.Background(), from, buildPhone(t).open(t, ""), Options{})
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if plan.Creating != 1 || plan.Merging != 0 {
		t.Errorf("creating=%d merging=%d, want one created and none merged", plan.Creating, plan.Merging)
	}
	if plan.Adding != 3 {
		t.Errorf("adding=%d, want all three", plan.Adding)
	}
	accounted(t, plan, 3)

	if got := plan.Conversations[0].Into; got != "" {
		t.Errorf("it claims to be merging into %q", got)
	}
}

func TestAConversationTheyBothHave(t *testing.T) {
	t.Parallel()

	// The phone already holds the first two of the three.
	p := buildPhone(t)
	p.holds(t, ana.String(), 0, "K1", "K2")

	from := history(said(1, "K1", 0, "one"), said(2, "K2", 1, "two"), said(3, "K3", 2, "three"))
	plan, err := Build(context.Background(), from, p.open(t, ""), Options{})
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if plan.Merging != 1 || plan.Creating != 0 {
		t.Errorf("merging=%d creating=%d, want one merged and none created", plan.Merging, plan.Creating)
	}
	if plan.Adding != 1 || plan.AlreadyThere != 2 {
		t.Errorf("adding=%d already=%d, want one added and two left alone", plan.Adding, plan.AlreadyThere)
	}
	accounted(t, plan, 3)

	c := plan.Conversations[0]
	if !c.Merging() || c.Into != ana.String() {
		t.Errorf("it is not merging into the conversation that is there: %+v", c)
	}
	if c.OnPhoneAlready != 2 {
		t.Errorf("on_phone_already=%d, want the two the phone holds", c.OnPhoneAlready)
	}

	// The one line somebody reads before deciding. It has to name what arrives and
	// say, in the same breath, that nothing already there is disturbed — because
	// that is the question everybody actually has.
	said := plan.Summary()
	for _, want := range []string{"1 message", "1 conversation", "2 messages already there"} {
		if !strings.Contains(said, want) {
			t.Errorf("the summary does not say %q: %q", want, said)
		}
	}
}

// TestRunningItTwiceWouldAddNothing is the property the whole design rests on.
//
// A migration somebody is unsure about has to be safe to think about twice. Because
// a message is recognised by WhatsApp's own identifier, which is the same on both
// phones, a second run over a phone that already has everything plans to write
// nothing at all.
func TestRunningItTwiceWouldAddNothing(t *testing.T) {
	t.Parallel()

	p := buildPhone(t)
	p.holds(t, ana.String(), 0, "K1", "K2", "K3")

	from := history(said(1, "K1", 0, "one"), said(2, "K2", 1, "two"), said(3, "K3", 2, "three"))
	plan, err := Build(context.Background(), from, p.open(t, ""), Options{})
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if !plan.Empty() {
		t.Errorf("a second run plans to add %d messages, and should add none", plan.Adding)
	}
	if got := plan.Summary(); !strings.Contains(got, "already on the iPhone") {
		t.Errorf("it does not say plainly that there is nothing to do: %q", got)
	}
	if c := plan.Conversations[0]; c.Skipped == "" {
		t.Error("the conversation does not say why nothing would happen to it")
	}
}

func TestWhatCannotBeCarriedAcross(t *testing.T) {
	t.Parallel()

	from := history(
		said(1, "K1", 0, "something somebody typed"),
		sent(2, "K2", 1, model.KindSystem),
		sent(3, "K3", 2, model.KindCall),
		sent(4, "K4", 3, model.KindIgnored),
		sent(5, "K5", 4, model.KindImage),
	)
	plan, err := Build(context.Background(), from, buildPhone(t).open(t, ""), Options{})
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if plan.Untranslatable != 3 {
		t.Errorf("untranslatable=%d, want the notice, the call and the hidden row", plan.Untranslatable)
	}
	if plan.Adding != 2 {
		t.Errorf("adding=%d, want the words and the picture", plan.Adding)
	}
	if plan.AsPlaceholders != 1 {
		t.Errorf("as_placeholders=%d, want the picture", plan.AsPlaceholders)
	}
	accounted(t, plan, 5)

	// Both limitations have to be said out loud, not left in a number.
	said := strings.Join(plan.Warnings, "\n")
	for _, want := range []string{"line of text", "call history"} {
		if !strings.Contains(said, want) {
			t.Errorf("the warnings do not mention %q:\n%s", want, said)
		}
	}
}

// TestTheSameMessageTwiceInTheSource covers a source that holds a duplicate of its
// own — a backup restored over another — so that this program does not add it twice
// by its own doing.
func TestTheSameMessageTwiceInTheSource(t *testing.T) {
	t.Parallel()

	from := history(said(1, "K1", 0, "one"), said(2, "K1", 1, "one again"), said(3, "K2", 2, "two"))
	plan, err := Build(context.Background(), from, buildPhone(t).open(t, ""), Options{})
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if plan.Adding != 2 {
		t.Errorf("adding=%d, want the duplicate counted once", plan.Adding)
	}
	accounted(t, plan, 3)
}

func TestWhatIsLeftOutAndWhy(t *testing.T) {
	t.Parallel()

	base := func() *android {
		return history(said(1, "K1", 0, "one")).
			with(model.Chat{ID: 2, JID: group, Kind: model.ChatGroup, Name: "Vermut"},
				said(10, "G1", 0, "in the group")).
			with(model.Chat{ID: 3, JID: hiden, Kind: model.ChatDirect, Name: "Somebody"},
				said(20, "H1", 0, "from a hidden identity")).
			with(model.Chat{ID: 4, JID: luis, Kind: model.ChatDirect, Name: "Luis"})
	}

	tests := []struct {
		name  string
		opts  Options
		want  int
		about string
	}{
		{name: "by default, only conversations with a number", want: 1},
		{name: "groups when asked for", opts: Options{Groups: true}, want: 2, about: "group"},
		{name: "hidden identities when asked for", opts: Options{Hidden: true}, want: 2, about: "hidden"},
		{name: "both when asked for", opts: Options{Groups: true, Hidden: true}, want: 3},
		{name: "one conversation only", opts: Options{Only: []string{ana.String()}}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan, err := Build(context.Background(), base(), buildPhone(t).open(t, ""), tt.opts)
			if err != nil {
				t.Fatalf("Build() failed: %v", err)
			}
			if got := plan.Creating + plan.Merging; got != tt.want {
				t.Errorf("%d conversations would be touched, want %d", got, tt.want)
			}
			// The empty conversation is never carried, whatever is asked for.
			for _, c := range plan.Conversations {
				if c.Address == luis.String() && c.Skipped == "" {
					t.Error("an empty conversation was not skipped")
				}
			}
		})
	}
}

// TestTheSamePersonUnderTwoNames is what stops somebody ending up with a
// conversation twice. One phone files them under a number, the other behind a hidden
// identifier, and only WhatsApp's own pairing file says they are the same person.
func TestTheSamePersonUnderTwoNames(t *testing.T) {
	t.Parallel()

	p := buildPhone(t)
	p.holds(t, "99887766554433@lid", 0, "K1")
	pairing := p.pairs(t, "99887766554433", "+34 600 111 222")

	from := history(said(1, "K1", 0, "one"), said(2, "K2", 1, "two"))

	t.Run("with the pairing, it is one conversation", func(t *testing.T) {
		plan, err := Build(context.Background(), from, p.open(t, pairing), Options{})
		if err != nil {
			t.Fatalf("Build() failed: %v", err)
		}
		if plan.Merging != 1 {
			t.Fatalf("merging=%d creating=%d, want it recognised as the same person",
				plan.Merging, plan.Creating)
		}
		if plan.Adding != 1 || plan.AlreadyThere != 1 {
			t.Errorf("adding=%d already=%d, want only the new one", plan.Adding, plan.AlreadyThere)
		}
	})

	t.Run("without it, the same person would arrive twice", func(t *testing.T) {
		plan, err := Build(context.Background(), from, p.open(t, ""), Options{})
		if err != nil {
			t.Fatalf("Build() failed: %v", err)
		}
		if plan.Creating != 1 {
			t.Errorf("creating=%d, want the duplicate this file exists to prevent", plan.Creating)
		}
	})
}

// TestPlanningWritesNothing is the promise, checked rather than asserted.
//
// The store handed in is somebody's phone history. Planning reads it and must leave
// it byte for byte as it was, with no -wal or -shm beside it: SQLite will create
// both on any database it opens in write-ahead mode unless it is told not to, and
// two new files inside somebody's backup is a modified backup.
func TestPlanningWritesNothing(t *testing.T) {
	t.Parallel()

	p := buildPhone(t)
	p.holds(t, ana.String(), 0, "K1")

	before, err := os.ReadFile(p.path)
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}

	from := history(said(1, "K1", 0, "one"), said(2, "K2", 1, "two"))
	if _, err := Build(context.Background(), from, p.open(t, ""), Options{}); err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	after, err := os.ReadFile(p.path)
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("planning changed the store it was only supposed to read")
	}
	for _, side := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(p.path + side); err == nil {
			t.Errorf("planning left a %s beside the store", side)
		}
	}
}

func TestAStoreThisCannotWriteInto(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		schema string
		want   error
	}{
		{
			name:   "not a message store at all",
			schema: `CREATE TABLE holidays (where_ TEXT)`,
			want:   ErrNotAStore,
		},
		{
			// Reading a store is forgiving, because a missing column means a detail
			// that cannot be shown. Writing one is not: a store whose shape has moved
			// is a store where a guess ends up on somebody's phone.
			name: "a message store whose shape has moved",
			schema: `CREATE TABLE Z_PRIMARYKEY (Z_ENT INTEGER, Z_NAME VARCHAR, Z_MAX INTEGER);
				CREATE TABLE ZWACHATSESSION (Z_PK INTEGER, ZCONTACTJID VARCHAR, ZSESSIONTYPE INTEGER,
					ZMESSAGECOUNTER INTEGER, ZLASTMESSAGE INTEGER, ZREMOVED INTEGER);
				CREATE TABLE ZWAMESSAGE (Z_PK INTEGER, ZCHATSESSION INTEGER, ZSTANZAID VARCHAR)`,
			want: ErrUnfamiliarStore,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeStore(t, tt.schema)
			_, err := OpenTarget(context.Background(), path, "")
			if err == nil {
				t.Fatal("it agreed to write into a store it does not understand")
			}
			if !errorIs(err, tt.want) {
				t.Errorf("refused with %v, want %v", err, tt.want)
			}
		})
	}
}
