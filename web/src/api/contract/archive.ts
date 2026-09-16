// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/archive — what the archive holds.
 */
export default {
  "by_kind": {
    "direct": 1,
    "group": 2
  },
  "conversations": 3,
  "earliest": "2019-06-13T09:00:00Z",
  "files": false,
  "latest": "2019-06-14T11:00:00Z",
  "layout": "modern",
  "messages": 7,
  "named": 2,
  "people": 2,
  "searchable": true,
  "time_zone": "UTC",
  "title": "Archive"
} as const;
