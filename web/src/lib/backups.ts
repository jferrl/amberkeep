import type { Backup } from "@/api/types";
import type { Language, Translate } from "@/i18n";
import { shortDate, timeOfDay } from "@/lib/format";

/**
 * Reading the list of backups this computer has made.
 *
 * Here rather than beside one screen because two screens ask the same question of
 * the same answer: the import wizard, which takes the messages out of a backup, and
 * the migration, which puts them into a copy of one. What a backup is called and
 * whether there is anything to show are facts about the answer, not about either
 * screen, and a second copy of them would be a second set of sentences to correct.
 */

/**
 * What a screen showing backups is actually showing, which is one of five things.
 *
 * Four of them are a kind of nothing and they are said differently on purpose. A
 * build without an importer has nothing to look with; a folder that could not be
 * read is a permission somebody can grant, and on macOS that is almost always what
 * this is; nowhere to look at all is a platform Apple ships nothing for, where the
 * answer is another computer rather than anything on this one; and an empty folder
 * means there really is no backup and one has to be made. Collapsing the four into
 * an empty list is how somebody concludes their history is gone — or is told to
 * open Finder on a machine that has never had one.
 */
export type Situation =
  "noImporter" | "problem" | "looking" | "nowhere" | "none" | "some";

/**
 * situationOf decides which of the five a screen is showing.
 *
 * A function rather than four nested conditions in the middle of a screen: what it
 * decides is a fact about an answer from the server, and it is the part worth being
 * able to read on its own.
 */
export function situationOf(
  missingImporter: boolean,
  problem: string | undefined,
  pending: boolean,
  count: number,
  /** Where the program looked. Empty means there was nowhere to look. */
  looked: readonly string[] | undefined,
): Situation {
  if (missingImporter) return "noImporter";
  if (problem !== undefined) return "problem";
  if (pending) return "looking";
  if (count > 0) return "some";
  // Undefined rather than empty is an older server that did not say where it
  // looked; assuming it looked somewhere keeps the message it used to give.
  return looked?.length === 0 ? "nowhere" : "none";
}

/** nameOf is what the phone called itself, or the plainest thing that is still true. */
export function nameOf(backup: Backup, t: Translate): string {
  const named = backup.device_name ?? "";
  if (named !== "") return named;
  const model = backup.product_type ?? "";
  return model === "" ? t("backupUnnamedDevice") : model;
}

/**
 * describe is the line that tells two backups apart: which iOS, and when.
 *
 * Each piece is left out when the backup's own index did not record it, rather than
 * shown as an empty gap, because an old backup written by an old iTunes is missing
 * about half of them. The date arrives as an instant in universal time and becomes
 * words here, through the same formatters every date in the viewer goes through, so
 * that a backup made last night says so in the reader's own zone.
 */
export function describe(
  backup: Backup,
  t: Translate,
  language: Language,
): string {
  const pieces: string[] = [];

  const version = backup.ios_version ?? "";
  if (version !== "") pieces.push(t("backupOS", { version }));

  const at = backup.last_backup;
  if (at !== undefined) {
    const day = shortDate(at, language);
    if (day !== "")
      pieces.push(
        t("backupWhen", { when: `${day}, ${timeOfDay(at, language)}` }),
      );
  }

  return pieces.join(" · ");
}
