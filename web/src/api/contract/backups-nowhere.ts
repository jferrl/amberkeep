// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/backups — on a platform Apple ships no Finder, iTunes or Apple Devices
 * for. `looked` is empty, which is how the page tells "you have not made one yet"
 * from "nothing here can make one" — the same empty list, and different advice.
 */
export default {
  "backups": [],
  "looked": []
} as const;
