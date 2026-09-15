package phone

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// Reading a phone, against a stand-in for adb.
//
// A real phone cannot be plugged into a test, so adb is replaced by a script that
// records what it was asked and answers with what a real one says. That is the right
// stand-in here, because what is being tested is mostly the argument list: this
// package exists to talk to a device holding somebody's whole life, and the promise
// it makes is that every command it can possibly run is a read.

// pretending writes a fake adb that records its arguments and prints `answer`.
func pretending(t *testing.T, answer string) (adb ADB, asked func() []string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script; the argument checks are platform-independent")
	}

	dir := t.TempDir()
	log := filepath.Join(dir, "asked")
	script := filepath.Join(dir, "adb")

	body := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + log + "\ncat <<'ANSWER'\n" + answer + "\nANSWER\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil { //nolint:gosec // a test's own stand-in
		t.Fatalf("writing the stand-in: %v", err)
	}

	return ADB{Binary: script}, func() []string {
		asked, err := os.ReadFile(log) //nolint:gosec // written by this test
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSpace(string(asked)), "\n")
	}
}

func TestListingPhones(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		said    string
		want    int
		ready   bool
		trouble string
	}{
		{
			name:  "one phone that has been trusted",
			said:  "List of devices attached\nR5CT30ABCDE            device usb:1 product:a model:SM_A566B",
			want:  1,
			ready: true,
		},
		{
			// Plugged in and useless. Saying so is the difference between somebody
			// tapping "allow" on the phone and concluding this does not work.
			name:    "one that has not had the prompt accepted",
			said:    "List of devices attached\nR5CT30ABCDE            unauthorized",
			want:    1,
			trouble: "unauthorized",
		},
		{
			name:    "one that is asleep or has a bad cable",
			said:    "List of devices attached\nR5CT30ABCDE            offline",
			want:    1,
			trouble: "offline",
		},
		{
			name: "none at all",
			said: "List of devices attached",
			want: 0,
		},
		{
			// adb prints its own chatter into the same stream the first time it runs.
			name: "the daemon's own noise is not a phone",
			said: "* daemon not running; starting now at tcp:5037\n* daemon started successfully\nList of devices attached",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The stand-in is written before this goes parallel, and deliberately.
			// Writing an executable and running it are two steps, and on Linux a
			// sibling test that forks in between inherits the descriptor the file is
			// still open for writing on, so the exec fails with "text file busy".
			// Everything written before t.Parallel happens while nothing else in this
			// package is running.
			adb, _ := pretending(t, tt.said)

			t.Parallel()
			phones, err := adb.Phones(context.Background())
			if err != nil {
				t.Fatalf("Phones() failed: %v", err)
			}
			if len(phones) != tt.want {
				t.Fatalf("found %d phones, want %d: %+v", len(phones), tt.want, phones)
			}
			if tt.want == 0 {
				return
			}
			if phones[0].Ready != tt.ready {
				t.Errorf("ready = %v, want %v", phones[0].Ready, tt.ready)
			}
			if phones[0].Trouble != tt.trouble {
				t.Errorf("trouble = %q, want %q", phones[0].Trouble, tt.trouble)
			}
			if phones[0].Name == "" {
				t.Error("it has nothing to call the phone on screen")
			}
		})
	}
}

