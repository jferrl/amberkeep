// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/state — nothing is open, which is where somebody with a dead phone
 * finds this program. Where things will be written is already known.
 */
export default {
  "stage": "empty",
  "workspace": "/tmp/amberkeep-test"
} as const;
