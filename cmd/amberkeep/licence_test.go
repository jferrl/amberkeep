package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jferrl/amberkeep/internal/licence"
)

// TestTheLicenceCommandSaysWhatIsHere covers the screen somebody reads when they are
// wondering whether the thing they paid for arrived.
func TestTheLicenceCommandSaysWhatIsHere(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	// A computer with nothing on it.
	said := capture(t, func() error {
		return run(context.Background(), []string{"licence"})
	})
	if !strings.Contains(said, "no licence on this computer") {
		t.Errorf("it does not say there is none:\n%s", said)
	}
	if !strings.Contains(said, "Nothing is for sale yet") {
		t.Errorf("it does not say why nothing is asking for one:\n%s", said)
	}

	// One that covers both things, which is what the licence machinery is for.
	key := issued(t, licence.Archive, licence.Migration)
	said = capture(t, func() error {
		return run(context.Background(), []string{"licence", "--add", key})
	})
	if !strings.Contains(said, "writing the archive out and moving a history onto an iPhone") {
		t.Errorf("it does not say what was bought:\n%s", said)
	}

	said = capture(t, func() error {
		return run(context.Background(), []string{"licence"})
	})
	if !strings.Contains(said, "bought on") || !strings.Contains(said, "updates until") {
		t.Errorf("it does not say when:\n%s", said)
	}

	// And putting it down again, for somebody moving to another computer.
	said = capture(t, func() error {
		return run(context.Background(), []string{"licence", "--remove"})
	})
	if !strings.Contains(said, "forgotten") {
		t.Errorf("it does not say it let go:\n%s", said)
	}
}

// TestALicenceCoveringOneThingSaysWhichOne covers the sentence somebody reads when
// they bought the archive and reached the migration.
func TestALicenceCoveringOneThingSaysWhichOne(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	said := capture(t, func() error {
		return run(context.Background(), []string{"licence", "--add", issued(t, licence.Migration)})
	})
	if !strings.Contains(said, "moving a history onto an iPhone") {
		t.Errorf("it does not say what it covers:\n%s", said)
	}
	if strings.Contains(said, "writing the archive out") {
		t.Errorf("it says it covers something nobody bought:\n%s", said)
	}
}

// issued mints a licence the way the seller would. The signing key is made here and
// thrown away with the test.
func issued(t *testing.T, grants ...licence.Grant) string {
	t.Helper()

	signer := signingKey(t)
	key, err := licence.Write(licence.Licence{
		Grants: grants, Issued: "2026-09-15", Updates: "2027-09-15", Order: "pdl_test",
	}, signer)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// signingKey is the seller's half, made fresh. A build with no public key in it
// verifies nothing, which is the state of this one, so what this proves is that the
// command reads a real licence rather than that it refuses a false one — that is the
// licence package's own test.
func signingKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()

	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return private
}
