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
  "inline-flex items-center justify-center gap-2 rounded-lg text-sm font-medium " +
    "transition-colors disabled:pointer-events-none disabled:opacity-50 " +
    "focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--color-accent)]",
  {
    variants: {
      variant: {
        default:
          "border border-[var(--color-line)] bg-[var(--color-panel)] hover:border-[var(--color-accent)]",
        ghost: "hover:bg-[var(--color-panel)]",
        quiet: "text-[var(--color-muted)] hover:text-[var(--color-ink)]",
      },
      size: {
        default: "h-9 px-3",
        sm: "h-8 px-2 text-xs",
        icon: "size-9",
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
