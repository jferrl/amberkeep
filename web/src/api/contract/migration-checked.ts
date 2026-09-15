// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/migration — the checks are done and nothing else has happened. A
 * finding that is not blocking is worth knowing; the one about the safety backup can
 * never pass, because nothing here can see whether it was made.
 */
export default {
  "backup": "/backups/00008030-0011",
  "checks": {
    "findings": [
      {
        "step": "encryption-off",
        "check": "not-encrypted",
        "title": "The backup is not encrypted",
        "passed": true,
        "blocking": true
      },
      {
        "step": "fresh-backup",
        "check": "holds-messages",
        "title": "The backup holds WhatsApp's messages",
        "passed": true,
        "blocking": true
      },
      {
        "step": "fresh-backup",
        "check": "is-recent",
        "note": "hours-ago",
        "values": {
          "hours": "2"
        },
        "title": "The backup is recent",
        "passed": true,
        "blocking": false,
        "detail": "taken 2 hours ago"
      },
      {
        "step": "power-and-space",
        "check": "has-room",
        "note": "needs-space",
        "values": {
          "size": "4.6"
        },
        "title": "There is room for a copy of the backup",
        "passed": true,
        "blocking": false,
        "detail": "it needs about 4.6 GB free"
      },
      {
        "step": "safety-backup",
        "check": "safety-backup",
        "note": "cannot-see-safety",
        "title": "A safety backup exists and has been archived",
        "passed": false,
        "blocking": false,
        "detail": "nothing here can see this; it is the only way back and it has to be done by hand"
      }
    ],
    "needs": 4912345678
  },
  "stage": "checked"
} as const;
