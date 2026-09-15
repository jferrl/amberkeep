import { useT } from "@/i18n";
import { outside } from "@/lib/desktop";
import { useThanks } from "@/lib/thanks";

/**
 * The two things this program has to say about itself, wherever somebody is looking.
 *
 * The first is a trademark matter and the second is a licence one, and both belong on
 * screen rather than only in a README nobody opens: the wizard and the archive are the
 * whole of what most people will ever see of this project.
 *
 * The address is text rather than a link on purpose. This page fetches nothing from
 * anywhere and there is a browser test that proves it; the one exception was never
 * going to be a link to a code-hosting company on every screen. Somebody who wants the
 * source can read it out and type it, which is what the licence asks for.
 */
export function Notices() {
  const t = useT();
  const where = useThanks();

  return (
    // mt-auto rather than a fixed position: on a short screen this sits at the
    // bottom of the window, and on a long one it is pushed below the fold and
    // scrolls away like the end of a document. What it must never do is cover
    // anything — it is a legal notice, not a tool.
    <footer className="mt-auto shrink-0 border-t border-[var(--color-line)] px-5 py-2.5 text-[0.6875rem] leading-snug text-[var(--color-muted)]">
      <div className="mx-auto max-w-2xl">
        <p className="m-0">{t("notAffiliated")}</p>
        <p className="m-0 mt-1">{t("freeSoftware")}</p>
        {/*
          And where to say thanks, for anybody who goes looking for it rather than
          waiting to be asked. A line among the notices rather than a button in the
          way: the asking is done once, on the screen that says something worked.
        */}
        {where !== undefined && where !== "" && (
          <p className="m-0 mt-1">
            {t("thanksFooter")}{" "}
            <a
              href={where}
              className="underline decoration-[var(--color-edge)] underline-offset-2 hover:text-[var(--color-ink)] hover:decoration-[var(--color-ink)]"
              onClick={(event) => {
                event.preventDefault();
                outside(where);
              }}
            >
              {where.replace(/^https?:\/\//, "")}
            </a>
          </p>
        )}
      </div>
    </footer>
  );
}
