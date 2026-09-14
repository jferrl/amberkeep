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

/** The word somebody has to type. The server accepts nothing else. */
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
  const plan = state.plan;

  const agree = (event: SyntheticEvent) => {
    event.preventDefault();
    if (typed.trim() === theWord) onCarryOut();
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
        <Aside heading={t("migrateChecksBlocked")}>
          {plan.warnings.map((warning) => (
            <Say key={warning}>{warning}</Say>
          ))}
        </Aside>
      )}

      <LeftOut conversations={plan.conversations} />

      <form className="flex flex-col gap-4" onSubmit={agree}>
        <h2 className="m-0 text-sm font-semibold">{t("migrateAgreeTitle")}</h2>
        <Say>{t("migrateAgreeHelp", { word: theWord })}</Say>

        <Field
          label={t("migrateIntoLabel")}
          choosing={{ what: "folder", named: t("chooseInto") }}
          hint={t("migrateIntoHint")}
          value={into}
          onChange={onInto}
        />
        <Field
          label={t("migrateAgreeLabel", { word: theWord })}
          value={typed}
          autoComplete="off"
          onChange={setTyped}
        />

        <div>
          <Button type="submit" disabled={busy || typed.trim() !== theWord}>
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
