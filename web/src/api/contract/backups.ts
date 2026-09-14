// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/backups — what this computer has. The last of them is a backup so old
 * that Apple's index recorded almost nothing about it.
 */
export default {
  "backups": [
    {
      "path": "/Users/ana/Library/Application Support/MobileSync/Backup/00008030-0011",
      "device_name": "Ana's iPhone",
      "product_type": "iPhone14,2",
      "ios_version": "26.0",
      "last_backup": "2026-09-12T20:14:03Z",
      "encrypted": false
    },
    {
      "path": "/Users/ana/Library/Application Support/MobileSync/Backup/00008020-0022",
      "device_name": "Old iPhone",
      "last_backup": "2019-03-02T08:41:00Z",
      "encrypted": true
    },
    {
      "path": "/Volumes/Backups/01234567-89ab",
      "encrypted": false
    }
  ],
  "looked": [
    "/Users/someone/Library/Application Support/MobileSync/Backup"
  ]
} as const;
