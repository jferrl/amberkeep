// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/backups — a folder this program was not allowed to look in. An empty
 * list and a refusal are the same emptiness and completely different situations.
 */
export default {
  "backups": [],
  "looked": [
    "/Users/someone/Library/Application Support/MobileSync/Backup"
  ],
  "problem": "the backup could not be read because of a permissions restriction; on macOS, grant access in System Settings > Privacy & Security > Full Disk Access\n\nAdd your terminal, or whichever program is running this, then try again."
} as const;
