import type { Finding, GuideStage, Migration } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Trouble } from "@/components/wizard/Failure";
import { Aside, Say, Shell } from "@/components/wizard/Shell";
import { Step } from "@/components/migration/Guide";
import { useT } from "@/i18n";
import type { Sentences } from "@/lib/said";
import { filled } from "@/lib/said";
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
  sentences,
  language,
  onPlan,
  onBack,
  busy,
  failure,
}: {
  state: Migration;
  stages: readonly GuideStage[];
  /** What the checks say they looked at, in the reader's own language. */
  sentences: Sentences;
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
            <Found finding={finding} sentences={sentences} />
          </li>
        ))}
      </ul>

      {blockers.length > 0 && (
        <Aside heading={t("migrateChecksBlocked")} tone="blocker">
          {blockers.map((blocker) => {
            const step = before.find((one) => one.id === blocker.step);
            return step === undefined ? (
              <Say key={blocker.title}>
                {sentence(blocker.note, blocker.detail, blocker, sentences) ??
                  sentence(blocker.check, blocker.title, blocker, sentences)}
              </Say>
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
 * One of a finding's two sentences, in the reader's own language.
 *
 * A finding names what it looked at and what it found; the guide carries both, in
 * whichever language the page asked for. The English travels beside the names and is
 * what shows when this build's guide has never heard of one — which happens when the
 * program is newer than the page it is serving, and is better than a blank line.
 */
function sentence(
  name: string | undefined,
  english: string | undefined,
  finding: Finding,
  sentences: Sentences,
): string | undefined {
  const template = name === undefined ? undefined : sentences?.[name];
  return template === undefined ? english : filled(template, finding.values);
}

/**
 * One finding: what was checked, and what was found.
 *
 * The verdict is a word before it is a mark. A tick and a cross carry the whole
 * meaning of this screen and neither is readable aloud, so the mark is decoration and
 * the word beside it — visible to a screen reader and to nothing else — is what says
 * whether this one passed.
 */
function Found({
  finding,
  sentences,
}: {
  finding: Finding;
  sentences: Sentences;
}) {
  const t = useT();

  const [tone, verdict] = finding.passed
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
          <span className="sr-only">{verdict} </span>
          {sentence(finding.check, finding.title, finding, sentences)}
        </span>
        {finding.detail !== undefined && (
          <span className="mt-0.5 block text-sm text-[var(--color-muted)]">
            {sentence(finding.note, finding.detail, finding, sentences)}
          </span>
        )}
      </span>
    </div>
  );
}
