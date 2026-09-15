import type { Said } from "@/api/types";
import type { Language, Phrase, Translate } from "@/i18n";
import { count as formatted } from "@/lib/format";

/**
 * The running commentary, in the reader's own language and numbers.
 *
 * The server names each sentence and sends the numbers behind it rather than
 * writing it out, because it knows neither who is reading nor how they write a
 * thousand. This is where the two meet: the name chooses the sentence, the values
 * fill its holes, and the counts are formatted here, where the language is known.
 *
 * Every table below is deliberately partial. A server newer than this page can send
 * a name this build has never heard of, and the honest answer to that is the
 * server's own English sentence — which it always sends — rather than a blank line
 * where a number had been counting up.
 */
const sentences: Record<string, Phrase> = {
  starting: "noteStarting",
  openingArchive: "noteOpeningArchive",
  takingMessagesOut: "noteTakingMessagesOut",
  decryptingBackup: "noteDecryptingBackup",
  readingArchive: "noteReadingArchive",
  copyingOffPhone: "noteCopyingOffPhone",
  copyingOffPhoneLong: "noteCopyingOffPhoneLong",
  lookingAtBackup: "noteLookingAtBackup",
  workingOutWhatMoves: "noteWorkingOutWhatMoves",
  movingMessages: "noteMovingMessages",
  readingBackupOf: "noteReadingBackupOf",
  takingPictures: "noteTakingPictures",
  tookPictures: "noteTookPictures",
  decryptingSize: "noteDecryptingSize",
  addingIndexes: "noteAddingIndexes",
  indexedSoFar: "workingIndexedSoFar",
  indexed: "noteIndexed",
  archiveChanged: "noteArchiveChanged",
  settingsChanged: "noteSettingsChanged",
  buildingIndex: "noteBuildingIndex",
  writtenSoFar: "noteWrittenSoFar",
  writingIndexPage: "noteWritingIndexPage",
  readingBothSides: "noteReadingBothSides",
  takingPhoneMessages: "noteTakingPhoneMessages",
  movingCount: "noteMovingCount",
  checkingWritten: "noteCheckingWritten",
  intoACopy: "noteIntoACopy",
};

/**
 * The sentences that read wrongly at one.
 *
 * Spanish and English both need a different sentence for a single thing, and the
 * plural form with a 1 in front of it — "Moviendo 1 mensajes" — is the tell of a
 * program that was translated rather than written. Only the notes whose count can
 * really be one are here: the index build reports every few thousand, and an export
 * reports every twenty-five.
 */
const alone: Record<string, { when: string; instead: Phrase }> = {
  tookPictures: { when: "pictures", instead: "noteTookPicturesOne" },
  movingCount: { when: "messages", instead: "noteMovingCountOne" },
};

/**
 * said is what to show for one of these, or the server's own words when this build
 * does not know the name.
 */
export function said(
  state: Said,
  language: Language,
  t: Translate,
): string | undefined {
  if (state.note === undefined) return state.detail;

  const one = alone[state.note];
  const single =
    one !== undefined &&
    state.counts?.find((c) => c.of === one.when)?.n === 1;

  const phrase = single ? one.instead : sentences[state.note];
  if (phrase === undefined) return state.detail;

  // The numbers are written after the values and therefore win: what the server
  // wrote into the English sentence is "1,234 messages", and this page wants the
  // same number written the way its reader writes it.
  const values: Record<string, string> = { ...state.values };
  for (const c of state.counts ?? []) {
    values[c.of] = formatted(c.n, language);
  }
  return t(phrase, values);
}

/**
 * carried copies the sentence out of one state and into another.
 *
 * The migration and the export both hand their own progress to the wizard's working
 * screen, which takes the wizard's own shape. Copying field by field at each of them
 * is how one of the four got left behind the last time this grew: for a while the
 * numbers travelled and the name did not.
 *
 * Absent fields stay absent rather than arriving as undefined, which the screen
 * would render as a blank line.
 */
export function carried(from: Said): Said {
  return {
    ...(from.note === undefined ? {} : { note: from.note }),
    ...(from.detail === undefined ? {} : { detail: from.detail }),
    ...(from.values === undefined ? {} : { values: from.values }),
    ...(from.counts === undefined ? {} : { counts: from.counts }),
  };
}
