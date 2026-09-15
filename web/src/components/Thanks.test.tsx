import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Thanks } from "@/components/Thanks";
import { LanguageProvider, translator } from "@/i18n";
import { ThanksProvider } from "@/lib/thanks";

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
        <ThanksProvider value={where}>
          <Thanks />
        </ThanksProvider>
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

  it("is a control somebody can see, pointing where it says", () => {
    shown("https://ko-fi.com/jferrl");

    const link = screen.getByRole("link", { name: /buy me a coffee/i });
    expect(link).toHaveAttribute("href", "https://ko-fi.com/jferrl");
  });

  /** On the screens that have room for it, the sentence is said as well. */
  it("says why, where there is room to", () => {
    render(
      <LanguageProvider value={translator("en")}>
        <ThanksProvider value="https://ko-fi.com/jferrl">
          <Thanks full />
        </ThanksProvider>
      </LanguageProvider>,
    );

    expect(screen.getByText(/voluntary/)).toBeVisible();
  });

  /**
   * Ko-fi publishes a button as a script tag pointing at their servers. Embedding it
   * would be this page fetching something from somewhere else, which is the one thing
   * it has never done — so the button is drawn here and nothing is loaded from
   * anybody. This is the test that says so out loud; the browser test that watches
   * every request is what would catch it.
   */
  it("loads nothing from anywhere to draw itself", () => {
    const { container } = shown("https://ko-fi.com/jferrl");

    expect(container.querySelector("script")).toBeNull();
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("iframe")).toBeNull();
  });

  it("says it in Spanish to a Spanish reader", () => {
    shown("https://ko-fi.com/jferrl", "es");
    expect(screen.getByRole("link", { name: /Invítame a un café/ })).toBeVisible();
  });

  /** In a window, following it in place would replace the program with a web page. */
  it("hands the address to the window rather than following it", async () => {
    const opened = vi.fn();
    vi.stubGlobal("runtime", { BrowserOpenURL: opened });

    shown("https://ko-fi.com/jferrl");
    await userEvent.click(screen.getByRole("link", { name: /coffee/i }));

    expect(opened).toHaveBeenCalledWith("https://ko-fi.com/jferrl");
  });

  /** In a browser, the same click is a new tab and the archive stays where it was. */
  it("opens a new tab when there is no window to ask", async () => {
    const opened = vi.fn();
    vi.stubGlobal("open", opened);

    shown("https://ko-fi.com/jferrl");
    await userEvent.click(screen.getByRole("link", { name: /coffee/i }));

    expect(opened).toHaveBeenCalledWith(
      "https://ko-fi.com/jferrl",
      "_blank",
      "noopener,noreferrer",
    );
  });
});
