// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/chats/{address}/messages — a page of one conversation, with a
 * page before it, so the field that says where that one starts is here to be checked.
 */
export default {
  "before": "1560502980000_4",
  "chat": {
    "id": 1,
    "address": "34600111222@s.whatsapp.net",
    "kind": "direct",
    "name": "Ana Lopez",
    "last_message_at": "2019-06-14T09:04:00Z",
    "message_count": 5
  },
  "messages": [
    {
      "id": 4,
      "sent_at": "2019-06-14T09:03:00Z",
      "from_me": false,
      "sender": "34600111222@s.whatsapp.net",
      "sender_name": "Ana Lopez",
      "kind": "text",
      "text": "four",
      "rendered": "four",
      "source_type": 0
    },
    {
      "id": 5,
      "sent_at": "2019-06-14T09:04:00Z",
      "from_me": false,
      "sender": "34600111222@s.whatsapp.net",
      "sender_name": "Ana Lopez",
      "kind": "text",
      "text": "five",
      "rendered": "five",
      "source_type": 0
    }
  ]
} as const;
