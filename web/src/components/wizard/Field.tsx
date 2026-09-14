import { useId, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useT } from "@/i18n";
import { choose, inAWindow, type Picking } from "@/lib/desktop";

/**
 * One thing to fill in, with its name attached and its explanation beside it.
 *
 * The label is a real label rather than a placeholder. A placeholder disappears the
 * moment somebody starts typing, which is exactly when a person who is upset needs
 * to check they are filling in the right box, and a screen reader is left with a
 * field called nothing.
 */
export function Field({
  label,
  hint,
  wrong,
  value,
  onChange,
  secret,
  autoComplete,
  choosing,
}: {
  label: string;
  hint?: string | undefined;
  /** What is wrong with what is in the box, shown and announced when there is.  */
  wrong?: string | undefined;
  value: string;
  onChange: (value: string) => void;
  /**
   * True for the decryption key, which is the only secret this program ever handles.
   *
   * It makes the box a password field: not to defend against anything technical, but
   * because the key is often read out loud off a phone in a room with other people
   * in it, and because a browser offering to remember it is the beginning of it
   * being kept.
   */
  secret?: boolean | undefined;
  autoComplete?: string | undefined;
  /**
   * What the operating system's own picker should look for, when there is one.
   *
   * Only Amberkeep's own window can open a picker; a browser cannot, and must not
   * be shown a button that does nothing. So this is an offer rather than an
   * instruction: it is taken up in the window and ignored everywhere else, and the
   * field on its own is always enough.
   */
  choosing?: { what: Picking; named: string } | undefined;
}) {
  const t = useT();
  const box = useId();
  const note = useId();
  const problem = useId();

  const describedBy = [hint === undefined ? "" : note, wrong === undefined ? "" : problem]
    .filter((id) => id !== "")
    .join(" ");

  // Decided once, when the field is first drawn. Whether this page is in a window
  // cannot change while somebody is looking at it.
  const [picker] = useState(() => (choosing === undefined ? false : inAWindow()));
  const [opening, setOpening] = useState(false);

  const open = () => {
    if (choosing === undefined) return;
    setOpening(true);
    void choose(choosing.what, t("chooseNamed", { what: choosing.named })).then(
      (chosen) => {
        setOpening(false);
        // Nothing chosen means somebody changed their mind, and what they had
        // typed is theirs to keep.
        if (chosen !== "") onChange(chosen);
      },
      () => {
        setOpening(false);
      },
    );
  };

  const box_ = (
    <Input
      id={box}
      type={secret === true ? "password" : "text"}
      value={value}
      spellCheck={false}
      autoCapitalize="off"
      autoCorrect="off"
      autoComplete={autoComplete ?? "off"}
      aria-invalid={wrong !== undefined}
      aria-describedby={describedBy === "" ? undefined : describedBy}
      onChange={(event) => {
        onChange(event.target.value);
      }}
    />
  );

  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={box} className="text-sm font-medium">
        {label}
      </label>
      {picker && choosing !== undefined ? (
        <div className="flex items-start gap-2">
          {box_}
          <Button
            disabled={opening}
            // "Choose…" three times on one screen tells a screen reader nothing.
            // What is read out is which of them this is.
            aria-label={t("chooseNamed", { what: choosing.named })}
            className="shrink-0"
            onClick={open}
          >
            {t("chooseAction")}
          </Button>
        </div>
      ) : (
        box_
      )}
      {hint !== undefined && (
        <p id={note} className="m-0 text-sm text-[var(--color-muted)]">
          {hint}
        </p>
      )}
      {wrong !== undefined && (
        <p id={problem} role="alert" className="m-0 text-sm font-medium text-[var(--color-accent)]">
          {wrong}
        </p>
      )}
    </div>
  );
}
