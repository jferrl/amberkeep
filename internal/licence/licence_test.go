package licence

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// signing returns a key pair and points this build's verifier at it, the way a
// released build will point at the seller's.
func signing(t *testing.T) ed25519.PrivateKey {
	t.Helper()

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	was := verifier
	verifier = public
	t.Cleanup(func() { verifier = was })
	return private
}

func TestAKeyCarriesWhatWasBought(t *testing.T) {
	signer := signing(t)

	want := Licence{
		Grants:  []Grant{Archive, Migration},
		Issued:  "2026-09-15",
		Updates: "2027-09-15",
		Order:   "pdl_0001",
	}
	key, err := Write(want, signer)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, prefix) {
		t.Errorf("a key that does not say what it is: %q", key)
	}

	got, err := Read(key)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Covers(Archive) || !got.Covers(Migration) {
		t.Errorf("it covers %v", got.Grants)
	}
	if got.Issued != want.Issued || got.Updates != want.Updates || got.Order != want.Order {
		t.Errorf("read %+v, want %+v", got, want)
	}
	if got.Covers("everything") {
		t.Error("it covers something nobody sold")
	}
}

// TestAKeySomebodyElseSignedIsRefused is the only thing a signature is for.
func TestAKeySomebodyElseSignedIsRefused(t *testing.T) {
	signing(t) // this build's key

	_, theirs, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := Write(Licence{Grants: []Grant{Archive, Migration}}, theirs)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Read(key); !errors.Is(err, ErrForged) {
		t.Errorf("a licence signed by somebody else was read as %v", err)
	}
}

// TestAKeyThatLostACharacterSaysSo covers the ordinary case, which is not forgery:
// somebody pasted a key out of an email and left half of it behind.
func TestAKeyThatLostACharacterSaysSo(t *testing.T) {
	signer := signing(t)

	key, err := Write(Licence{Grants: []Grant{Archive}, Issued: "2026-09-15"}, signer)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		key  string
	}{
		{name: "nothing at all", key: ""},
		{name: "somebody's password by mistake", key: "hunter2"},
		{name: "the right shape, the wrong program", key: "OTHERTOOL-1.aaa.bbb"},
		{name: "cut short", key: key[:len(key)-20]},
		{name: "one half only", key: strings.Split(key, ".")[0]},
		{name: "the signature turned into words", key: prefix + "not-base64!.also-not"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Read(tt.key); !errors.Is(err, ErrMalformed) {
				t.Errorf("Read(%q) = %v, want it to say the key is malformed", tt.key, err)
			}
		})
	}
}

// TestTheBuildSomebodyPaidForKeepsWorking covers the promise in the pricing: twelve
// months of updates, and the version you were entitled to never stops.
func TestTheBuildSomebodyPaidForKeepsWorking(t *testing.T) {
	t.Parallel()

	licence := Licence{Grants: []Grant{Archive}, Issued: "2026-09-15", Updates: "2027-09-15"}

	tests := []struct {
		name  string
		built time.Time
		want  bool
	}{
		{name: "the build they bought", built: day(t, "2026-09-15"), want: true},
		{name: "one eleven months later", built: day(t, "2027-08-15"), want: true},
		{name: "the last day of it", built: day(t, "2027-09-15"), want: true},
		{name: "the day after", built: day(t, "2027-09-16"), want: false},
		{name: "years later", built: day(t, "2030-01-01"), want: false},
		{name: "a build that does not say when it was made", built: time.Time{}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := licence.Current(tt.built); got != tt.want {
				t.Errorf("Current(%s) = %v, want %v", tt.built.Format(time.DateOnly), got, tt.want)
			}
		})
	}
}

func day(t *testing.T, on string) time.Time {
	t.Helper()
	when, err := time.Parse(time.DateOnly, on)
	if err != nil {
		t.Fatal(err)
	}
	return when
}

// TestNothingIsSoldYet is here so that switching the shop on is a deliberate act with
// a failing test in front of it, rather than something that happens by accident in a
// refactor. Delete it in the commit that opens the shop.
func TestNothingIsSoldYet(t *testing.T) {
	t.Parallel()

	if Sells() {
		t.Error("the shop is open; every gate in the program now asks for a licence")
	}
}

func TestAKeyIsKeptWhereTheOperatingSystemKeepsSettings(t *testing.T) {
	signer := signing(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	if _, err := Held(); !errors.Is(err, ErrNoLicence) {
		t.Errorf("a computer with no licence said %v", err)
	}

	key, err := Write(Licence{Grants: []Grant{Archive}, Issued: "2026-09-15"}, signer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Keep(key); err != nil {
		t.Fatal(err)
	}

	held, err := Held()
	if err != nil {
		t.Fatal(err)
	}
	if !held.Covers(Archive) {
		t.Errorf("what was kept covers %v", held.Grants)
	}
	if !strings.Contains(Where(), "amberkeep") {
		t.Errorf("it is kept at %q", Where())
	}

	// Somebody moving to another computer.
	if err := Forget(); err != nil {
		t.Fatal(err)
	}
	if _, err := Held(); !errors.Is(err, ErrNoLicence) {
		t.Errorf("after forgetting it, Held said %v", err)
	}
	// And again, because tidying something already tidy is not a failure.
	if err := Forget(); err != nil {
		t.Errorf("forgetting nothing failed: %v", err)
	}
}

// TestAKeyThatIsNotOneIsNeverKept covers the thing that would otherwise leave
// somebody with a file full of rubbish and a program that says nothing about it.
func TestAKeyThatIsNotOneIsNeverKept(t *testing.T) {
	signing(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	if _, err := Keep("hunter2"); !errors.Is(err, ErrMalformed) {
		t.Errorf("Keep of a non-key = %v", err)
	}
	if _, err := Held(); !errors.Is(err, ErrNoLicence) {
		t.Errorf("something was written anyway: %v", err)
	}
}

// TestNothingIsGatedWhileNothingIsSold is the safety line under the whole thing: a
// build with no shop must not refuse a single thing, or somebody is stuck with a
// program that wants money it cannot take.
func TestNothingIsGatedWhileNothingIsSold(t *testing.T) {
	t.Parallel()

	for _, grant := range []Grant{Archive, Migration, "something nobody sells"} {
		if err := Allows(grant); err != nil {
			t.Errorf("Allows(%s) = %v, and there is nowhere to buy one", grant, err)
		}
	}
}

// TestAMissingLicenceSaysWhichOne covers what somebody reads when the shop is open:
// having bought one of the two and reached the other is the common case, and "a
// licence is needed" would not tell them which.
func TestAMissingLicenceSaysWhichOne(t *testing.T) {
	t.Parallel()

	tests := []struct {
		grant Grant
		says  string
	}{
		{grant: Archive, says: "writing the archive out"},
		{grant: Migration, says: "moving a history onto an iPhone"},
	}

	for _, tt := range tests {
		t.Run(string(tt.grant), func(t *testing.T) {
			t.Parallel()

			err := error(&Error{Grant: tt.grant, Guidance: GuidanceNeeded})
			if !strings.Contains(err.Error(), tt.says) {
				t.Errorf("it says %q, want something about %q", err, tt.says)
			}
			// And anything that cares only whether it was about money can ask that.
			if !errors.Is(err, ErrNeeded) {
				t.Error("a licence failure is not recognisable as one")
			}
		})
	}
}
