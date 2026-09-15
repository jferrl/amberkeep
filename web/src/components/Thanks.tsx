import { Coffee } from "lucide-react";

import { styles } from "@/components/ui/control";
import { useT } from "@/i18n";
import { outside } from "@/lib/desktop";
import { useThanks } from "@/lib/thanks";

/**
 * The one thing this program has to say about money.
 *
 * It was a line of eleven-point grey under two legal notices, which is not restraint
 * — it is hiding. A tip jar nobody sees earns nothing, and pretending otherwise is a
 * way of avoiding the decision rather than making it.
 *
 * So it is a control, with a coffee cup on it, in the places somebody actually looks:
 * at the foot of every screen of the wizard, in the sidebar of the archive they are
 * reading, and on the two screens that say something worked. Small, outlined, at the
 * bottom, never in the way of anything and never on a failure.
 *
 * Ko-fi publishes a button for this. It is a script tag pointing at their servers,
 * and embedding it would mean this page fetching something from somewhere else,
 * which it has never done and which there is a browser test to prevent. So the button
 * is drawn here, in this program's own palette, and clicking it hands the address to
 * the operating system. Nothing is loaded from anybody.
 */
export function Thanks({ full = false }: { full?: boolean }) {
  const t = useT();
  const where = useThanks();
  if (where === undefined || where === "") return null;

  return (
    <div className="flex flex-col items-start gap-1.5">
      {full && (
        <p className="m-0 text-[0.8125rem] text-[var(--color-muted)]">
          {t("thanksAsk")}
        </p>
      )}
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
        <Coffee aria-hidden className="size-3.5 text-[var(--color-accent)]" />
        {t("thanksButton")}
      </a>
    </div>
  );
}
