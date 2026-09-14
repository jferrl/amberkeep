// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/search — a page of results, each with the matched words marked.
 */
export default {
  "hits": [
    {
      "chat_address": "34600111222@s.whatsapp.net",
      "chat_name": "Ana Lopez",
      "from_me": false,
      "kind": "text",
      "sender": "Ana Lopez",
      "sent_at": "2019-06-14T09:03:00Z",
      "snippet": "\u0002four\u0003"
    }
  ],
  "total": 1
} as const;
