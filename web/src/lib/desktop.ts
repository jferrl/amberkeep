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

  const bound = (window as { go?: { main?: { Picker?: unknown } } }).go?.main?.Picker;
  if (bound === undefined || bound === null) return undefined;

  // Checked rather than asserted: this comes from outside the bundle, and a version
  // of the window that binds something different should leave the text field working
  // rather than throw on the first click.
  const maybe = bound as Partial<Bridge>;
  if (typeof maybe.ChooseFolder !== "function" || typeof maybe.ChooseFile !== "function") {
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
