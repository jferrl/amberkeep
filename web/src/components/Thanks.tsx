import { useT } from "@/i18n";

/**
 * The one line this program has about money.
 *
 * Shown twice in its life: when an archive has been written out, and when a
 * migration has produced the backup. Both are moments when something that mattered
 * has just worked, which is the only time a program has any business asking.
 *
 * It arrives with the result rather than being fetched, and it is absent until there
 * is somewhere to point at — a program that points somebody at a page that does not
 * exist has spent the only goodwill the line was ever going to earn.
 *
 * Quiet on purpose: the smallest type on the screen, under everything that matters,
 * no button, no badge, nothing to dismiss because there is nothing in the way.
 */
export function Thanks({ where }: { where: string | undefined }) {
  const t = useT();
  if (where === undefined || where === "") return null;

  return (
    <p className="m-0 text-[0.8125rem] text-[var(--color-muted)]">
      {t("thanksAsk", { where })}
    </p>
  );
}
