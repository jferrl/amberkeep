// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/state — there is an archive. It describes itself here so the page
 * does not have to ask a second time.
 */
export default {
  "archive": {
    "by_kind": {
      "direct": 1,
      "group": 1
    },
    "conversations": 2,
    "latest": "2019-06-14T10:00:00Z",
    "layout": "modern",
    "messages": 6,
    "named": 2,
    "people": 2,
    "searchable": false,
    "time_zone": "UTC"
  },
  "stage": "ready",
  "thanks": "https://ko-fi.com/jferrl",
  "workspace": "/tmp/amberkeep-test"
} as const;
