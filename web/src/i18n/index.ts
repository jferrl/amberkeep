import { createContext, useContext } from "react";

import en from "./en.json";
import es from "./es.json";

/** The catalogue for one language. English is the shape every other must match. */
export type Catalogue = typeof en;
export type Phrase = keyof Catalogue;

/**
 * The languages this build speaks.
 *
 * Spanish is here from the start rather than added later because the archives this
 * program was written for are Spanish, and a tool that only speaks English to
 * somebody reading their own history in Spanish is a tool that feels borrowed.
 */
export const catalogues = { en, es } satisfies Record<string, Catalogue>;
export type Language = keyof typeof catalogues;

/** languageOf picks a catalogue from what the browser says it prefers. */
export function languageOf(preferred: readonly string[]): Language {
  for (const tag of preferred) {
    const base = tag.toLowerCase().split("-")[0];
    if (base === "es") return "es";
    if (base === "en") return "en";
  }
  return "en";
}

/** Translate turns a phrase into words, filling in whatever it names. */
export type Translate = (phrase: Phrase, values?: Record<string, string | number>) => string;

/**
 * translator returns the function components use.
 *
 * A phrase that names something is written with the name in braces rather than by
 * joining strings, because the order of the pieces is not the same in every
 * language and joining them fixes the English order everywhere.
 */
export function translator(language: Language): Translate {
  const catalogue = catalogues[language];
  return (phrase, values) => {
    const template = catalogue[phrase];
    if (values === undefined) return template;
    return template.replace(/\{(\w+)\}/g, (whole, name: string) => {
      const value = values[name];
      return value === undefined ? whole : String(value);
    });
  };
}

const LanguageContext = createContext<Translate>(translator("en"));

export const LanguageProvider = LanguageContext.Provider;

/** useT returns the translator for the language in force. */
export function useT(): Translate {
  return useContext(LanguageContext);
}
