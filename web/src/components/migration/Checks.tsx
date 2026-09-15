import type { Finding, GuideStage, Migration } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Trouble } from "@/components/wizard/Failure";
import { Aside, Say, Shell } from "@/components/wizard/Shell";
import { Step } from "@/components/migration/Guide";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";

/**
 * What could be checked, and the far larger part that could not.
 *
 * Four things are checked here and one of them can never pass: nothing on this
 * computer can see whether somebody made a safety backup and archived it. It is shown
 * anyway, every time, because leaving it out for being uncheckable is how the most
 * important step becomes the invisible one.
 */
export function MigrationChecks({
  state,
  stages,
  language,
  onPlan,
  onBack,
  busy,
  failure,
}: {
  state: Migration;
  stages: readonly GuideStage[];
  language: Language;
  onPlan: () => void;
  onBack: () => void;
  busy: boolean;
  failure: Migration | undefined;
}) {
  const t = useT();
  const findings = state.checks?.findings ?? [];
  const blockers = findings.filter((f) => f.blocking && !f.passed);
  const before = stages.find((s) => s.stage === "before")?.steps ?? [];

  return (
    <Shell
      step={3}
      total={5}
      heading={t("migrateChecksTitle")}
      lead={t("migrateChecksHelp")}
      trouble={
        failure === undefined ? undefined : (
          <Trouble state={failure} language={language} correctable />
        )
      }
      onBack={onBack}
    >
      <ul className="m-0 flex list-none flex-col gap-2 p-0">
        {findings.map((finding) => (
          <li key={finding.step + finding.title}>
            <Found finding={finding} />
          </li>
        ))}
      </ul>

      {blockers.length > 0 && (
        <Aside heading={t("migrateChecksBlocked")} tone="blocker">
          {blockers.map((blocker) => {
            const step = before.find((one) => one.id === blocker.step);
            return step === undefined ? (
              <Say key={blocker.title}>{blocker.detail ?? blocker.title}</Say>
            ) : (
              <Step
                key={blocker.step}
                step={step}
                number={before.indexOf(step) + 1}
              />
            );
          })}
        </Aside>
      )}

      <div className="flex flex-wrap gap-2">
        <Button
          variant="primary"
          disabled={busy || blockers.length > 0}
          onClick={onPlan}
        >
          {t("migratePlanIt")}
        </Button>
        <Button variant="quiet" onClick={onBack}>
          {t("migrateChangeThings")}
        </Button>
      </div>
    </Shell>
  );
}

/**
 * One finding: what was checked, and what was found.
 *
 * The verdict is a word before it is a mark. A tick and a cross carry the whole
 * meaning of this screen and neither is readable aloud, so the mark is decoration and
 * the word beside it — visible to a screen reader and to nothing else — is what says
 * whether this one passed.
 */
function Found({ finding }: { finding: Finding }) {
  const t = useT();

  const [tone, said] = finding.passed
    ? ["text-[var(--color-muted)]", t("migrateCheckPassed")]
    : finding.blocking
      ? ["text-[var(--color-accent)]", t("migrateCheckFailed")]
      : ["text-[var(--color-ink)]", t("migrateCheckWarned")];

  return (
    <div className="flex items-start gap-3">
      <span aria-hidden className={`mt-0.5 text-sm ${tone}`}>
        {finding.passed ? "✓" : finding.blocking ? "✕" : "!"}
      </span>
      <span>
        <span className="block text-sm">
          <span className="sr-only">{said} </span>
          {finding.title}
        </span>
        {finding.detail !== undefined && (
          <span className="mt-0.5 block text-sm text-[var(--color-muted)]">
            {finding.detail}
          </span>
        )}
      </span>
    </div>
  );
}
