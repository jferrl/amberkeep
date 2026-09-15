import { useBackups, useOfferedBackups, useWhatToDo } from "@/api/queries";
import type { Backup } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Prose } from "@/components/wizard/Prose";
import { Aside, Say } from "@/components/wizard/Shell";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";
import { describe, nameOf, type Situation } from "@/lib/backups";

/**
 * The backups this computer has already made, offered rather than described.
 *
 * The screen used to ask somebody to type "the folder named after a long string of
 * letters and numbers, inside the place Finder keeps backups". The program already
 * knows where every one of them is — the import wizard has listed them since the
 * beginning — so asking was never a limitation, only an oversight, and it was the
 * worst sentence in the product.
 *
 * Picking rather than typing also moves two of the checks forward. Whether a backup
 * is encrypted and when it was last made are both visible here, before anybody has
 * committed to one, instead of arriving as a finding after they have.
 *
 * Typing stays possible underneath: a backup on an external disk, or a copy somebody
 * made for safety, is not in the list and is still perfectly good.
 */
export function ChooseBackup({
  chosen,
  onChoose,
  language,
  busy,
}: {
  /** The path in the field, so the one already picked can say so. */
  chosen: string;
  onChoose: (path: string) => void;
  language: Language;
  busy: boolean;
}) {
  const t = useT();
  const backups = useBackups();
  const { situation, problem, guidance } = useOfferedBackups();
  const found = backups.data?.backups ?? [];

  if (situation !== "some")
    return (
      <Nothing
        situation={situation}
        said={problem ?? ""}
        guidance={guidance}
        language={language}
      />
    );

  return (
    <section className="flex flex-col gap-3">
      <h2 className="m-0 text-sm font-semibold">{t("migrateBackupFound")}</h2>

      <ul className="m-0 flex list-none flex-col gap-3 p-0">
        {found.map((backup) => (
          <li
            key={backup.path}
            className="rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)] p-4"
          >
            <One
              backup={backup}
              chosen={backup.path === chosen}
              busy={busy}
              language={language}
              onChoose={onChoose}
            />
          </li>
        ))}
      </ul>
    </section>
  );
}

/** One backup, and either the way to pick it or the reason it cannot be picked. */
function One({
  backup,
  chosen,
  busy,
  language,
  onChoose,
}: {
  backup: Backup;
  chosen: boolean;
  busy: boolean;
  language: Language;
  onChoose: (path: string) => void;
}) {
  const t = useT();

  return (
    <>
      <div className="flex flex-wrap items-baseline gap-x-2">
        <h3 className="m-0 text-base font-semibold">{nameOf(backup, t)}</h3>
        {chosen && (
          <span className="text-sm font-medium text-[var(--color-accent)]">
            {t("migrateBackupChosen")}
          </span>
        )}
      </div>
      <p className="m-0 mt-0.5 text-sm text-[var(--color-muted)]">
        {describe(backup, t, language)}
      </p>
      <p className="m-0 mt-0.5 text-xs break-all text-[var(--color-muted)]">
        {backup.path}
      </p>

      {backup.encrypted ? (
        // No button at all rather than one that is dimmed. A control somebody can
        // reach and not use is a promise this screen cannot keep, and the four
        // sentences are more use than a tooltip nobody on a keyboard will find.
        <div className="mt-3 flex flex-col gap-2 border-t border-[var(--color-line)] pt-3 text-sm">
          <p className="m-0 font-semibold">{t("backupEncrypted")}</p>
          <Say>{t("backupEncryptedWhy")}</Say>
          <Say>{t("backupEncryptedFix")}</Say>
          <Say>{t("backupEncryptedWarn")}</Say>
        </div>
      ) : (
        <div className="mt-3">
          <Button
            variant={chosen ? "quiet" : "default"}
            disabled={busy || chosen}
            onClick={() => {
              onChoose(backup.path);
            }}
          >
            {chosen ? t("migrateBackupChosen") : t("backupUse")}
          </Button>
        </div>
      )}
    </>
  );
}

/**
 * Whichever of the four nothings.
 *
 * None of them is a dead end here, because typing a path is still open underneath,
 * so each says what it is and stops rather than sending somebody away.
 */
function Nothing({
  situation,
  said,
  guidance,
  language,
}: {
  situation: Situation;
  said: string;
  guidance: string | undefined;
  language: Language;
}) {
  const t = useT();
  const told = useWhatToDo(guidance, language);

  switch (situation) {
    case "looking":
      return <Say>{t("backupsReading")}</Say>;

    case "problem": {
      // The program says what stopped it; what to do about it comes from the guide,
      // in whichever language this page asked for.
      return (
        <Aside heading={t("backupsProblem")} tone="blocker">
          <p role="alert" className="m-0 font-medium">
            {said}
          </p>
          {/*
            Folded away rather than shown, which is the one thing this screen does
            differently from the import wizard's. There, granting the permission is
            the only way forward and the instructions are the screen. Here the field
            below takes a typed path and works, so several paragraphs of System
            Settings navigation would bury the thing somebody can actually do.
          */}
          <details>
            <summary className="cursor-pointer text-sm font-medium">
              {t("migrateBackupHowToFix")}
            </summary>
            <div className="mt-2">
              {told === undefined ? (
                <Say>{t("backupsFullDisk")}</Say>
              ) : (
                <Prose text={told.body} />
              )}
            </div>
          </details>
        </Aside>
      );
    }

    case "none":
      return (
        <Aside heading={t("backupsNone")}>
          <Say>{t("backupsNoneHelp")}</Say>
        </Aside>
      );

    // Apple ships no Finder, iTunes or Apple Devices for this platform, so there is
    // nowhere for a backup to be and nothing here that could restore one. Saying
    // "no backup was found, open Finder" to somebody on Linux is a dead end dressed
    // up as an instruction.
    case "nowhere":
      return (
        <Aside heading={t("backupsNowhere")}>
          <Say>{t("backupsNowhereHelp")}</Say>
          <Say>{t("migrateRestoreElsewhere")}</Say>
        </Aside>
      );

    // A build without the part that reads backups cannot migrate at all, so this
    // screen is unreachable in one. Said rather than left blank all the same.
    default:
      return (
        <Aside heading={t("noImporter")}>
          <Say>{t("noImporterHelp")}</Say>
        </Aside>
      );
  }
}
