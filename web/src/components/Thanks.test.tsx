import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Thanks } from "@/components/Thanks";
import { LanguageProvider, translator } from "@/i18n";

/**
 * The one line this program has about money.
 *
 * Two things it must never do: appear when there is nowhere to point at, because a
 * program that sends somebody to a page which does not exist has spent the only
 * goodwill it was going to earn; and be followed in place, because in a window there
 * is no address bar and no way back from a web page.
 */
describe("saying thanks", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

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

  it("asks once, quietly, and shows the address the way anybody would write it", () => {
    shown("https://ko-fi.com/jferrl");

    expect(screen.getByText(/buy me a coffee/)).toBeVisible();
    // Voluntary is said out loud rather than implied.
    expect(screen.getByText(/voluntary/)).toBeVisible();

    const link = screen.getByRole("link", { name: "ko-fi.com/jferrl" });
    expect(link).toHaveAttribute("href", "https://ko-fi.com/jferrl");
  });

  it("says it in Spanish to a Spanish reader", () => {
    shown("https://ko-fi.com/jferrl", "es");
    expect(screen.getByText(/invitarme a un café/)).toBeVisible();
  });

  /** In a window, following it in place would replace the program with a web page. */
  it("hands the address to the window rather than following it", async () => {
    const opened = vi.fn();
    vi.stubGlobal("runtime", { BrowserOpenURL: opened });

    shown("https://ko-fi.com/jferrl");
    await userEvent.click(screen.getByRole("link", { name: "ko-fi.com/jferrl" }));

    expect(opened).toHaveBeenCalledWith("https://ko-fi.com/jferrl");
  });

  /** In a browser, the same click is a new tab and the archive stays where it was. */
  it("opens a new tab when there is no window to ask", async () => {
    const opened = vi.fn();
    vi.stubGlobal("open", opened);

    shown("https://ko-fi.com/jferrl");
    await userEvent.click(screen.getByRole("link", { name: "ko-fi.com/jferrl" }));

    expect(opened).toHaveBeenCalledWith(
      "https://ko-fi.com/jferrl",
      "_blank",
      "noopener,noreferrer",
    );
  });
});
