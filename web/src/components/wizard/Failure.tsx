/**
 * Work that did not finish, whichever of the two jobs it was.
 *
 * Narrowed to the two fields this actually shows rather than taking a whole state,
 * because bringing an archive in and moving one onto a phone both fail the same way:
 * a sentence saying what went wrong, and several lines of what to try. Two components
 * saying that differently would be two chances to say it worse.
 */
export interface Failed {
  detail?: string | undefined;
  guidance?: string | undefined;
}
import { useAdvice } from "@/api/queries";
import { Button } from "@/components/ui/button";
import { Prose } from "@/components/wizard/Prose";
import { Shell } from "@/components/wizard/Shell";
import { useT } from "@/i18n";

/**
 * Work that did not finish, and what to do about it.
 *
 * The state names its advice rather than carrying it — the words come from the
 * program, in the reader's own language — and what it does carry is the engine's own
 * sentence about what went wrong, which is one line and is in English wherever the
 * failure was not one anybody anticipated. So the heading is the advice's when there
 * is advice, and the engine's sentence goes underneath it, where it keeps whatever
 * specifics it had: the name of a file, the path that was not there.
 *
 * Everything arrives with its line breaks already in it and is rendered as
 * preformatted text: a text node, wrapped, never markup. That is not a styling
 * choice — the sentences come out of the engine's error set, and the day one of them
 * quotes a file name with an angle bracket in it must be a day nothing happens.
 */
export function Trouble({
  state,
  language,
  correctable,
}: {
  state: Failed;
  /** Which language to say what to do in. */
  language: string;
  /**
   * Whether the thing that caused this is still on screen to be put right.
   *
   * A failure is not terminal: the server takes the next attempt without being
   * reset. Where the form that caused it is still there, saying so is the whole
   * instruction; on the screen that has no form behind it, it would be advice to
   * correct something that is not there.
   */
  correctable?: boolean;
}) {
  const t = useT();
  const advice = useAdvice(language);
  const told =
    state.guidance === undefined ? undefined : advice.data?.[state.guidance];

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-[var(--color-accent)] p-4">
      {/*
        Both lines are the alert, not just the first. What went wrong is a heading
        somebody can read at a glance and a sentence carrying the specifics — the
        name of the file, the path that was not there — and a screen reader that
        announced only the heading would leave out the half that says which file.
        The advice below is outside it: it is several lines, read at leisure.
      */}
      <div role="alert" className="flex flex-col gap-1">
        <p className="m-0 text-lg font-medium">
          {told?.title ?? state.detail ?? t("failedTitle")}
        </p>

        {told !== undefined && state.detail !== undefined && (
          <p className="m-0 text-sm text-[var(--color-muted)]">{state.detail}</p>
        )}
      </div>

      {told !== undefined && (
        <div className="flex flex-col gap-1.5">
          <h2 className="m-0 text-sm font-semibold">{t("failedGuidance")}</h2>
          <div className="rounded-md bg-[var(--color-surface)] p-3">
            <Prose text={told.body} />
          </div>
        </div>
      )}

      {correctable === true && (
        <p className="m-0 text-sm text-[var(--color-muted)]">
          {t("correctAndTryAgain")}
        </p>
      )}
    </div>
  );
}

/**
 * A failure with nothing to correct behind it.
 *
 * This is the screen somebody meets when they reload the page after the work failed,
 * or when the failure belongs to a route they have already left: the form that
 * caused it is gone, so the only thing left to offer is the way back to the start.
 * Everywhere else the same explanation appears above the form, which is both more
 * use and less work, and this screen exists only so that there is never a state the
 * wizard cannot be got out of.
 */
export function Failure({
  state,
  language,
  onBack,
}: {
  state: Failed;
  language: string;
  onBack: () => void;
}) {
  const t = useT();

  return (
    <Shell step={1} heading={t("failedTitle")}>
      <Trouble state={state} language={language} />
      <div>
        <Button variant="primary" onClick={onBack}>
          {t("tryAgain")}
        </Button>
      </div>
    </Shell>
  );
}
