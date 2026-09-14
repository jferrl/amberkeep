// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/migration — what would move, which is the thing somebody agrees to.
 * It stops here until they do.
 */
export default {
  "backup": "/backups/00008030-0011",
  "plan": {
    "conversations": [
      {
        "address": "34600111222@s.whatsapp.net",
        "name": "Ana Lopez",
        "kind": "direct",
        "destination": "34600111222@s.whatsapp.net",
        "into": "34600111222@s.whatsapp.net",
        "session": 1617,
        "adding": 34,
        "already_there": 15,
        "untranslatable": 6,
        "as_placeholders": 9,
        "on_phone_already": 15,
        "earliest": "2019-06-14T09:00:00Z",
        "latest": "2019-06-17T09:00:00Z"
      },
      {
        "address": "120363001@g.us",
        "name": "Vermut del sabado",
        "kind": "group",
        "destination": "120363001@g.us",
        "adding": 0,
        "already_there": 0,
        "untranslatable": 0,
        "as_placeholders": 0,
        "on_phone_already": 0,
        "skipped": "groups were not included"
      }
    ],
    "adding": 34,
    "already_there": 15,
    "untranslatable": 6,
    "as_placeholders": 9,
    "merging": 1,
    "creating": 0,
    "untouched": 553,
    "warnings": [
      "9 messages will arrive as a line of text saying what was sent, not as the picture, recording or file itself."
    ],
    "earliest": "2019-06-14T09:00:00Z",
    "latest": "2019-06-17T09:00:00Z"
  },
  "stage": "planned"
} as const;
