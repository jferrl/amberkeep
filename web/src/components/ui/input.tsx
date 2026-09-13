import type { InputHTMLAttributes } from "react";

import { cn } from "@/lib/utils";

export type InputProps = InputHTMLAttributes<HTMLInputElement>;

export function Input({ className, ...props }: InputProps) {
  return (
    <input
      className={cn(
        "h-9 w-full min-w-0 rounded-lg border border-[var(--color-line)] bg-[var(--color-paper)]",
        "px-2.5 text-sm text-[var(--color-ink)] placeholder:text-[var(--color-muted)]",
        "focus-visible:outline-2 focus-visible:outline-offset-[-1px] focus-visible:outline-[var(--color-accent)]",
        className,
      )}
      {...props}
    />
  );
}
