import { describe, expect, it } from "vitest";

import en from "./en.json";
import es from "./es.json";
import { languageOf, translator } from "./index";

describe("choosing a language", () => {
  const cases = [
    { name: "Spanish, plainly", preferred: ["es"], want: "es" },
    { name: "a Spanish region", preferred: ["es-ES"], want: "es" },
    { name: "Latin American Spanish", preferred: ["es-419", "en"], want: "es" },
    { name: "English", preferred: ["en-GB"], want: "en" },
    {
      name: "the first one this build speaks",
      preferred: ["fr", "es", "en"],
      want: "es",
    },
    { name: "nothing this build speaks", preferred: ["fr", "de"], want: "en" },
    { name: "nothing at all", preferred: [], want: "en" },
    { name: "case does not matter", preferred: ["ES-es"], want: "es" },
  ] as const;

  it.each(cases)("$name", ({ preferred, want }) => {
    expect(languageOf(preferred)).toBe(want);
  });
});

/**
 * Every phrase must exist in every language. A missing one is not a typing error,
 * because the catalogues are JSON: it shows up as the word "undefined" on screen in
 * front of somebody reading their own messages.
 */
describe("the catalogues", () => {
  it("say the same things", () => {
    expect(Object.keys(es).sort()).toEqual(Object.keys(en).sort());
  });

  it.each(Object.keys(es))("has Spanish for %s", (phrase) => {
    expect(es[phrase as keyof typeof es]).not.toBe("");
  });

  /**
   * A phrase that names a number does so with a placeholder rather than by joining
   * strings, because the pieces do not come in the same order in every language and
   * joining them fixes the English order everywhere.
   */
  it("uses the same placeholders in both languages", () => {
    const placeholders = (text: string) =>
      (text.match(/\{\w+\}/g) ?? []).sort();
    for (const phrase of Object.keys(en) as (keyof typeof en)[]) {
      expect(placeholders(es[phrase])).toEqual(placeholders(en[phrase]));
    }
  });
});

describe("filling in a phrase", () => {
  const t = translator("en");

  const cases = [
    {
      name: "a plain phrase",
      phrase: "archive",
      values: undefined,
      want: "Archive",
    },
    {
      name: "one that names a number",
      phrase: "matches",
      values: { count: 12 },
      want: "12 matches",
    },
    {
      name: "a number already written out",
      phrase: "matches",
      values: { count: "1,311" },
      want: "1,311 matches",
    },
    {
      name: "a name nobody supplied is left alone rather than blanked",
      phrase: "matches",
      values: {},
      want: "{count} matches",
    },
  ] as const;

  it.each(cases)("$name", ({ phrase, values, want }) => {
    expect(t(phrase, values)).toBe(want);
  });

  it("speaks Spanish when asked to", () => {
    expect(translator("es")("conversations")).toBe("conversaciones");
  });
});
