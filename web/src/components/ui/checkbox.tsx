import * as RadixCheckbox from "@radix-ui/react-checkbox";
import { Check } from "lucide-react";
import type { ComponentProps } from "react";

import { cn } from "@/lib/utils";

/**
 * A checkbox that looks the same on both systems this ships to.
 *
 * The native one does not. `accent-color` normalises the fill and nothing else: the
 * box is a different size and a different shape on macOS and Windows, and the two
 * places this program uses one are screens where somebody is deciding what happens
 * to their whole history. A control that looks improvised there is a control people
 * hesitate over.
 *
 * Radix underneath, because the accessibility of a checkbox is the whole reason not
 * to draw one out of divs: this keeps the role, the checked state, the space key and
 * the label association that a styled div would have to reimplement and would get
 * subtly wrong.
 */
export function Checkbox({
  className,
  ...props
}: ComponentProps<typeof RadixCheckbox.Root>) {
  return (
    <RadixCheckbox.Root
      className={cn(
        "peer size-4 shrink-0 rounded-[4px] border border-[var(--color-edge)] bg-[var(--color-bg)]",
        "transition-colors duration-150 hover:border-[var(--color-muted)]",
        "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--color-accent)]",
        "disabled:cursor-not-allowed disabled:opacity-45",
        "data-[state=checked]:border-[var(--color-accent)] data-[state=checked]:bg-[var(--color-accent)]",
        className,
      )}
      {...props}
    >
      <RadixCheckbox.Indicator className="flex items-center justify-center text-[var(--color-bg)]">
        <Check className="size-3" strokeWidth={3} />
      </RadixCheckbox.Indicator>
    </RadixCheckbox.Root>
  );
}
