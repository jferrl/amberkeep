import { cva, type VariantProps } from "class-variance-authority";
import type { ButtonHTMLAttributes } from "react";

import { cn } from "@/lib/utils";

/**
 * The components under ui/ are copied into this repository rather than imported
 * from a package. That is deliberate: nothing is fetched at build time or at run
 * time that is not in the repository, and a component can be adapted to this
 * archive's vocabulary instead of the archive being bent to a library's.
 */
const styles = cva(
  // min-h rather than h: Spanish is longer than English almost everywhere, and a
  // fixed height meant "Cerrar este archivo" wrapped to three lines inside a
  // twenty-eight pixel box and spilled over its own border.
  "inline-flex items-center justify-center gap-2 rounded-md text-[0.8125rem] font-medium " +
    "text-center leading-tight " +
    "transition-[background-color,border-color,color,opacity] duration-150 " +
    "disabled:pointer-events-none disabled:opacity-45 " +
    "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--color-accent)]",
  {
    variants: {
      variant: {
        /**
         * The one thing this screen is for.
         *
         * Exactly one of these per screen. When every button is drawn the same
         * weight, somebody who is anxious and skim-reading has to read all of them
         * to find the way forward — which, on the screens in this program, is how
         * you end up clicking the one that writes to your phone.
         */
        primary:
          "bg-[var(--color-ink)] text-[var(--color-bg)] hover:opacity-90 active:opacity-100",
        /**
         * The step that writes.
         *
         * Exactly one button in this program ends with a file being produced that
         * somebody will restore onto a phone. It was drawn as `primary`, which is
         * also what "Next" is drawn as, so the most consequential control looked
         * exactly like the most harmless one.
         */
        commit:
          "bg-[var(--color-alarm)] text-[var(--color-bg)] hover:opacity-90 active:opacity-100",
        default:
          "border border-[var(--color-edge)] bg-[var(--color-bg)] hover:bg-[var(--color-surface)] " +
          "hover:border-[var(--color-muted)]",
        ghost: "hover:bg-[var(--color-surface)]",
        quiet:
          "text-[var(--color-muted)] underline decoration-[var(--color-edge)] underline-offset-4 " +
          "hover:text-[var(--color-ink)] hover:decoration-[var(--color-ink)]",
      },
      size: {
        default: "min-h-8 px-3 py-1.5",
        sm: "min-h-7 px-2 py-1 text-xs",
        icon: "size-8",
      },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
);

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> &
  VariantProps<typeof styles>;

export function Button({
  className,
  variant,
  size,
  type,
  ...props
}: ButtonProps) {
  return (
    <button
      type={type ?? "button"}
      className={cn(styles({ variant, size }), className)}
      {...props}
    />
  );
}
