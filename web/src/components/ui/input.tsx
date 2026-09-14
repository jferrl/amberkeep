import type { InputHTMLAttributes } from "react";

import { cn } from "@/lib/utils";

export type InputProps = InputHTMLAttributes<HTMLInputElement>;

export function Input({ className, ...props }: InputProps) {
  return (
    <input
      className={cn(
        "h-8 w-full min-w-0 rounded-md border border-[var(--color-line)] bg-[var(--color-bg)]",
        "px-2.5 text-[0.8125rem] text-[var(--color-ink)] placeholder:text-[var(--color-muted)]",
        "transition-colors duration-150 hover:border-[var(--color-muted)]",
        "focus-visible:border-[var(--color-accent)] focus-visible:outline-2 " +
          "focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--color-accent)]",
        className,
      )}
      {...props}
    />
  );
}
