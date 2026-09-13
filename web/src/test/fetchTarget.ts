/**
 * Where a call to fetch was going.
 *
 * fetch takes three different things as its first argument, and each of them says
 * where it is going differently. Converting one to a string without asking which it
 * is gives "[object Object]" for two of the three, so a test comparing that to an
 * expected address passes for the wrong reason. This is the one place that asks.
 */
export function fetchTarget(input: unknown): string {
  if (typeof input === "string") return input;
  if (input instanceof URL) return input.href;
  if (input instanceof Request) return input.url;
  return "";
}
