import { useState } from "react";

import {
  carryOut,
  checkBackup,
  forgetMigration,
  planMigration,
} from "@/api/client";
import {
  useAdvice,
  useGuide,
  useMigration,
  useMigrationStep,
} from "@/api/queries";
import type { Migration as State } from "@/api/types";
import { MigrationChecks } from "@/components/migration/Checks";
import { MigrationDone } from "@/components/migration/Done";
import { MigrationPaths } from "@/components/migration/Paths";
import type { Paths } from "@/components/migration/Paths";
import { MigrationPlanned, theWord } from "@/components/migration/Plan";
import { Working } from "@/components/wizard/Working";
import { useT } from "@/i18n";
import { carried } from "@/lib/said";
import type { Language } from "@/i18n";

/**
 * What the working screen shows while the server is busy.
 *
 * It is the import wizard's screen, which wants a workspace it has no use for here,
 * so the empty string is deliberate rather than missing. The step and the detail are
 * only present once the server has said something; an absent one must stay absent
 * rather than arrive as undefined, which the screen would render as a blank line.
 */
function progress(now: State): Parameters<typeof Working>[0]["state"] {
  return {
    stage: "working",
    workspace: "",
    ...(now.step === undefined ? {} : { step: now.step }),
    ...carried(now),
  };
}

/** What the server said when it would not take an instruction. */
function Refused({ detail }: { detail: string }) {
  const t = useT();

  return (
    <p
      role="alert"
      className="m-0 border-b border-[var(--color-accent)] px-5 py-2 text-center text-sm font-medium"
    >
      {t("refused", { detail })}
    </p>
  );
}

/**
 * Moving a history onto a phone.
 *
 * The server owns how far along this is and this page cannot change that except by
 * asking. What it owns is what somebody has typed, which the server knows nothing
 * about and must not: a reload in the middle of a migration should show the migration,
 * not the form that started it.
 *
 * Nothing here chains. There is no effect that begins planning because checking
 * finished, and none that begins writing because a plan arrived. Every step happens
 * because somebody pressed something, which is the whole design and the easiest thing
 * to undo by accident.
 */
export function Migration({
  language,
  onLeave,
}: {
  language: Language;
  onLeave: () => void;
}) {
  const state = useMigration();
  const guide = useGuide(language);
  // Fetched while nothing has gone wrong, so the screen that explains a failure is
  // not the one waiting on a request. The answer is read from the cache wherever it
  // is needed.
  useAdvice(language);
  const step = useMigrationStep();

  const [paths, setPaths] = useState<Paths>({
    backup: "",
    android: "",
    pairing: "",
    groups: false,
    hidden: false,
  });
  const [into, setInto] = useState("");
  const [read, setRead] = useState(false);

  const now: State = state.data ?? { stage: "idle" };
  const stages = guide.data?.stages ?? [];

  // A failure is not the end of anything: the server takes the next attempt without
  // being reset, so it is shown above the screen that caused it rather than instead
  // of it, with what was typed still there to correct.
  const failure = now.stage === "failed" && !read ? now : undefined;

  const type = (change: Partial<Paths>) => {
    setPaths((was) => ({ ...was, ...change }));
  };

  const backToForm = () => {
    setRead(true);
    step.start(() => forgetMigration());
  };

  if (state.isPending || guide.isPending) {
    return (
      <Working
        state={{ stage: "working", workspace: "" }}
        language={language}
      />
    );
  }

  switch (now.stage) {
    case "checking":
    case "planning":
    case "working":
      return <Working state={progress(now)} language={language} />;

    case "checked":
      return (
        <MigrationChecks
          language={language}
          sentences={guide.data?.sentences}
          state={now}
          stages={stages}
          busy={step.busy}
          failure={failure}
          onBack={backToForm}
          onPlan={() => {
            setRead(false);
            step.start(() =>
              planMigration(paths.backup.trim(), paths.android.trim(), {
                pairing: paths.pairing.trim(),
                groups: paths.groups,
                hidden: paths.hidden,
              }),
            );
          }}
        />
      );

    case "planned":
      return (
        <MigrationPlanned
          sentences={guide.data?.sentences}
          state={now}
          language={language}
          into={into}
          onInto={setInto}
          busy={step.busy}
          failure={failure}
          onBack={backToForm}
          onCarryOut={() => {
            setRead(false);
            step.start(() => carryOut(theWord, into.trim()));
          }}
        />
      );

    case "done":
      return (
        <MigrationDone
          state={now}
          stages={stages}
          language={language}
          onAgain={() => {
            setRead(true);
            setInto("");
            step.start(() => forgetMigration());
          }}
        />
      );

    default:
      return (
        <>
          {step.refused !== undefined && <Refused detail={step.refused} />}
          <MigrationPaths
            paths={paths}
            language={language}
            onPaths={type}
            busy={step.busy}
            failure={failure}
            onBack={onLeave}
            onCheck={() => {
              setRead(false);
              step.start(() => checkBackup(paths.backup.trim()));
            }}
          />
        </>
      );
  }
}
