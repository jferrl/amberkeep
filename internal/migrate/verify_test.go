package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestACorrectMigrationPasses is the baseline. Without it the table below would be
// satisfied by a checker that failed everything.
func TestACorrectMigrationPasses(t *testing.T) {
	t.Parallel()

	original, result, plan := migrated(t)
	report, err := Verify(context.Background(), original, result, plan)
	if err != nil {
		t.Fatalf("Verify() failed: %v", err)
	}

	if !report.OK() {
		for _, c := range report.Failures() {
			t.Errorf("a correct migration failed %q: %s", c.Name, c.Detail)
		}
	}
	if len(report.Checks) < 15 {
		t.Errorf("only %d checks ran, which is not enough to be worth trusting", len(report.Checks))
	}
	if got := report.Added["ZWAMESSAGE"]; got != 2 {
		t.Errorf("it counted %d messages added, want 2", got)
	}
	if !strings.Contains(report.Summary(), "passed") {
		t.Errorf("summary = %q", report.Summary())
	}
}

// TestEveryCheckCatchesWhatItIsFor is the point of writing this before the writer.
//
// Each case damages a correct migration in exactly one way and insists the check
// meant for it fails. A check nobody has watched fail is a check that has never been
// shown to do anything, and this one stands between a person and their own phone.
func TestEveryCheckCatchesWhatItIsFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// harm is run against the migrated store.
		harm string
		// file, when set, damages the file rather than its contents.
		file func(t *testing.T, path string)
		// catches is the check that must notice, matched on part of its name.
		catches string
	}{
		{
			name:    "a message already on the phone is deleted",
			harm:    "DELETE FROM ZWAMESSAGE WHERE ZSTANZAID = 'OLD1'",
			catches: "nothing was removed from ZWAMESSAGE",
		},
		{
			name:    "a message already on the phone is reworded",
			harm:    "UPDATE ZWAMESSAGE SET ZTEXT = 'something else' WHERE ZSTANZAID = 'OLD1'",
			catches: "rows already on the phone are unchanged in ZWAMESSAGE",
		},
		{
			name:    "a conversation nobody touched is reordered",
			harm:    "UPDATE ZWAMESSAGE SET ZSORT = 99 WHERE ZSTANZAID = 'OLD1'",
			catches: "conversations this did not touch are ordered",
		},
		{
			name:    "a conversation already on the phone is renamed",
			harm:    "UPDATE ZWACHATSESSION SET ZCONTACTJID = 'someone.else@s.whatsapp.net' WHERE ZCONTACTJID = '" + luisAddress + "'",
			catches: "rows already on the phone are unchanged in ZWACHATSESSION",
		},
		{
			// The one that silently destroys a message weeks later: the phone hands
			// the same identifier out again and the new row replaces the old.
			name:    "Core Data is left able to reuse an identifier",
			harm:    "UPDATE Z_PRIMARYKEY SET Z_MAX = 0 WHERE Z_NAME = 'WAMessage'",
			catches: "will not hand out an identifier that is already in use",
		},
		{
			name:    "an added conversation is numbered with a gap",
			harm:    "UPDATE ZWAMESSAGE SET ZSORT = 7 WHERE ZSTANZAID = 'NEW2'",
			catches: "numbered 1 to N in date order",
		},
		{
			name:    "a conversation points at the wrong last message",
			harm:    "UPDATE ZWACHATSESSION SET ZLASTMESSAGE = 1 WHERE ZCONTACTJID = '" + anaAddress + "'",
			catches: "points at its own last message",
		},
		{
			name:    "the counter is left where the phone would overwrite a message",
			harm:    "UPDATE ZWACHATSESSION SET ZMESSAGECOUNTER = 1 WHERE ZCONTACTJID = '" + anaAddress + "'",
			catches: "will not write a new message on top of an old one",
		},
		{
			name: "a message is added with no conversation to belong to",
			harm: "INSERT INTO ZWAMESSAGE (Z_PK, Z_ENT, ZCHATSESSION, ZSORT, ZISFROMME, ZMESSAGEDATE, ZSTANZAID, ZTEXT, ZFROMJID) " +
				"VALUES (900, 2, 888, 1, 0, 9.0, 'LOOSE', 'nowhere', '" + anaAddress + "')",
			catches: "without a conversation to belong to",
		},
		{
			name:    "an added message has nothing to show",
			harm:    "UPDATE ZWAMESSAGE SET ZTEXT = '' WHERE ZSTANZAID = 'NEW1'",
			catches: "has something to show",
		},
		{
			name:    "the same message is added twice",
			harm:    "UPDATE ZWAMESSAGE SET ZSTANZAID = 'NEW1' WHERE ZSTANZAID = 'NEW2'",
			catches: "no message appears twice",
		},
		{
			name:    "somebody ends up in the list twice",
			harm:    "UPDATE ZWACHATSESSION SET ZCONTACTJID = '" + luisAddress + "' WHERE ZCONTACTJID = '" + anaAddress + "'",
			catches: "nobody appears in the conversation list twice",
		},
		{
			name:    "an added message says it is both from and to somebody",
			harm:    "UPDATE ZWAMESSAGE SET ZTOJID = '" + anaAddress + "' WHERE ZSTANZAID = 'NEW1'",
			catches: "says who each added message was from or to",
		},
		{
			// A store with a log beside it has not been finished. The phone replays
			// whatever is in that log over a database it did not write.
			name: "a write-ahead log is left beside the result",
			file: func(t *testing.T, path string) {
				t.Helper()
				if err := os.WriteFile(path+"-wal", []byte("unfinished"), 0o600); err != nil {
					t.Fatalf("damaging the fixture: %v", err)
				}
			},
			catches: "no write-ahead log left beside",
		},
		{
			name: "the result is no longer the kind of database the phone expects",
			file: func(t *testing.T, path string) {
				t.Helper()
				f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
				if err != nil {
					t.Fatalf("damaging the fixture: %v", err)
				}
				defer func() { _ = f.Close() }()
				if _, err := f.WriteAt([]byte{1, 1}, 18); err != nil {
					t.Fatalf("damaging the fixture: %v", err)
				}
			},
			catches: "write-ahead-log database, as the phone expects",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original, result, plan := migrated(t)
			if tt.harm != "" {
				damage(t, result, tt.harm)
			}
			if tt.file != nil {
				tt.file(t, result)
			}

			report, err := Verify(context.Background(), original, result, plan)
			if err != nil {
				t.Fatalf("Verify() failed: %v", err)
			}
			if report.OK() {
				t.Fatalf("a store damaged this way passed every check, and should not have")
			}

			var noticed bool
			for _, c := range report.Failures() {
				if strings.Contains(c.Name, tt.catches) {
					noticed = true
				}
			}
			if !noticed {
				t.Errorf("nothing named %q failed; what did fail was %v",
					tt.catches, namesOf(report.Failures()))
			}
		})
	}
}

