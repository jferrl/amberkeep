import { useId } from "react";

import { Input } from "@/components/ui/input";

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
}) {
  const box = useId();
  const note = useId();
  const problem = useId();

  const describedBy = [hint === undefined ? "" : note, wrong === undefined ? "" : problem]
    .filter((id) => id !== "")
    .join(" ");

  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={box} className="text-sm font-medium">
        {label}
      </label>
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
