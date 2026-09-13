import type { Language } from "@/i18n";

/**
 * Dates and counts, rendered in the reader's own language and time zone.
 *
 * The server sends every instant in universal time and says which zone the archive
 * was read in; the browser is what turns that into a time of day. Formatters are
 * built once per language because constructing one is expensive and a conversation
 * renders thousands of timestamps.
 */
const formatters = new Map<string, Intl.DateTimeFormat>();

function formatter(language: Language, options: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const key = language + JSON.stringify(options);
  let found = formatters.get(key);
  if (found === undefined) {
    found = new Intl.DateTimeFormat(language, options);
    formatters.set(key, found);
  }
  return found;
}

/** timeOfDay is what a message shows beside its sender. */
export function timeOfDay(iso: string, language: Language): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return "";
  return formatter(language, { hour: "2-digit", minute: "2-digit" }).format(at);
}

/** dayOf is the separator a message belongs under, and the key that groups them. */
export function dayOf(iso: string, language: Language): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return "";
  return formatter(language, {
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
  }).format(at);
}

/** shortDate is what a conversation shows in the list, and a search result. */
export function shortDate(iso: string | undefined, language: Language): string {
  if (iso === undefined) return "";
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return "";
  return formatter(language, { day: "numeric", month: "short", year: "numeric" }).format(at);
}

/** count renders a number the way the reader's language groups digits. */
export function count(n: number, language: Language): string {
  return new Intl.NumberFormat(language).format(n);
}
