import type { SyntheticEvent } from "react";
import { useId, useState } from "react";

import type { Migration } from "@/api/types";
import { Button } from "@/components/ui/button";
import { ChooseBackup } from "@/components/migration/ChooseBackup";
import { Field } from "@/components/wizard/Field";
import { Trouble } from "@/components/wizard/Failure";
import { Aside, Say, Shell } from "@/components/wizard/Shell";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";

/** What somebody types before anything is looked at. */
export interface Paths {
  backup: string;
  android: string;
  pairing: string;
  groups: boolean;
  hidden: boolean;
}

/**
 * Where the two halves are named.
 *
 * Both are already on this computer and neither is uploaded, which is said here
 * rather than left to be assumed: somebody about to hand over their entire message
 * history is entitled to be told where it is going, on the screen where they hand it
 * over.
 */
export function MigrationPaths({
  paths,
  onPaths,
  onCheck,
  onBack,
  busy,
  failure,
  language,
}: {
  paths: Paths;
  onPaths: (change: Partial<Paths>) => void;
  onCheck: () => void;
  onBack: () => void;
  busy: boolean;
  failure: Migration | undefined;
  language: Language;
}) {
  const t = useT();
  const [wrong, setWrong] = useState<{ backup?: string; android?: string }>({});

  const send = (event: SyntheticEvent) => {
    event.preventDefault();

    const missing: { backup?: string; android?: string } = {};
    if (paths.backup.trim() === "") missing.backup = t("fileNeeded");
    if (paths.android.trim() === "") missing.android = t("fileNeeded");
    setWrong(missing);
    if (Object.keys(missing).length === 0) onCheck();
  };

  return (
    <Shell
      step={2}
      total={5}
      heading={t("migratePathsTitle")}
      lead={t("migratePathsHelp")}
      trouble={failure === undefined ? undefined : <Trouble state={failure} correctable />}
      onBack={onBack}
    >
      <Aside heading={t("migrateUnproven")}>
        <Say>{t("migrateUnprovenHelp")}</Say>
      </Aside>

      <ChooseBackup
        chosen={paths.backup}
        busy={busy}
        language={language}
        onChoose={(backup) => {
          setWrong({});
          onPaths({ backup });
        }}
      />

      <form className="flex flex-col gap-5" onSubmit={send}>
        <Field
          label={t("migrateBackupLabel")}
          hint={t("migrateBackupHint")}
          value={paths.backup}
          wrong={wrong.backup}
          onChange={(backup) => {
            setWrong({});
            onPaths({ backup });
          }}
        />
        <Field
          label={t("migrateAndroidLabel")}
          hint={t("migrateAndroidHint")}
          value={paths.android}
          wrong={wrong.android}
          onChange={(android) => {
            setWrong({});
            onPaths({ android });
          }}
        />
        <Field
          label={t("migratePairingLabel")}
          hint={t("migratePairingHint")}
          value={paths.pairing}
          onChange={(pairing) => {
            onPaths({ pairing });
          }}
        />

        <Choice
          label={t("migrateGroups")}
          hint={t("migrateGroupsHint")}
          on={paths.groups}
          onChange={(groups) => {
            onPaths({ groups });
          }}
        />
        <Choice
          label={t("migrateHidden")}
          hint={t("migrateHiddenHint")}
          on={paths.hidden}
          onChange={(hidden) => {
            onPaths({ hidden });
          }}
        />

        <div>
          <Button type="submit" disabled={busy}>
            {t("migrateCheck")}
          </Button>
        </div>
      </form>
    </Shell>
  );
}

/**
 * One thing that is off unless somebody turns it on, with the reason beside it.
 *
 * Both of these widen what gets moved in ways that are hard to undo, so each says
 * what it costs rather than only what it does.
 */
function Choice({
  label,
  hint,
  on,
  onChange,
}: {
  label: string;
  hint: string;
  on: boolean;
  onChange: (on: boolean) => void;
}) {
  const box = useId();
  const note = useId();

  return (
    <div className="flex items-start gap-3">
      <input
        id={box}
        type="checkbox"
        checked={on}
        aria-describedby={note}
        className="mt-1 accent-[var(--color-accent)]"
        onChange={(event) => {
          onChange(event.target.checked);
        }}
      />
      <div>
        <label htmlFor={box} className="block cursor-pointer text-sm font-medium">
          {label}
        </label>
        <p id={note} className="m-0 mt-0.5 text-sm text-[var(--color-muted)]">
          {hint}
        </p>
      </div>
    </div>
  );
}
