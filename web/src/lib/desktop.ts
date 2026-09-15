/**
 * The window this page is sometimes running inside.
 *
 * Amberkeep is one program with two front doors. In a browser it is served over
 * loopback and every path has to be typed; in its own window the same pages can ask
 * the operating system for a file picker. This module is the whole of the difference,
 * and it is a feature test rather than a build flag: one set of pages, one bundle,
 * and nothing in the wizard that knows which it is running in.
 *
 * Everything here answers "no" when the window is absent, so a browser gets the text
 * field it always had rather than a button that does nothing.
 */

/** What is being looked for, which decides what the picker offers to show. */
export type Picking = "folder" | "database" | "encrypted" | "contacts";

/**
 * What the desktop build binds into the page.
 *
 * Declared rather than imported. Wails generates bindings at build time and this
 * page is built once, for both front doors, so the shape is written down here and
 * checked at run time instead.
 */
interface Bridge {
  ChooseFolder: (title: string) => Promise<string>;
  ChooseFile: (title: string, kind: string) => Promise<string>;
}

function bridge(): Bridge | undefined {
  if (typeof window === "undefined") return undefined;

  const bound = (window as { go?: { main?: { Picker?: unknown } } }).go?.main
    ?.Picker;
  if (bound === undefined || bound === null) return undefined;

  // Checked rather than asserted: this comes from outside the bundle, and a version
  // of the window that binds something different should leave the text field working
  // rather than throw on the first click.
  const maybe = bound as Partial<Bridge>;
  if (
    typeof maybe.ChooseFolder !== "function" ||
    typeof maybe.ChooseFile !== "function"
  ) {
    return undefined;
  }
  return maybe as Bridge;
}

/** inAWindow reports whether the operating system's own picker can be opened. */
export function inAWindow(): boolean {
  return bridge() !== undefined;
}

/**
 * choose opens the picker and answers with what was chosen.
 *
 * An empty string means somebody changed their mind. That is not a failure and is
 * not reported as one — the field keeps whatever was already in it — which is also
 * what happens if the window refuses, because a picker that will not open is not a
 * reason to lose what somebody typed.
 */
export async function choose(what: Picking, title: string): Promise<string> {
  const window = bridge();
  if (window === undefined) return "";

  try {
    return what === "folder"
      ? await window.ChooseFolder(title)
      : await window.ChooseFile(title, what);
  } catch {
    return "";
  }
}

/** What a menu item can ask the page to do. */
export type Asked = "keep" | "close";

/**
 * asking listens for the window's menu, and reports how to stop.
 *
 * The menu cannot do the work itself: the window and the browser run the same
 * screens, and a menu that opened its own would be a second implementation of one
 * that already exists. So it says what was chosen and the page decides what that
 * means on whichever screen it is showing.
 *
 * In a browser there is no menu and this does nothing, which is why it reports a way
 * to stop rather than throwing: the caller unsubscribes on the way out either way.
 */
export function asking(hear: (what: Asked) => void): () => void {
  if (typeof window === "undefined") return () => undefined;

  const wails = (
    window as { runtime?: { EventsOn?: unknown; EventsOff?: unknown } }
  ).runtime;
  if (typeof wails?.EventsOn !== "function") return () => undefined;

  const on = wails.EventsOn as (
    name: string,
    handler: (...args: unknown[]) => void,
  ) => void;
  on("amberkeep:menu", (...args: unknown[]) => {
    const what = args[0];
    if (what === "keep" || what === "close") hear(what);
  });

  return () => {
    const off = wails.EventsOff;
    if (typeof off === "function")
      (off as (name: string) => void)("amberkeep:menu");
  };
}

/**
 * outside opens an address in the person's own browser.
 *
 * Always through here, never a plain link. In a window there is no address bar and
 * no way back: a link followed in place would replace the program with a web page
 * and strand somebody who only wanted to look at something. Wails has a call for
 * handing an address to the operating system, and when it is absent — which is to
 * say in a browser — a new tab is the same thing by other means.
 *
 * This is not the program fetching anything. Nothing is requested here; an address
 * is handed to something else, because somebody clicked it.
 */
export function outside(address: string): void {
  const wails = (
    window as {
      runtime?: { BrowserOpenURL?: (url: string) => void };
    }
  ).runtime;

  if (typeof wails?.BrowserOpenURL === "function") {
    wails.BrowserOpenURL(address);
    return;
  }
  window.open(address, "_blank", "noopener,noreferrer");
}
