// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/migration — nothing started. Every stage of a migration ends and
 * waits to be asked for the next; none of them leads to the next on its own.
 */
export default {
  "stage": "idle"
} as const;
