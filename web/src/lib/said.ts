import type { Said, Spoken } from "@/api/types";
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

/**
 * filled puts values into a sentence's holes.
 *
 * For the sentences that arrive from the server already in the reader's language —
 * the guide's steps, a preflight check — where the catalogue doing the substituting
 * is the program's rather than this page's. The braces are the same on both sides.
 *
 * A hole nothing fills is left as it is rather than emptied: a visible {size} is a
 * bug somebody can see and report, and a blank where a number should be is a bug that
 * reads as a finished sentence.
 */
export function filled(
  template: string,
  values: Readonly<Record<string, string>> | undefined,
): string {
  if (values === undefined) return template;
  return template.replace(/\{(\w+)\}/g, (whole, name: string) =>
    name in values ? (values[name] ?? whole) : whole,
  );
}

/** Sentences are the program's own words, as it serves them: by name, in one language. */
export type Sentences = Readonly<Record<string, string>> | undefined;

/**
 * spoken is what a named sentence says, in the reader's own language.
 *
 * The program names what it wants said and sends the English beside it. When this
 * build's guide knows the name, the reader gets their own language; when it does
 * not — a program newer than the page it is serving — they get the English, which is
 * better than a blank line where a sentence was.
 */
export function spoken(
  said: Spoken | undefined,
  sentences: Sentences,
): string | undefined {
  if (said === undefined) return undefined;

  const template = said.note === undefined ? undefined : sentences?.[said.note];
  return template === undefined ? said.text : filled(template, said.values);
}