// TestAnUncheckedReportIsNotAPass covers the state that would otherwise look like
// success: a report with nothing in it.
func TestAnUncheckedReportIsNotAPass(t *testing.T) {
	t.Parallel()

	var empty Report
	if empty.OK() {
		t.Error("a report that checked nothing called itself fine")
	}
	if !strings.Contains(empty.Summary(), "not the same as nothing being wrong") {
		t.Errorf("summary = %q", empty.Summary())
	}
}

func namesOf(checks []Check) []string {
	out := make([]string, 0, len(checks))
	for _, c := range checks {
		out = append(out, c.Name)
	}
	return out
}

// TestCheckingLeavesNothingBehind is a regression, and the bug it guards was found
// by running the checks against the real migration twice.
//
// SQLite creates a -shm beside any write-ahead-log database it opens, read-only or
// not. The file being checked is the one about to be restored onto a phone, so two
// new files beside it is modifying the very thing under inspection — and it made the
// second run fail the first check, on a store the first run had just called sound.
func TestCheckingLeavesNothingBehind(t *testing.T) {
	t.Parallel()

	original, result, plan := migrated(t)

	for attempt := 1; attempt <= 2; attempt++ {
		report, err := Verify(context.Background(), original, result, plan)
		if err != nil {
			t.Fatalf("checking, attempt %d: %v", attempt, err)
		}
		if !report.OK() {
			t.Fatalf("attempt %d failed %v", attempt, namesOf(report.Failures()))
		}
		for _, side := range []string{"-wal", "-shm"} {
			for _, path := range []string{result, original} {
				if _, err := os.Stat(path + side); err == nil {
					t.Fatalf("attempt %d left a %s beside %s", attempt, side, filepath.Base(path))
				}
			}
		}
	}
}
