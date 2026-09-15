import { styles } from "@/components/ui/control";
import { useT } from "@/i18n";
import { outside } from "@/lib/desktop";
import { useThanks } from "@/lib/thanks";

/**
 * The one line this program has about money.
 *
 * Shown twice in its life: when an archive has been written out, and when a
 * migration has produced the backup. Both are moments when something that mattered
 * has just worked, which is the only time a program has any business asking.
 *
 * The address comes from the state every screen already polls, and is absent until
 * there is somewhere to point at — a program that points somebody at a page that does not
 * exist has spent the only goodwill the line was ever going to earn.
 *
 * Ko-fi publishes a button for this. It is a script tag pointing at their servers,
 * and embedding it would mean this page fetching something from somewhere else,
 * which it has never done and which there is a browser test to prevent. So the
 * button is drawn here, in this program's own palette, and clicking it hands the
 * address to the operating system. Nothing is loaded from anybody.
 */
export function Thanks() {
  const t = useT();
  const where = useThanks();
  if (where === undefined || where === "") return null;

  return (
    <div className="flex flex-col items-start gap-2">
      <p className="m-0 text-[0.8125rem] text-[var(--color-muted)]">
        {t("thanksAsk")}
      </p>
      <a
        href={where}
        className={styles({ variant: "default", size: "sm" })}
        onClick={(event) => {
          // Never followed in place. In a window there is no address bar and no way
          // back, so a link followed there would replace the program with a web page.
          event.preventDefault();
          outside(where);
        }}
      >
        {t("thanksButton", { where: readable(where) })}
      </a>
    </div>
  );
}

/** readable is the address as somebody would write it down, without the scheme. */
function readable(address: string): string {
  return address.replace(/^https?:\/\//, "");
}
