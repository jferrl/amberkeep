// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/state — work is running. `note` names the sentence so the page can
 * say it in the reader's own language, `values` is what fills its holes, and
 * `detail` is the same sentence in English, for a name this build has never met.
 */
export default {
  "detail": "Decrypting 236.4 MB. Nothing is being uploaded: this happens on this computer.",
  "note": "decryptingSize",
  "stage": "working",
  "step": "decrypting",
  "thanks": "https://ko-fi.com/jferrl",
  "values": {
    "size": "236.4 MB"
  },
  "workspace": "/tmp/amberkeep-test"
} as const;
