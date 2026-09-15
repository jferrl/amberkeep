package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jferrl/amberkeep/internal/licence"
)

// TestASellerCanMakeAKeyAndIssueALicence is the seller's whole path, which has to
// work on the day somebody has paid and is waiting for their key.
func TestASellerCanMakeAKeyAndIssueALicence(t *testing.T) {
	signing := filepath.Join(t.TempDir(), "signing")

	if err := run([]string{"keys", "--into", signing}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(signing)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the signing key is %v, and should be readable by nobody else", mode)
	}

	key := say(t, func() error {
		return run([]string{"issue", "--key", signing, "--grants", "archive,migration", "--order", "pdl_1"})
	})

	held, err := licence.Read(strings.TrimSpace(key))
	if err != nil {
		t.Fatalf("what it issued does not read back: %v", err)
	}
	if !held.Covers(licence.Archive) || !held.Covers(licence.Migration) {
		t.Errorf("it covers %v", held.Grants)
	}
	if held.Order != "pdl_1" {
		t.Errorf("order = %q", held.Order)
	}
	if held.Issued == "" || held.Updates == "" {
		t.Errorf("a licence with no dates on it: %+v", held)
	}
}

func TestTheSellerIsToldWhenTheyAskForSomethingNobodySells(t *testing.T) {
	t.Parallel()

	signing := filepath.Join(t.TempDir(), "signing")
	if err := run([]string{"keys", "--into", signing}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
	}{
		{name: "nothing to do", args: nil},
		{name: "something that is not a thing to do", args: []string{"revoke"}},
		{name: "keys with nowhere to put them", args: []string{"keys"}},
		{name: "issue with no signing key", args: []string{"issue"}},
		{name: "issue with a file that is not one", args: []string{"issue", "--key", "/dev/null"}},
		{name: "a grant nobody sells", args: []string{"issue", "--key", signing, "--grants", "everything"}},
		{name: "a licence covering nothing", args: []string{"issue", "--key", signing, "--grants", ","}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := run(tt.args); err == nil {
				t.Errorf("run(%v) was accepted", tt.args)
			}
		})
	}
}

// say runs something that writes to standard output and returns what it wrote.
func say(t *testing.T, run func() error) string {
	t.Helper()

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	was := os.Stdout
	os.Stdout = write
	defer func() { os.Stdout = was }()

	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := read.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- b.String()
	}()

	if err := run(); err != nil {
		_ = write.Close()
		<-done
		t.Fatal(err)
	}
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	return <-done
}
