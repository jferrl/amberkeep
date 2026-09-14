import type { SyntheticEvent } from "react";
import { useState } from "react";

import type {
  Migration,
  MigrationConversation,
  MigrationPlan,
} from "@/api/types";
import { Button } from "@/components/ui/button";
import { Field } from "@/components/wizard/Field";
import { Trouble } from "@/components/wizard/Failure";
import { Aside, Say, Shell } from "@/components/wizard/Shell";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";
import { count, dayOf } from "@/lib/format";

/**
 * What the server is sent when somebody agrees. It accepts nothing else.
 *
 * This is the wire value, not the word on screen. A Spanish reader was being asked
 * to type an English word at the one gate whose whole purpose is deliberate
 * comprehension — copying five letters you do not read is the opposite of the thing
 * being asked for. They type a word they understand; this is what travels.
 */
export const theWord = "migrate";

/**
 * What would move, and the one place somebody agrees to it.
 *
 * Nothing has been written when this is on screen and nothing will be until the word
 * is typed. The limitations are shown as words rather than left in the numbers,
 * because "99,982 as placeholders" is a statistic and "nearly a hundred thousand
 * photographs will arrive as a line of text" is a thing somebody can decide about.
 */
export function MigrationPlanned({
  state,
  language,
  into,
  onInto,
  onCarryOut,
  onBack,
  busy,
  failure,
}: {
  state: Migration;
  language: Language;
  into: string;
  onInto: (into: string) => void;
  onCarryOut: () => void;
  onBack: () => void;
  busy: boolean;
  failure: Migration | undefined;
}) {
  const t = useT();
  const [typed, setTyped] = useState("");
  const [wrong, setWrong] = useState<string | undefined>(undefined);
  const plan = state.plan;

  // The word in the reader's own language. The server's word is sent regardless.
  const said = t("confirmWord");

  const agree = (event: SyntheticEvent) => {
    event.preventDefault();
    if (typed.trim() === said) {
      onCarryOut();
      return;
    }
    // Said rather than silently refused. This screen's own sibling argues that a
    // control somebody can reach and not use is a promise the screen cannot keep,
    // and a button that just stays dim on a capital letter is exactly that.
    setWrong(t("migrateAgreeWrong", { word: said }));
  };

  if (plan === undefined || plan.adding === 0) {
    return (
      <Shell step={4} total={5} heading={t("migratePlanTitle")} onBack={onBack}>
        <Say>{t("migrateNothingToMove")}</Say>
      </Shell>
    );
  }

  return (
    <Shell
      step={4}
      total={5}
      heading={t("migratePlanTitle")}
      lead={t("migratePlanHelp")}
      trouble={
        failure === undefined ? undefined : (
          <Trouble state={failure} correctable />
        )
      }
      onBack={onBack}
    >
      <Totals plan={plan} language={language} />

      {plan.warnings !== undefined && plan.warnings.length > 0 && (
        <Aside heading={t("migrateChecksBlocked")} tone="warning">
          {plan.warnings.map((warning) => (
            <Say key={warning}>{warning}</Say>
          ))}
        </Aside>
      )}

      <LeftOut conversations={plan.conversations} />

      {/*
        The warning this program leads with, on the one screen where it was missing.
        It was shown before anybody had invested anything, and again after it was
        already done, and not at the moment consent is actually given.
      */}
      <Aside heading={t("migrateUnproven")} tone="warning">
        <Say>{t("migrateUnprovenHelp")}</Say>
      </Aside>

      <form className="flex flex-col gap-4" onSubmit={agree}>
        <h2 className="m-0 text-sm font-semibold">{t("migrateAgreeTitle")}</h2>

        {/* What is about to be read and written, named again. It was chosen two
            screens ago and has not been on screen since. */}
        {state.backup !== undefined && (
          <Say>{t("migrateAgreeChosen", { backup: state.backup })}</Say>
        )}

        <Field
          label={t("migrateIntoLabel")}
          choosing={{ what: "folder", named: t("chooseInto") }}
          hint={t("migrateIntoHint")}
          value={into}
          onChange={onInto}
        />

        <Say>{t("migrateAgreeHelp", { word: said })}</Say>
        <Field
          label={t("migrateAgreeLabel", { word: said })}
          value={typed}
          wrong={wrong}
          autoComplete="off"
          onChange={(value) => {
            setWrong(undefined);
            setTyped(value);
          }}
        />

        <div>
          {/* Not the same button as "Next". This is the one that writes. */}
          <Button variant="commit" type="submit" disabled={busy}>
            {t("migrateCarryOut")}
          </Button>
        </div>
      </form>
    </Shell>
  );
}

/** The numbers, each with the sentence that says what it means. */
function Totals({
  plan,
  language,
}: {
  plan: MigrationPlan;
  language: Language;
}) {
  const t = useT();

  const lines: [number, string][] = [
    [plan.adding, t("migrateAdding")],
    [plan.already_there, t("migrateAlready")],
    [plan.as_placeholders, t("migrateAsText")],
    [plan.untranslatable, t("migrateNotCarried")],
    [plan.creating, t("migrateCreating")],
    [plan.merging, t("migrateMerging")],
    [plan.untouched, t("migrateUntouched")],
  ];

  return (
    <div className="flex flex-col gap-3">
      <dl className="m-0 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5">
        {lines.map(([n, said]) => (
          <div key={said} className="contents">
            <dt className="text-right text-sm font-medium tabular-nums">
              {count(n, language)}
            </dt>
            <dd className="m-0 text-sm text-[var(--color-muted)]">{said}</dd>
          </div>
        ))}
      </dl>

      {plan.earliest !== undefined && plan.latest !== undefined && (
        <p className="m-0 text-sm text-[var(--color-muted)]">
          {t("migrateBetween", {
            from: dayOf(plan.earliest, language),
            to: dayOf(plan.latest, language),
          })}
        </p>
      )}
    </div>
  );
}

/**
 * The conversations nothing would happen to, and why.
 *
 * Shown rather than counted. A conversation left out silently is one somebody finds
 * missing weeks later, on a phone they can no longer compare against.
 */
function LeftOut({
  conversations,
}: {
  conversations: readonly MigrationConversation[];
}) {
  const t = useT();
  const skipped = conversations.filter(
    (c) => c.skipped !== undefined && c.skipped !== "",
  );
  if (skipped.length === 0) return null;

  return (
    <details className="rounded-lg border border-[var(--color-line)] p-3">
      <summary className="cursor-pointer text-sm font-medium">
        {t("migrateSkipped")} ({skipped.length})
      </summary>
      <ul className="m-0 mt-3 flex list-none flex-col gap-1.5 p-0">
        {skipped.map((c) => (
          <li key={c.address} className="text-sm">
            <span>{c.name}</span>
            <span className="text-[var(--color-muted)]"> — {c.skipped}</span>
          </li>
        ))}
      </ul>
    </details>
  );
}
