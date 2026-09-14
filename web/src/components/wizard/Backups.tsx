import { isMissingImporter, useBackups } from "@/api/queries";
import type { Backup, Setup } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Trouble } from "@/components/wizard/Failure";
import { Aside, Say, Shell } from "@/components/wizard/Shell";
import { Where } from "@/components/wizard/Where";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";
import { describe, nameOf, situationOf, type Situation } from "@/lib/backups";

/**
 * The iPhone backups this computer has already made.
 *
 * An encrypted one is listed rather than hidden. Somebody who cannot see the backup
 * they know exists concludes the program is broken, or worse, that the backup is;
 * seeing it there with a sentence about why it cannot be used and what to change is
 * the difference between a dead end and a next step.
 */
export function Backups({
  into,
  onInto,
  contacts,
  onContacts,
  onChoose,
  onBack,
  onInstead,
  busy,
  failure,
  language,
}: {
  into: string;
  onInto: (into: string) => void;
  contacts: string;
  onContacts: (contacts: string) => void;
  onChoose: (path: string) => void;
  onBack: () => void;
  /** The way out for a build that cannot import: open a file somebody already has. */
  onInstead: () => void;
  busy: boolean;
  failure: Setup | undefined;
  language: Language;
}) {
  const t = useT();
  const backups = useBackups();

  // A refusal by the server and a folder it was not allowed to read are the same
  // thing to the person reading this screen: something stopped it looking.
  const problem = backups.data?.problem ?? backups.error?.message;
  const found = backups.data?.backups ?? [];
  const situation = situationOf(
    isMissingImporter(backups.error),
    problem,
    backups.isPending,
    found.length,
  );

  return (
    <Shell
      step={2}
      total={2}
      heading={t("backupsTitle")}
      lead={situation === "some" ? t("backupsHelp") : undefined}
      trouble={failure === undefined ? undefined : <Trouble state={failure} correctable />}
      onBack={onBack}
    >
      {situation === "some" ? (
        <>
          <ul className="m-0 flex list-none flex-col gap-3 p-0">
            {found.map((backup) => (
              <li
                key={backup.path}
                className="rounded-lg border border-[var(--color-line)] bg-[var(--color-panel)] p-4"
              >
                <One backup={backup} busy={busy} language={language} onChoose={onChoose} />
              </li>
            ))}
          </ul>

          <Where
            into={into}
            onInto={onInto}
            writes="workspaceExtract"
            contacts={contacts}
            onContacts={onContacts}
          />
        </>
      ) : (
        <Instead situation={situation} said={problem ?? ""} onInstead={onInstead} />
      )}
    </Shell>
  );
}

/** Whichever of the three nothings, or the moment before the answer arrives. */
function Instead({
  situation,
  said,
  onInstead,
}: {
  situation: Situation;
  said: string;
  onInstead: () => void;
}) {
  const t = useT();

  switch (situation) {
    case "noImporter":
      return <NoImporter onInstead={onInstead} />;
    case "problem":
      return <Problem said={said} />;
    case "looking":
      return <Say>{t("backupsReading")}</Say>;
    default:
      return <NoBackups />;
  }
}

/** One backup, and either the way to use it or the reason there is none. */
function One({
  backup,
  busy,
  language,
  onChoose,
}: {
  backup: Backup;
  busy: boolean;
  language: Language;
  onChoose: (path: string) => void;
}) {
  const t = useT();

  return (
    <>
      <h2 className="m-0 text-base font-semibold">{nameOf(backup, t)}</h2>
      <p className="m-0 mt-0.5 text-sm text-[var(--color-muted)]">
        {describe(backup, t, language)}
      </p>
      <p className="m-0 mt-0.5 text-xs break-all text-[var(--color-muted)]">{backup.path}</p>

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
            disabled={busy}
            onClick={() => {
              onChoose(backup.path);
            }}
          >
            {t("backupUse")}
          </Button>
        </div>
      )}
    </>
  );
}

/** This build has no importer, so there was never anything here to look through. */
function NoImporter({ onInstead }: { onInstead: () => void }) {
  const t = useT();

  return (
    <>
      <Aside heading={t("noImporter")}>
        <Say>{t("noImporterHelp")}</Say>
      </Aside>
      <div>
        <Button onClick={onInstead}>{t("openAFileInstead")}</Button>
      </div>
    </>
  );
}

/**
 * Something stopped the server looking, which on macOS is a permission.
 *
 * What comes back is not one sentence. It is a sentence, a blank line, and then
 * several lines of what to do about it with the exact folder in them — which is
 * more use than anything this page could compose, because only the server knows
 * where it was actually refused. So it is split where the server split it: the
 * first paragraph is the alert, and the rest is rendered with its line breaks
 * intact, as a text node and never as markup.
 *
 * This page's own advice is kept for the server that sends a bare sentence. Being
 * told it is a permission without being told where to grant it is no use at all.
 */
function Problem({ said }: { said: string }) {
  const t = useT();
  const [sentence, ...rest] = said.split(/\n{2,}/);
  const advice = rest.join("\n\n").trim();

  return (
    <Aside heading={t("backupsProblem")}>
      <p role="alert" className="m-0 font-medium">
        {sentence ?? said}
      </p>
      {advice === "" ? (
        <Say>{t("backupsFullDisk")}</Say>
      ) : (
        <pre className="m-0 overflow-x-auto bg-[var(--color-paper)] p-3 font-sans text-sm whitespace-pre-wrap">
          {advice}
        </pre>
      )}
    </Aside>
  );
}

/** There really are none, which is a thing somebody can go and fix in Finder. */
function NoBackups() {
  const t = useT();

  return (
    <Aside heading={t("backupsNone")}>
      <Say>{t("backupsNoneHelp")}</Say>
    </Aside>
  );
}

