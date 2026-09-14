// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/state — work is running. `detail` is the server's own words and is
 * shown as they are; it changes while the work runs, and is the only thing that does.
 */
export default {
  "detail": "Decrypting 236.4 MB. Nothing is being uploaded.",
  "stage": "working",
  "step": "decrypting",
  "workspace": "/tmp/amberkeep-test"
} as const;
