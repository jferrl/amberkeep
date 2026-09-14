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
  wayOn = true,
  heading,
  lead,
  trouble,
  onBack,
  children,
}: {
  step: number;
  total?: number | undefined;
  /**
   * Whether this screen has a next one.
   *
   * A count says "you are part of the way through something", which on a screen
   * nobody can get past is untrue at the moment somebody is most likely to close the
   * window. The screen that finds no backup, or is refused permission to look, is
   * one of those: it reported "Step 2 of 2" beside instructions to go and use System
   * Settings.
   */
  wayOn?: boolean;
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
    <main className="mx-auto flex w-full max-w-2xl flex-col gap-7 px-5 py-10">
      <div className="flex flex-col gap-3">
        {wayOn && <Progress step={step} total={total} />}
        <div className="flex flex-col gap-2">
          <h1
            ref={title}
            tabIndex={-1}
            className="m-0 text-[1.625rem] leading-[1.2] font-semibold outline-none"
          >
            {heading}
          </h1>
          {lead !== undefined && (
            <p className="m-0 max-w-[62ch] text-[0.9375rem] text-[var(--color-muted)]">
              {lead}
            </p>
          )}
        </div>
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
 * How far along this is, drawn rather than announced.
 *
 * The count matters — somebody with a dead phone has no idea how long this takes —
 * but it was set as a tracked uppercase kicker above the heading, which is the
 * decoration every generated page puts there whether or not it means anything. Here
 * the same fact is a filled rule: readable at a glance, and it stops competing with
 * the heading for the top of the page.
 *
 * The first screen has no total, because it genuinely is not known yet: an iPhone
 * backup takes two screens and an Android phone takes six.
 */
function Progress({
  step,
  total,
}: {
  step: number;
  total?: number | undefined;
}) {
  const t = useT();
  const said =
    total === undefined ? t("step", { step }) : t("stepOf", { step, total });

  return (
    <div className="flex items-center gap-3">
      {total !== undefined && (
        <div
          className="flex h-1 w-24 gap-0.5 overflow-hidden rounded-full"
          // One element, one label. Twenty divs each announcing themselves is how a
          // progress bar becomes unreadable to everything except eyes.
          role="img"
          aria-label={said}
        >
          {Array.from({ length: total }, (_, at) => (
            <span
              key={at}
              className={`h-full flex-1 rounded-full ${
                at < step
                  ? "bg-[var(--color-accent)]"
                  : "bg-[var(--color-line)]"
              }`}
            />
          ))}
        </div>
      )}
      <p
        aria-hidden={total !== undefined}
        className="m-0 text-[0.8125rem] text-[var(--color-muted)]"
      >
        {said}
      </p>
    </div>
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
  const t = useT();

  // No box. A preview of the next screen is the quietest thing on this one, and it
  // was wearing the same container as a blocking failure.
  return (
    <p className="m-0 text-[0.8125rem] text-[var(--color-muted)]">
      <span className="font-medium text-[var(--color-ink)]">
        {t("expectLabel")}{" "}
      </span>
      {children}
    </p>
  );
}

/**
 * How much a box wants to be believed.
 *
 * One treatment carried four meanings: a helpful tip about a phone, a permanent
 * disclaimer about what this program has never been proved to do, a check that
 * blocks the next step, and a failure that has just happened. Four boxes, one grey,
 * two of them stacked adjacently on the screen where somebody hands over their
 * history. A reader had no way to tell which was which without reading all of them.
 *
 * Three tones, and the difference is structural rather than only coloured: a note is
 * unmarked, a warning is ringed in the accent, and a blocker is filled. That way it
 * survives a printout, and it survives whoever cannot tell the amber from the grey.
 */
export type Tone = "note" | "warning" | "blocker";

const tones: Record<Tone, string> = {
  note: "bg-[var(--color-surface)] ring-[var(--color-line)]",
  warning: "bg-[var(--color-surface)] ring-[var(--color-accent)]",
  blocker: "bg-[var(--color-alarm-wash)] ring-[var(--color-alarm)]",
};

/** A box of advice beside something that went wrong or cannot be used. */
export function Aside({
  heading,
  tone = "note",
  children,
}: {
  heading?: string;
  tone?: Tone;
  children: ReactNode;
}) {
  return (
    <div
      className={`flex flex-col gap-2 rounded-lg p-4 text-[0.8125rem] ring-1 ring-inset ${tones[tone]}`}
    >
      {heading !== undefined && (
        <p className="m-0 text-sm font-semibold">{heading}</p>
      )}
      {children}
    </div>
  );
}
