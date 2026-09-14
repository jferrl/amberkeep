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
import { Button } from "@/components/ui/button";
import { Shell } from "@/components/wizard/Shell";
import { useT } from "@/i18n";

/**
 * Work that did not finish, and what to do about it.
 *
 * The detail is the server's own sentence about what went wrong and the guidance is
 * several lines of what to try, written for somebody who has just been told no. The
 * guidance arrives with its line breaks already in it and is rendered as
 * preformatted text: a text node, wrapped, never markup. That is not a styling
 * choice — the sentences come out of the engine's error set, and the day one of them
 * quotes a file name with an angle bracket in it must be a day nothing happens.
 */
export function Trouble({
  state,
  correctable,
}: {
  state: Failed;
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

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-[var(--color-accent)] p-4">
      <p role="alert" className="m-0 text-lg font-medium">
        {state.detail ?? t("failedTitle")}
      </p>

      {state.guidance !== undefined && (
        <div className="flex flex-col gap-1.5">
          <h2 className="m-0 text-sm font-semibold">{t("failedGuidance")}</h2>
          <pre className="m-0 overflow-x-auto bg-[var(--color-surface)] p-3 font-sans text-sm whitespace-pre-wrap">
            {state.guidance}
          </pre>
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
  onBack,
}: {
  state: Failed;
  onBack: () => void;
}) {
  const t = useT();

  return (
    <Shell step={1} heading={t("failedTitle")}>
      <Trouble state={state} />
      <div>
        <Button variant="primary" onClick={onBack}>
          {t("tryAgain")}
        </Button>
      </div>
    </Shell>
  );
}
