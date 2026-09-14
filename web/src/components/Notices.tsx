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
    <footer className="border-t border-[var(--color-line)] px-5 py-2.5 text-[0.6875rem] leading-snug text-[var(--color-muted)]">
      <div className="mx-auto max-w-2xl">
        <p className="m-0">{t("notAffiliated")}</p>
        <p className="m-0 mt-1">{t("freeSoftware")}</p>
      </div>
    </footer>
  );
}
