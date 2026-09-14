import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Field } from "@/components/wizard/Field";
import { asking } from "@/lib/desktop";
import { render } from "@/test/render";

/**
 * Asking for a path, in the two places this page runs.
 *
 * Amberkeep is one program with two front doors, and this field is where the
 * difference shows: in its own window it can open the operating system's picker, and
 * in a browser it cannot. What is tested here is mostly that the browser is not made
 * worse by the window existing — the text field has to keep working exactly as it
 * did, with no button that does nothing.
 */

/** What the desktop build binds into the page, and what it answers this time. */
function windowAround(answers: { folder?: string; file?: string } = {}) {
  const ChooseFolder = vi.fn(() => Promise.resolve(answers.folder ?? ""));
  const ChooseFile = vi.fn(() => Promise.resolve(answers.file ?? ""));
  vi.stubGlobal("go", { main: { Picker: { ChooseFolder, ChooseFile } } });
  return { ChooseFolder, ChooseFile };
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("in a browser, where there is no picker", () => {
  it("offers no button, because one that did nothing would be worse than none", async () => {
    render(
      <Field
        label="The iPhone backup folder"
        value=""
        onChange={() => undefined}
        choosing={{ what: "folder", named: "the iPhone backup folder" }}
      />,
    );

    expect(
      await screen.findByLabelText("The iPhone backup folder"),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("still takes a typed path, which is the only way in", async () => {
    const user = userEvent.setup();
    const typed: string[] = [];

    render(
      <Field
        label="The decrypted Android database"
        value=""
        onChange={(v) => typed.push(v)}
        choosing={{ what: "database", named: "the decrypted Android database" }}
      />,
    );

    await user.type(
      await screen.findByLabelText("The decrypted Android database"),
      "/tmp/m.db",
    );
    expect(typed.join("")).toBe("/tmp/m.db");
  });
});

describe("in its own window", () => {
  it("offers the picker, and says which field it belongs to", async () => {
    windowAround();

    render(
      <Field
        label="The iPhone backup folder"
        value=""
        onChange={() => undefined}
        choosing={{ what: "folder", named: "the iPhone backup folder" }}
      />,
    );

    // "Choose…" three times on one screen tells a screen reader nothing, so what is
    // read out is which of them this is.
    const button = await screen.findByRole("button", {
      name: "Choose the iPhone backup folder",
    });
    expect(button).toBeVisible();
    expect(button).toHaveTextContent("Choose…");
  });

  it("puts what was chosen into the field", async () => {
    const user = userEvent.setup();
    const { ChooseFolder } = windowAround({ folder: "/Backups/00008110-aaa" });
    const chosen: string[] = [];

    render(
      <Field
        label="The iPhone backup folder"
        value=""
        onChange={(v) => chosen.push(v)}
        choosing={{ what: "folder", named: "the iPhone backup folder" }}
      />,
    );

    await user.click(await screen.findByRole("button"));
    expect(chosen).toEqual(["/Backups/00008110-aaa"]);
    // The dialog is titled with the thing being looked for, not with "Choose a file".
    expect(ChooseFolder).toHaveBeenCalledWith(
      "Choose the iPhone backup folder",
    );
  });

  /**
   * Changing your mind is not a failure, and must not cost you what you had. Somebody
   * who typed a path, opened the picker to check, and closed it again should find
   * their path still there.
   */
  it("keeps what was already typed when nobody chooses anything", async () => {
    const user = userEvent.setup();
    windowAround({ folder: "" });
    const changed: string[] = [];

    render(
      <Field
        label="The iPhone backup folder"
        value="/Backups/typed-by-hand"
        onChange={(v) => changed.push(v)}
        choosing={{ what: "folder", named: "the iPhone backup folder" }}
      />,
    );

    await user.click(await screen.findByRole("button"));
    expect(changed).toEqual([]);
    expect(screen.getByLabelText("The iPhone backup folder")).toHaveValue(
      "/Backups/typed-by-hand",
    );
  });

  /** A file and a folder are different questions, and get asked differently. */
  it("asks for a file when the thing wanted is a file", async () => {
    const user = userEvent.setup();
    const { ChooseFile, ChooseFolder } = windowAround({
      file: "/tmp/msgstore.db",
    });

    render(
      <Field
        label="The decrypted Android database"
        value=""
        onChange={() => undefined}
        choosing={{ what: "database", named: "the decrypted Android database" }}
      />,
    );

    await user.click(await screen.findByRole("button"));
    expect(ChooseFile).toHaveBeenCalledWith(
      "Choose the decrypted Android database",
      "database",
    );
    expect(ChooseFolder).not.toHaveBeenCalled();
  });
});

describe("when the window is not what this page expects", () => {
  /**
   * The page is built once for both front doors and the bindings come from outside
   * the bundle, so a window that binds something different has to leave the text
   * field working rather than throw on the first click.
   */
  const strange: { name: string; bound: unknown }[] = [
    { name: "nothing bound at all", bound: undefined },
    { name: "bound to null", bound: { main: { Picker: null } } },
    {
      name: "bound to something that is not a picker",
      bound: { main: { Picker: {} } },
    },
    {
      name: "bound with only half of it",
      bound: { main: { Picker: { ChooseFolder: () => "" } } },
    },
  ];

  it.each(strange)("$name leaves the field alone", async ({ bound }) => {
    if (bound !== undefined) vi.stubGlobal("go", bound);

    render(
      <Field
        label="The iPhone backup folder"
        value=""
        onChange={() => undefined}
        choosing={{ what: "folder", named: "the iPhone backup folder" }}
      />,
    );

    expect(
      await screen.findByLabelText("The iPhone backup folder"),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("does not lose what was typed when the picker itself fails", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("go", {
      main: {
        Picker: {
          ChooseFolder: () => Promise.reject(new Error("no window")),
          ChooseFile: () => Promise.resolve(""),
        },
      },
    });
    const changed: string[] = [];

    render(
      <Field
        label="The iPhone backup folder"
        value="/Backups/typed-by-hand"
        onChange={(v) => changed.push(v)}
        choosing={{ what: "folder", named: "the iPhone backup folder" }}
      />,
    );

    await user.click(await screen.findByRole("button"));
    expect(changed).toEqual([]);
    expect(screen.getByLabelText("The iPhone backup folder")).toHaveValue(
      "/Backups/typed-by-hand",
    );
  });
});

describe("the window's menu", () => {
  /**
   * The menu bar is not decoration. Without an Edit menu a macOS webview has no
   * standard editing selectors and paste does not work, and the one thing this
   * program asks anybody to paste is a sixty-four character key read off a phone.
   * That part is the window's; this is the part the page has to hold up.
   */
  it("does nothing, and unsubscribes from nothing, in a browser", () => {
    const heard: string[] = [];
    const stop = asking((what) => heard.push(what));

    expect(heard).toEqual([]);
    // The caller unsubscribes on the way out whether or not there was a menu.
    expect(() => {
      stop();
    }).not.toThrow();
  });

  it("hears what a menu item asked for", () => {
    const handlers: Record<string, (...args: unknown[]) => void> = {};
    vi.stubGlobal("runtime", {
      EventsOn: (name: string, handler: (...args: unknown[]) => void) => {
        handlers[name] = handler;
      },
      EventsOff: () => undefined,
    });

    const heard: string[] = [];
    asking((what) => heard.push(what));

    handlers["amberkeep:menu"]?.("keep");
    handlers["amberkeep:menu"]?.("close");
    expect(heard).toEqual(["keep", "close"]);
  });

  /** The event carries whatever Go sent. Anything unrecognised is not acted on. */
  it("ignores anything it does not recognise", () => {
    const handlers: Record<string, (...args: unknown[]) => void> = {};
    vi.stubGlobal("runtime", {
      EventsOn: (name: string, handler: (...args: unknown[]) => void) => {
        handlers[name] = handler;
      },
      EventsOff: () => undefined,
    });

    const heard: string[] = [];
    asking((what) => heard.push(what));

    handlers["amberkeep:menu"]?.("delete-everything");
    handlers["amberkeep:menu"]?.(undefined);
    handlers["amberkeep:menu"]?.();
    expect(heard).toEqual([]);
  });
});

describe("the one secret this program handles", () => {
  /**
   * Masked by default is right: the key is usually read aloud off a phone in a room
   * with other people in it, and a browser offering to remember it is the beginning
   * of it being kept. Unreadable forever is not right: it is sixty-four characters,
   * and nobody types those correctly without being able to look.
   */
  it("starts hidden", async () => {
    render(<Field label="The 64-digit key" value="abc" onChange={() => undefined} secret />);

    const box = await screen.findByLabelText("The 64-digit key");
    expect(box).toHaveAttribute("type", "password");
  });

  it("can be checked, and hidden again", async () => {
    const user = userEvent.setup();
    render(<Field label="The 64-digit key" value="abc" onChange={() => undefined} secret />);

    await user.click(await screen.findByRole("button", { name: "Show" }));
    expect(screen.getByLabelText("The 64-digit key")).toHaveAttribute("type", "text");

    await user.click(screen.getByRole("button", { name: "Hide" }));
    expect(screen.getByLabelText("The 64-digit key")).toHaveAttribute("type", "password");
  });

  it("says which state it is in, for anybody not looking at the letters", async () => {
    const user = userEvent.setup();
    render(<Field label="The 64-digit key" value="abc" onChange={() => undefined} secret />);

    const button = await screen.findByRole("button");
    expect(button).toHaveAttribute("aria-pressed", "false");
    await user.click(button);
    expect(screen.getByRole("button")).toHaveAttribute("aria-pressed", "true");
  });

  /** A field that is not a secret is not given a control that suggests it is. */
  it("is offered on nothing else", async () => {
    render(<Field label="The full path to the file" value="" onChange={() => undefined} />);

    expect(await screen.findByLabelText("The full path to the file")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show" })).not.toBeInTheDocument();
  });
});
