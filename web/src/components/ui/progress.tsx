import * as RadixProgress from "@radix-ui/react-progress";
import type { ComponentProps } from "react";

import { cn } from "@/lib/utils";

/**
 * How far along something is, said properly.
 *
 * This replaces a row of filled spans wearing `role="img"` and an aria-label, which
 * was a fudge: a screen reader was handed a picture with a caption rather than a
 * value it could report. Radix gives the real thing — role="progressbar" with a now
 * and a max — for the same drawing.
 *
 * Still segmented rather than a smooth bar, because the wizard's steps are countable
 * and a person is entitled to see how many are left. Nothing here estimates: every
 * step this appears on is one somebody completes by pressing something.
 */
export function Progress({
  value,
  max,
  className,
  ...props
}: ComponentProps<typeof RadixProgress.Root> & { value: number; max: number }) {
  return (
    <RadixProgress.Root
      value={value}
      max={max}
      className={cn("flex h-1 w-24 gap-0.5 overflow-hidden rounded-full", className)}
      {...props}
    >
      {Array.from({ length: max }, (_, at) => (
        <span
          key={at}
          className={`h-full flex-1 rounded-full ${
            at < value ? "bg-[var(--color-accent)]" : "bg-[var(--color-edge)]"
          }`}
        />
      ))}
    </RadixProgress.Root>
  );
}
