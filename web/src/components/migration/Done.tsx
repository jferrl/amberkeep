import type { GuideStage, Migration } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Thanks } from "@/components/Thanks";
import { Guide } from "@/components/migration/Guide";
import { Aside, Say, Shell } from "@/components/wizard/Shell";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";
import { count } from "@/lib/format";

/**
 * A backup on disk, and a phone that has not been touched.
 *
 * The whole guide is here rather than a link to it, because this is the moment
 * somebody stops using the program and starts using Finder, and nothing after this
 * point can be corrected by anything on screen.
 */
export function MigrationDone({
  state,
  stages,
  language,
  onAgain,
}: {
  state: Migration;
  stages: readonly GuideStage[];
  language: Language;
  onAgain: () => void;
}) {
  const t = useT();
  const result = state.result;

  return (
    <Shell
      step={5}
      total={5}
      heading={t("migrateDoneTitle")}
      lead={t("migrateDoneHelp")}
    >
      {result !== undefined && (
        <div className="flex flex-col gap-3">
          <dl className="m-0 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5">
            {(
              [
                [result.added, t("migrateDoneAdded")],
                [result.checks, t("migrateDoneChecks")],
                [result.files, t("migrateDoneFiles")],
              ] as [number, string][]
            ).map(([n, said]) => (
              <div key={said} className="contents">
                <dt className="text-right text-sm font-medium tabular-nums">
                  {count(n, language)}
                </dt>
                <dd className="m-0 text-sm text-[var(--color-muted)]">
                  {said}
                </dd>
              </div>
            ))}
          </dl>

          <p className="m-0 text-sm">
            <span className="font-medium">{t("migrateDoneWhere")} </span>
            <span className="break-all text-[var(--color-muted)]">
              {result.backup}
            </span>
          </p>
        </div>
      )}

      <Aside heading={t("migrateUnproven")} tone="warning">
        <Say>{t("migrateUnprovenHelp")}</Say>
      </Aside>

      <Guide stages={stages} />

      <Thanks />

      <div>
        <Button variant="quiet" onClick={onAgain}>
          {t("migrateStartAgain")}
        </Button>
      </div>
    </Shell>
  );
}
