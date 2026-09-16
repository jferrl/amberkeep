import { useId } from "react";

import { Checkbox } from "@/components/ui/checkbox";

/**
 * One thing that is off unless somebody turns it on, with the reason beside it.
 *
 * The reason is not optional. Every one of these costs something a person can feel —
 * an hour of copying, six gigabytes of disk, an archive of housekeeping notices
 * nobody wrote — and a tick box whose label is three words and whose consequence is
 * an hour is a tick box that gets ticked by accident.
 */
export function Pick({
  label,
  hint,
  on,
  onChange,
}: {
  label: string;
  hint: string;
  on: boolean;
  onChange: (on: boolean) => void;
}) {
  const box = useId();
  const note = useId();

  return (
    <div className="flex items-start gap-3 py-1">
      <Checkbox
        id={box}
        checked={on}
        aria-describedby={note}
        className="mt-0.5"
        onCheckedChange={(state) => {
          onChange(state === true);
        }}
      />
      <div>
        <label
          htmlFor={box}
          className="block cursor-pointer text-sm font-medium"
        >
          {label}
        </label>
        <p
          id={note}
          className="m-0 mt-0.5 text-[0.8125rem] text-[var(--color-muted)]"
        >
          {hint}
        </p>
      </div>
    </div>
  );
}