// TestTheIncrementFragmentsAreNamed covers the trap the guide currently spends a
// paragraph on: a file sitting beside the one somebody wants, looking almost
// identical, and useless on its own.
func TestTheIncrementFragmentsAreNamed(t *testing.T) {
	const listing = "-rw-rw---- 1 root sdcard_rw 247382016 2026-09-12 03:00 msgstore.db.crypt15\n" +
		"-rw-rw---- 1 root sdcard_rw   1048576 2026-09-13 03:00 msgstore-increment-1.db.crypt15"

	// Written before this goes parallel; see the note in TestListingPhones.
	adb, _ := pretending(t, listing)

	t.Parallel()
	found, err := adb.Backups(context.Background(), "R5CT30ABCDE")
	if err != nil {
		t.Fatalf("Backups() failed: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("it found no backups in a listing that has two")
	}

	var whole, fragment int
	for _, b := range found {
		if b.Partial {
			fragment++
			continue
		}
		whole++
		if b.Size == 0 {
			t.Errorf("%s has no size, so nothing can say how long it will take", b.Name)
		}
	}
	if whole == 0 || fragment == 0 {
		t.Errorf("whole = %d, fragments = %d; it cannot tell them apart", whole, fragment)
	}
}

// TestItCannotBeTalkedIntoWriting is the promise this package exists to keep.
func TestItCannotBeTalkedIntoWriting(t *testing.T) {
	// Written before this goes parallel; see the note in TestListingPhones.
	adb, asked := pretending(t, "")

	t.Parallel()

	// A serial is put on a command line, so anything that is not one is refused
	// before it gets there.
	crooked := []string{
		"; rm -rf /",
		"$(rm -rf /)",
		"`reboot`",
		"R5CT30ABCDE hello",
		"--help",
		"",
	}
	for _, serial := range crooked {
		if _, err := adb.Backups(context.Background(), serial); err == nil && serial != "--help" {
			t.Errorf("it accepted %q as a device identifier", serial)
		}
	}

	// And a pull only comes from the two folders WhatsApp keeps backups in.
	elsewhere := []string{
		"/sdcard/DCIM/Camera/photo.jpg",
		"/data/data/com.whatsapp/databases/msgstore.db",
		"/sdcard/Android/media/com.whatsapp/WhatsApp/Databases/../../../../etc/hosts",
		"/sdcard/WhatsApp/Databases/../../secrets.txt",
	}
	for _, remote := range elsewhere {
		if _, err := adb.Fetch(context.Background(), "R5CT30ABCDE", remote, t.TempDir(), nil); err == nil {
			t.Errorf("it agreed to copy %q off the phone", remote)
		}
	}

	// Whatever it did run, none of it writes.
	for _, line := range asked() {
		for _, forbidden := range []string{"push", "install", "uninstall", "rm ", "root", "reboot", "shell rm"} {
			if strings.Contains(line, forbidden) {
				t.Errorf("it ran something that is not a read: %q", line)
			}
		}
	}
}

// TestAPullIsAPull covers the one command that moves bytes, and what it is given.
func TestAPullIsAPull(t *testing.T) {
	// Written before this goes parallel; see the note in TestListingPhones.
	adb, asked := pretending(t, "1 file pulled, 0 skipped.")

	t.Parallel()
	into := t.TempDir()

	local, err := adb.Fetch(context.Background(), "R5CT30ABCDE",
		"/sdcard/Android/media/com.whatsapp/WhatsApp/Databases/msgstore.db.crypt15", into, nil)
	if err != nil {
		t.Fatalf("Fetch() failed: %v", err)
	}
	if filepath.Base(local) != "msgstore.db.crypt15" {
		t.Errorf("it wrote %q; the name should come from the phone", local)
	}
	if filepath.Dir(local) != into {
		t.Errorf("it wrote outside the folder it was given: %q", local)
	}

	ran := strings.Join(asked(), "\n")
	if !strings.Contains(ran, "pull") {
		t.Errorf("it did not pull anything: %q", ran)
	}
	if strings.Contains(ran, "push") {
		t.Errorf("it pushed something: %q", ran)
	}
}

// TestNoADBIsSaidPlainly covers the machine that has never had Android tooling.
func TestNoADBIsSaidPlainly(t *testing.T) {
	t.Parallel()

	var none ADB
	if _, err := none.Phones(context.Background()); err == nil {
		t.Fatal("it claimed to reach a phone with no adb to reach it with")
	}
}

// TestItLooksWhereTheInstructionsSay is the test that would have caught a promise
// this program was making and not keeping.
//
// The screen tells somebody to download a zip, unzip it, and leave the folder in
// Downloads or Applications, and says Amberkeep will notice on its own. It did not:
// it looked only where a package manager or Android Studio puts adb. Telling
// somebody to put a folder somewhere and then not looking there is the kind of
// promise that makes a program feel broken by people who followed it exactly.
func TestItLooksWhereTheInstructionsSay(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("the folders the instructions name differ; the Windows list is checked by reading it")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory to reason about: %v", err)
	}

	// Exactly the places phoneInstallMac and phoneInstallLinux name.
	told := []string{
		filepath.Join(home, "Downloads", "platform-tools", "adb"),
		filepath.Join(home, "Applications", "platform-tools", "adb"),
		"/Applications/platform-tools/adb",
		filepath.Join(home, "platform-tools", "adb"),
	}

	looked := guesses()
	for _, want := range told {
		if !slices.Contains(looked, want) {
			t.Errorf("the instructions say to put it in %s and nothing looks there", want)
		}
	}
}

// TestTheGuessesAreAbsolute covers the one way this list could become a way to run
// something unexpected: a relative entry would resolve against the working
// directory, which is wherever somebody happened to start the program.
func TestTheGuessesAreAbsolute(t *testing.T) {
	t.Parallel()

	for _, guess := range guesses() {
		if !filepath.IsAbs(guess) {
			t.Errorf("%q is relative, so what it finds depends on where this was started", guess)
		}
		if filepath.Base(guess) != "adb" && filepath.Base(guess) != "adb.exe" {
			t.Errorf("%q does not end in adb", guess)
		}
	}
}
