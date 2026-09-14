import type { GuideStage, GuideStep } from "@/api/types";
import { Aside, Say } from "@/components/wizard/Shell";
import { useT } from "@/i18n";

/**
 * What somebody has to be told, as the program wrote it.
 *
 * The sentences arrive from the server rather than living here, so correcting one
 * corrects it in the terminal too. They carry their own line breaks, which are
 * deliberate, and they are rendered as text: the day one of them quotes a file name
 * with an angle bracket in it has to be a day nothing happens.
 */
export function Guide({ stages }: { stages: readonly GuideStage[] }) {
  return (
    <div className="flex flex-col gap-8">
      {stages.map((stage) => (
        <section key={stage.stage} className="flex flex-col gap-4">
          <h2 className="m-0 text-base font-semibold">{stage.heading}</h2>
          <ol className="m-0 flex list-none flex-col gap-5 p-0">
            {stage.steps.map((step, at) => (
              <li key={step.id}>
                <Step step={step} number={at + 1} />
              </li>
            ))}
          </ol>
        </section>
      ))}
    </div>
  );
}

/**
 * One step.
 *
 * A step that loses something irreversibly when skipped is drawn differently, and
 * says so in words as well: three of them do, and a mark nobody can name is a mark
 * nobody acts on.
 */
export function Step({ step, number }: { step: GuideStep; number: number }) {
  const t = useT();
  const critical = step.critical === true;

  return (
    <article
      className={
        critical ? "rounded-lg p-4 ring-1 ring-[var(--color-accent)]" : "py-1"
      }
    >
      <h3 className="m-0 flex flex-wrap items-baseline gap-2 text-sm font-semibold">
        <span className="text-[var(--color-muted)]">{number}.</span>
        <span>{step.title}</span>
        {critical && (
          <span className="text-xs font-medium tracking-wide text-[var(--color-accent)] uppercase">
            {t("guideCritical")}
          </span>
        )}
      </h3>

      <pre className="m-0 mt-2 overflow-x-auto bg-transparent font-sans text-sm whitespace-pre-wrap">
        {step.body}
      </pre>

      {step.expect !== undefined && (
        <p className="m-0 mt-3 text-sm">
          <span className="font-medium">{t("guideExpect")}: </span>
          <span className="text-[var(--color-muted)]">{step.expect}</span>
        </p>
      )}

      {step.minutes !== undefined && (
        <p className="m-0 mt-2 text-xs text-[var(--color-muted)]">
          {t("guideTakes", { minutes: String(step.minutes) })}
        </p>
      )}
    </article>
  );
}

/** A note that the guidance has not been translated yet, shown only when it matters. */
export function NotTranslated({ language }: { language: string }) {
  const t = useT();
  if (language === "en") return null;

  return (
    <Aside heading={t("guideEnglishOnly")}>
      <Say>{t("migrateUnprovenHelp")}</Say>
    </Aside>
  );
}
