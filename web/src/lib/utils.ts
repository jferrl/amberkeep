import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

/** cn joins class names and resolves the conflicts Tailwind would otherwise leave. */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
