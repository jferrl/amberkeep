// Recorded by internal/api/contract_test.go. Do not edit; re-record it:
//
//   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
//
// This is what the server actually sent. `as const` keeps every string literal,
// so the unions the page declares are checked rather than widened to `string`.

/**
 * GET /api/advice?lang=es — what to say about every failure somebody can act
 * on, in the language the page asked for. A failed state names one of these
 * rather than carrying its words.
 * 
 * One entry of the reply, not all two dozen: the whole of it is the guide's own
 * prose, and recording that here would put a second copy of it in this page's
 * source and make correcting a sentence a re-recording.
 */
export default {
  "crypt15.wrong-key": {
    "title": "Esa clave no abre esta copia de seguridad",
    "body": "Esto lo producen dos cosas, y desde fuera no se pueden distinguir:\n\n  * La clave es de otra copia de seguridad. Comprueba que copiaste la clave\n    que WhatsApp enseñó para este teléfono y esta cuenta.\n  * El archivo está incompleto. Vuelve a copiarlo del teléfono y compara el\n    tamaño con el original antes de intentarlo otra vez."
  }
} as const;
