// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/chats — a page of the conversation list.
 */
export default {
  "chats": [
    {
      "id": 4,
      "address": "120363999@g.us",
      "kind": "group",
      "name": "Everything at once",
      "participants": [
        {
          "address": "34600111222@s.whatsapp.net",
          "name": "Ana Lopez",
          "admin": true
        }
      ],
      "description": "A conversation that exists to be written down",
      "created_at": "2019-06-13T09:00:00Z",
      "last_message_at": "2019-06-14T11:00:00Z",
      "message_count": 1
    },
    {
      "id": 2,
      "address": "120363001@g.us",
      "kind": "group",
      "name": "Vermut",
      "participants": [
        {
          "address": "34600111222@s.whatsapp.net",
          "name": "Ana Lopez"
        },
        {
          "address": "34600333444@s.whatsapp.net",
          "name": "Luis"
        }
      ],
      "last_message_at": "2019-06-14T10:00:00Z",
      "message_count": 1
    }
  ],
  "total": 3
} as const;
