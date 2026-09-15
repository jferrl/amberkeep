import { useT } from "@/i18n";

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

  return (
    // Pushed to the bottom by whatever sits above it rather than claiming mt-auto
    // itself: the coffee does that now, and two things both claiming the space left
    // one of them floating in the middle of a short screen. What this must never do
    // is cover anything — it is a legal notice, not a tool.
    <footer className="shrink-0 border-t border-[var(--color-line)] py-2.5 text-[0.6875rem] leading-snug text-[var(--color-muted)]">
      <div className="mx-auto w-full max-w-2xl px-5">
        <p className="m-0">{t("notAffiliated")}</p>
        <p className="m-0 mt-1">{t("freeSoftware")}</p>
      </div>
    </footer>
  );
}
