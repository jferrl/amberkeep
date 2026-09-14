import { useEffect, useRef } from "react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/i18n";

/**
 * The frame every screen of the wizard sits in.
 *
 * The step number is here rather than in each screen because the person using this
 * has a dead phone and no idea how long the process is; a count that appears on some
 * screens and not others reads as the program having lost its place. The total is
 * absent on the first screen alone, where it genuinely is not known yet: choosing an
 * iPhone backup takes two screens and walking through an Android phone takes six.
 */
export function Shell({
  step,
  total,
  heading,
  lead,
  trouble,
  onBack,
  children,
}: {
  step: number;
  total?: number | undefined;
  heading: string;
  lead?: string | undefined;
  /**
   * Work that failed, shown above the screen that started it rather than instead
   * of it.
   *
   * A failure here is never the end of anything: the server will accept the next
   * attempt without being reset, so the form that was filled in wrongly stays on
   * screen with what was typed still in it, and the explanation appears above it.
   * Replacing the screen would throw away the very thing that has to be corrected.
   */
  trouble?: ReactNode;
  onBack?: (() => void) | undefined;
  children: ReactNode;
}) {
  const t = useT();
  const title = useRef<HTMLHeadingElement>(null);

  /**
   * A wizard replaces everything on the screen without the page moving. A sighted
   * reader sees that; a screen reader carries on from wherever it was, and a
   * keyboard is left on a button that no longer exists. Moving focus to the new
   * heading is what makes one screen becoming another perceivable rather than
   * merely visible.
   */
  useEffect(() => {
    title.current?.focus();
  }, [heading]);

  return (
    <main className="mx-auto flex w-full max-w-2xl flex-col gap-6 px-5 py-8">
      <div>
        <p className="m-0 text-xs font-semibold tracking-wide text-[var(--color-accent)] uppercase">
          {total === undefined ? t("step", { step }) : t("stepOf", { step, total })}
        </p>
        <h1 ref={title} tabIndex={-1} className="m-0 mt-1 text-2xl font-semibold outline-none">
          {heading}
        </h1>
        {lead !== undefined && <p className="m-0 mt-2 text-[var(--color-muted)]">{lead}</p>}
      </div>

      {trouble}

      {children}

      {onBack !== undefined && (
        <div>
          <Button variant="quiet" onClick={onBack}>
            {t("back")}
          </Button>
        </div>
      )}
    </main>
  );
}

/**
 * A paragraph of the wizard's own prose.
 *
 * Everything the wizard says is a text node. None of it is ever markup, for the same
 * reason no message ever is: the moment one paragraph is allowed to carry markup,
 * the next one to be written carries something the archive supplied.
 */
export function Say({ children }: { children: ReactNode }) {
  return <p className="m-0 text-[var(--color-ink)]">{children}</p>;
}

/**
 * What the next screen will look like, so somebody on a phone knows they are on
 * course before they act rather than after.
 */
export function Expect({ children }: { children: ReactNode }) {
  return (
    <p className="m-0 border-l-2 border-[var(--color-accent)] pl-3 text-sm text-[var(--color-muted)]">
      {children}
    </p>
  );
}

/** A box of advice beside something that went wrong or cannot be used. */
export function Aside({ heading, children }: { heading?: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2 rounded-lg border border-[var(--color-line)] bg-[var(--color-panel)] p-3.5 text-sm">
      {heading !== undefined && <p className="m-0 font-semibold">{heading}</p>}
      {children}
    </div>
  );
}
