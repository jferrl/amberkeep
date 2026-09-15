import type { VariantProps } from "class-variance-authority";
import type { ButtonHTMLAttributes } from "react";

import { styles } from "@/components/ui/control";
import { cn } from "@/lib/utils";

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
