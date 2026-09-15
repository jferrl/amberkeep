import type { Setup, SetupStep } from "@/api/types";
import { Shell } from "@/components/wizard/Shell";
import { useT } from "@/i18n";
import type { Language, Phrase } from "@/i18n";
import { said } from "@/lib/said";

/**
 * What each part of the work is, said as a sentence rather than named as a stage.
 *
 * Partial rather than complete on purpose: a server newer than this page can report
 * a step this build has never heard of, and the honest answer to that is a plainer
 * sentence, not a blank screen or the word "undefined".
 */
const sentences: Partial<Record<SetupStep, Phrase>> = {
  extracting: "workingExtracting",
  decrypting: "workingDecrypting",
  preparing: "workingPreparing",
  indexing: "workingIndexing",
  opening: "workingOpening",
};

/**
 * Roughly how long each step takes, for the steps that take long enough to worry
 * somebody.
 *
 * Not a countdown and not a bar — a sentence, said once, in the units a person
 * thinks in. It is the honest half of the decision below: this program will not
 * pretend to know how far through it is, but it does know from measurement that
 * decrypting a 236 MB backup is seconds and building an index over a million
 * messages is minutes, and saying so is the difference between waiting and
 * wondering whether to kill it.
 *
 * Absent for the quick steps, because "about a second" on screen is noise.
 */
const takes: Partial<Record<SetupStep, Phrase>> = {
  decrypting: "workingTakesAMoment",
  indexing: "workingTakesMinutes",
  writing: "workingTakesMinutes",
  extracting: "workingTakesAMoment",
};

/**
 * Work in progress, reported rather than estimated.
 *
 * The server's own sentence is the big line and this page's is the small one
 * underneath, which is the opposite of the usual arrangement and is deliberate. The
 * detail is the only thing that changes over the several minutes this can take — it
 * counts megabytes and messages — so it is the only evidence anybody has that the
 * program has not hung. Burying it under a fixed heading would leave a screen that
 * looks identical for four minutes.
 *
 * There is no bar here, and that is a decision rather than an omission. Not one of
 * these steps can say how far through it is: decrypting knows the size of the file
 * and nothing about how much of it has been understood, and building indexes knows
 * neither. A bar that fills at a rate somebody made up is a lie told to a person who
 * is already worried, and when it sticks at ninety per cent they conclude the
 * program has crashed and kill it halfway through writing a database.
 */
export function Working({
  state,
  language,
}: {
  state: Setup;
  language: Language;
}) {
  const t = useT();
  const step = state.step === undefined ? undefined : sentences[state.step];
  const doing = t(step ?? "workingSomething");
  const howLong = state.step === undefined ? undefined : takes[state.step];

  // The server names the sentence rather than only writing it, so it is composed
  // here, where the reader's language and the way they write a thousand are known.
  const line = said(state, language, t) ?? doing;

  return (
    <Shell step={1} heading={t("workingTitle")}>
      <div className="flex items-start gap-3">
        <span
          aria-hidden="true"
          className="mt-1.5 size-3 shrink-0 animate-pulse rounded-full bg-[var(--color-accent)]"
        />
        <div className="flex flex-col gap-1" aria-live="polite">
          <p className="m-0 text-lg font-medium">{line}</p>
          {line !== doing && (
            <p className="m-0 text-sm text-[var(--color-muted)]">{doing}</p>
          )}
        </div>
      </div>

      {howLong !== undefined && (
        <p className="m-0 text-[0.8125rem] text-[var(--color-muted)]">
          {t(howLong)}
        </p>
      )}
    </Shell>
  );
}
