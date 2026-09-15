import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Thanks } from "@/components/Thanks";
import { LanguageProvider, translator } from "@/i18n";

/**
 * The one line this program has about money. What it must never do is appear when
 * there is nowhere to point at, because a program that sends somebody to a page that
 * does not exist has spent the only goodwill the line was ever going to earn.
 */
describe("saying thanks", () => {
  const shown = (where: string | undefined, language: "en" | "es" = "en") =>
    render(
      <LanguageProvider value={translator(language)}>
        <Thanks where={where} />
      </LanguageProvider>,
    );

  it("says nothing at all without somewhere to point at", () => {
    const { container } = shown(undefined);
    expect(container).toBeEmptyDOMElement();
  });

  it("says nothing for an address that is empty", () => {
    const { container } = shown("");
    expect(container).toBeEmptyDOMElement();
  });

  it("asks once, quietly, in the reader's own language", () => {
    shown("ko-fi.com/amberkeep");
    expect(screen.getByText(/buy me a coffee at ko-fi.com\/amberkeep/)).toBeVisible();
    // Voluntary is said out loud rather than implied.
    expect(screen.getByText(/voluntary/)).toBeVisible();
  });

  it("says it in Spanish to a Spanish reader", () => {
    shown("ko-fi.com/amberkeep", "es");
    expect(screen.getByText(/invitarme a un café en ko-fi.com\/amberkeep/)).toBeVisible();
  });
});
