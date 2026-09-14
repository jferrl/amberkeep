// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/state — it did not work. Failure is not the end of the road: the
 * next request works from the same screen.
 */
export default {
  "detail": "this file is a database, but not one this version can read",
  "guidance": "what to do about: this file is a database, but not one this version can read",
  "stage": "failed",
  "workspace": "/tmp/amberkeep-test"
} as const;
