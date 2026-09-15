package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jferrl/amberkeep/internal/canary"
	"github.com/jferrl/amberkeep/internal/fixture"
)

// TestTheCanaryReportsWhatItFound covers the page somebody reads, which is the whole
// interface: a list of names is useless if the report does not say what they are.
func TestTheCanaryReportsWhatItFound(t *testing.T) {
	t.Parallel()

	report := canary.Report{
		Build:   "1.2.3",
		Against: []string{"android WhatsApp 2.26.35.75 on Android 16"},
		Schema: canary.Schema{Platform: "android", Tables: map[string][]string{
			"message": {"_id", "message_type"},
			"newer":   {"_id"},
		}},
		NewTables:  []string{"newer"},
		NewColumns: map[string][]string{"message": {"quoted_row_id"}},
		Counted:    "message.message_type",
		Unknown:    []canary.Count{{Type: 201, Rows: 4}, {Type: -1, Rows: 1}},
		Recognised: 900,
	}

	page := page(report)
	for _, want := range []string{
		"Amberkeep 1.2.3",
		"an Android database",
		"2 tables, 3 columns",
		"Compared against android WhatsApp 2.26.35.75 on Android 16",
		"Tables this build has never seen",
		"newer",
		"Columns this build has never seen",
		"quoted_row_id",
		"message.message_type",
		"201",
		"4 messages",
		// The row whose type is NULL, which is a real thing a real database has and
		// is not code zero.
		"none",
		"1 message",
		"900 recognised",
		"can be pasted into an",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the report does not mention %q:\n%s", want, page)
		}
	}
}

// TestACleanReportSaysSo covers the other half: somebody who runs this on a database
// nothing has changed in should be told that in a sentence, not left to infer it from
// the absence of lists.
func TestACleanReportSaysSo(t *testing.T) {
	t.Parallel()

	page := page(canary.Report{
		Build:      "1.2.3",
		Against:    []string{"iphone WhatsApp 2.26.33.73 on iOS 26"},
		Schema:     canary.Schema{Platform: "iphone", Tables: map[string][]string{"ZWAMESSAGE": {"Z_PK"}}},
		Counted:    "ZWAMESSAGE.ZMESSAGETYPE",
		Recognised: 12,
	})

	if !strings.Contains(page, "Nothing here is new to this build.") {
		t.Errorf("a clean report does not say so:\n%s", page)
	}
	if !strings.Contains(page, "carries a type this build has a meaning for") {
		t.Errorf("a clean report does not say the types were all recognised:\n%s", page)
	}
}

// TestAReportWithNothingToCompareAgainstSaysThatToo covers the case that would
// otherwise read as alarming: a platform with no shapes reports everything as new.
func TestAReportWithNothingToCompareAgainstSaysThatToo(t *testing.T) {
	t.Parallel()

	page := page(canary.Report{
		Build:  "1.2.3",
		Schema: canary.Schema{Platform: "android", Tables: map[string][]string{"message": {"_id"}}},
	})

	if !strings.Contains(page, "no shape to compare") {
		t.Errorf("it does not say there was nothing to compare against:\n%s", page)
	}
}

// TestEmittingAShapeNeedsTheVersion covers the one thing a database cannot say about
// itself and the one thing an entry is useless without.
func TestEmittingAShapeNeedsTheVersion(t *testing.T) {
	t.Parallel()

	db := filepath.Join(t.TempDir(), "msgstore.db")
	fixture.TinyArchive(t, db)

	err := run(context.Background(), []string{"canary", "--db", db, "--emit"})
	if err == nil {
		t.Fatal("a shape was emitted with no version on it")
	}
	if !strings.Contains(err.Error(), "--whatsapp") {
		t.Errorf("the refusal does not say what is missing: %v", err)
	}
}

// TestEmittingAShapeWritesSomethingSendable is the contribution path, end to end.
func TestEmittingAShapeWritesSomethingSendable(t *testing.T) {
	db := filepath.Join(t.TempDir(), "msgstore.db")
	fixture.TinyArchive(t, db)

	written := capture(t, func() error {
		return run(context.Background(), []string{
			"canary", "--db", db, "--emit", "--whatsapp", "2.26.36.1", "--os", "Android 16",
		})
	})

	var entry canary.Entry
	if err := json.Unmarshal([]byte(written), &entry); err != nil {
		t.Fatalf("what it wrote is not an entry: %v\n%s", err, written)
	}
	if entry.Platform != "android" || entry.WhatsApp != "2.26.36.1" || entry.OS != "Android 16" {
		t.Errorf("entry = %+v", entry)
	}
	if entry.Seen == "" {
		t.Error("an entry has to say when it was seen")
	}
	if len(entry.Tables["message"]) == 0 {
		t.Errorf("the shape has no message table: %v", entry.Tables)
	}
}

// capture runs something that writes to standard output and returns what it wrote.
//
// Not parallel-safe, which is why the test above does not ask to be: it replaces the
// process's own standard output for the duration.
func capture(t *testing.T, run func() error) string {
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
