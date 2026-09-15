import { useT } from "@/i18n";
import { outside } from "@/lib/desktop";

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
      {t("thanksAsk", { where: readable(where) })}{" "}
      <a
        href={where}
        className="underline decoration-[var(--color-edge)] underline-offset-4 hover:text-[var(--color-ink)] hover:decoration-[var(--color-ink)]"
        onClick={(event) => {
          // Never followed in place. In a window that would replace the program
          // with a web page and leave somebody with no way back.
          event.preventDefault();
          outside(where);
        }}
      >
        {readable(where)}
      </a>
    </p>
  );
}

/** readable is the address as somebody would write it down, without the scheme. */
function readable(address: string): string {
  return address.replace(/^https?:\/\//, "");
}
