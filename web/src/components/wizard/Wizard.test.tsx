import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { UserEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { App } from "@/App";
import type { BackupList, Setup } from "@/api/types";
import { fetchTarget } from "@/test/fetchTarget";
import { render } from "@/test/render";

/**
 * The wizard, against a stand-in for the server it will meet.
 *
 * The whole point of this part of the program is that somebody who cannot open a
 * terminal can still read their own messages, so what is tested here is the journey
 * rather than the components: which screen follows which, what is sent when a button
 * is pressed, and what happens when the answer is no. It talks to the stand-in at the
 * same addresses and with the same shapes the real server uses.
 */

/** Where the server says it will write what it makes, unless somebody changes it. */
const workspace = "/Users/someone/Amberkeep";

/** A key as WhatsApp shows one: 64 hexadecimal characters. */
const key = "0123456789abcdef".repeat(4);

/** One request that started work, and what it carried. */
interface Posted {
  at: string;
  body: Record<string, unknown>;
}

let fetching: ReturnType<typeof vi.fn<typeof fetch>>;
let posted: Posted[];
let states: Setup[];
// The real shape, not one written down beside it: a stand-in that can drift from the
// server is a stand-in that proves nothing.
let backups: BackupList;
let closing: Response | undefined;
let refusing: Response | undefined;
let leadsTo: Setup | undefined;

function answer(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/**
 * What the server says this time.
 *
 * The queue is drained one answer per request and then holds still, which is how a
 * stage that changes on its own — the only one that does — is written down without
 * any timing in the test.
 */
function nextState(): Setup {
  const now = states[0]!;
  if (states.length > 1) states.shift();
  return now;
}

/** Every address this page asked for, in order, which is where a secret would show up. */
function addressesAsked(): string[] {
  return fetching.mock.calls.map((call) => fetchTarget(call[0]));
}

beforeEach(() => {
  posted = [];
  states = [{ stage: "empty", workspace }];
  backups = { backups: [] };
  closing = undefined;
  refusing = undefined;
  leadsTo = undefined;

  fetching = vi.fn<typeof fetch>((input, init) => {
    const at = fetchTarget(input);

    if (init?.method === "POST") {
      const body: unknown =
        typeof init.body === "string" ? JSON.parse(init.body) : {};
      posted.push({ at, body: body as Record<string, unknown> });

      if (at.startsWith("/api/close")) {
        states = [{ stage: "empty", workspace }];
        return Promise.resolve(closing ?? answer({}));
      }

      if (refusing !== undefined) return Promise.resolve(refusing);

      const started: Setup = leadsTo ?? {
        stage: "working",
        workspace,
        step: at.startsWith("/api/decrypt") ? "decrypting" : "extracting",
        detail: "Reading the backup index.",
      };
      states = [started];
      return Promise.resolve(answer(started, 202));
    }

    if (at.startsWith("/api/state"))
      return Promise.resolve(answer(nextState()));
    if (at.startsWith("/api/backups")) return Promise.resolve(answer(backups));
    if (at.startsWith("/api/archive")) {
      return Promise.resolve(
        answer({
          title: "Archive",
          conversations: 1,
          messages: 2,
          searchable: false,
        }),
      );
    }
    if (at.startsWith("/api/chats"))
      return Promise.resolve(answer({ total: 0, chats: [] }));

    return Promise.resolve(new Response("not found", { status: 404 }));
  });

  vi.stubGlobal("fetch", fetching);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

/** chooseRoute presses one of the three things somebody can say they have. */
async function chooseRoute(user: UserEvent, name: RegExp): Promise<void> {
  await user.click(await screen.findByRole("button", { name }));
}

/** walkThePhone presses Next through the four screens that happen on the phone. */
async function walkThePhone(user: UserEvent): Promise<void> {
  for (let screens = 0; screens < 4; screens++) {
    await user.click(await screen.findByRole("button", { name: "Next" }));
  }
}

/**
 * Each route, pressed through to the request that starts the work.
 *
 * The bodies are the contract with `internal/api`. A field renamed on either side
 * turns into a 400 the person cannot act on, so the names are asserted rather than
 * the fact that something was sent.
 */
const routes: {
  name: string;
  before?: () => void;
  walk: (user: UserEvent) => Promise<void>;
  at: string;
  body: Record<string, unknown>;
}[] = [
  {
    name: "a file somebody already has is opened by its path alone",
    walk: async (user) => {
      await chooseRoute(user, /I already have a file/);
      await user.type(
        await screen.findByLabelText("The full path to the file"),
        "/tmp/msgstore.db",
      );
      await user.click(screen.getByRole("button", { name: "Open it" }));
    },
    at: "/api/open",
    body: { path: "/tmp/msgstore.db" },
  },
  {
    name: "an iPhone backup is extracted, and the workspace nobody touched is left out",
    before: () => {
      backups = {
        backups: [
          {
            path: "/Backups/00008110-aaa",
            device_name: "Ana's iPhone",
            ios_version: "17.4",
            last_backup: "2024-03-02T21:04:00Z",
            encrypted: false,
          },
        ],
      };
    },
    walk: async (user) => {
      await chooseRoute(user, /My iPhone is backed up to this computer/);
      await user.click(
        await screen.findByRole("button", { name: "Use this backup" }),
      );
    },
    at: "/api/extract",
    body: { backup: "/Backups/00008110-aaa" },
  },
  {
    name: "an Android backup is decrypted with the file and the key and nothing else",
    walk: async (user) => {
      await chooseRoute(user, /I have an Android phone/);
      await walkThePhone(user);
      await user.type(
        await screen.findByLabelText(/Where msgstore.db.crypt15 is/),
        "/tmp/msgstore.db.crypt15",
      );
      await user.type(screen.getByLabelText("The 64-digit key"), key);
      await user.click(screen.getByRole("button", { name: "Unlock the file" }));
    },
    at: "/api/decrypt",
    body: { file: "/tmp/msgstore.db.crypt15", key },
  },
];

describe("the three ways in", () => {
  it.each(routes)("$name", async ({ before, walk, at, body }) => {
    before?.();
    const user = userEvent.setup();
    render(<App language="en" />);

    await walk(user);

    await waitFor(() => {
      expect(posted).toEqual([{ at, body }]);
    });
  });

  it.each(routes)("stays on this machine: $name", async ({ before, walk }) => {
    before?.();
    const user = userEvent.setup();
    render(<App language="en" />);

    await walk(user);

    for (const at of addressesAsked())
      expect(at.startsWith("/api/")).toBe(true);
  });
});

describe("the workspace", () => {
  it("is shown before anything is written, as the server suggested it", async () => {
    backups = {
      backups: [
        { path: "/Backups/one", device_name: "Ana's iPhone", encrypted: false },
      ],
    };
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /My iPhone is backed up to this computer/);

    expect(
      await screen.findByLabelText("Where the files should go"),
    ).toHaveValue(workspace);
  });

  /**
   * Sending it back would reassert a choice nobody made. The server treats a folder
   * it is given as the new workspace, so a page that echoed the one it was shown
   * would be deciding for somebody every time they pressed a button.
   */
  it("is what the work is told to use once somebody has changed it", async () => {
    backups = {
      backups: [
        { path: "/Backups/one", device_name: "Ana's iPhone", encrypted: false },
      ],
    };
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /My iPhone is backed up to this computer/);
    const folder = await screen.findByLabelText("Where the files should go");
    await user.clear(folder);
    await user.type(folder, "/tmp/elsewhere");
    await user.click(screen.getByRole("button", { name: "Use this backup" }));

    await waitFor(() => {
      expect(posted[0]?.body).toEqual({
        backup: "/Backups/one",
        into: "/tmp/elsewhere",
      });
    });
  });
});

describe("an encrypted iPhone backup", () => {
  beforeEach(() => {
    backups = {
      backups: [
        {
          path: "/Backups/locked",
          device_name: "Ana's old iPhone",
          encrypted: true,
        },
        {
          path: "/Backups/open",
          device_name: "Ana's iPhone",
          encrypted: false,
        },
      ],
    };
  });

  /** Hiding it is how somebody concludes the backup they know exists has been lost. */
  it("is listed rather than hidden", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /My iPhone is backed up to this computer/);

    expect(
      await screen.findByRole("heading", { name: "Ana's old iPhone" }),
    ).toBeInTheDocument();
  });

  it("cannot be chosen", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /My iPhone is backed up to this computer/);
    const locked = (
      await screen.findByRole("heading", { name: "Ana's old iPhone" })
    ).closest("li");

    expect(within(locked!).queryByRole("button")).not.toBeInTheDocument();
    // The one that is not encrypted still can be, so this is not a broken screen.
    expect(
      screen.getByRole("button", { name: "Use this backup" }),
    ).toBeInTheDocument();
  });

  it("says why, and what to change in Finder, and what to save first", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /My iPhone is backed up to this computer/);
    const locked = (
      await screen.findByRole("heading", { name: "Ana's old iPhone" })
    ).closest("li");
    const said = within(locked!);

    expect(said.getByText(/locked with a password/)).toBeInTheDocument();
    expect(said.getByText(/untick .Encrypt local backup./)).toBeInTheDocument();
    expect(said.getByText(/chats are not affected/)).toBeInTheDocument();
    expect(
      said.getByText(/one backup per phone, and the next one replaces it/),
    ).toBeInTheDocument();
  });
});

describe("when the backups cannot be looked at", () => {
  /**
   * The server's own words, in the shape it really sends them: a sentence, a blank
   * line, and then several lines of advice with the folder it was refused in them.
   * An empty list beside that message is what this whole case exists to prevent.
   */
  const refused =
    "the backup could not be read because of a permissions restriction: " +
    "/Users/someone/Library/Application Support/MobileSync/Backup\n\n" +
    "macOS keeps iPhone backups behind Full Disk Access, and this program does\n" +
    "not have it.\n\n" +
    "  System Settings > Privacy & Security > Full Disk Access";

  beforeEach(() => {
    backups = { backups: [], problem: refused };
  });

  it("says what stopped it rather than showing an empty list", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /My iPhone is backed up to this computer/);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /permissions restriction/,
    );
    expect(
      screen.queryByText("No iPhone backup was found on this computer."),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Use this backup" }),
    ).not.toBeInTheDocument();
  });

  /**
   * Being told it is a permission is no use without being told where to grant it,
   * and only the server knows which folder it was actually refused.
   */
  it("keeps the line breaks the advice was written with", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /My iPhone is backed up to this computer/);

    const advice = await screen.findByText(/Full Disk Access/);
    expect(advice.tagName).toBe("PRE");
    expect(advice.textContent).toContain("\n");
  });

  /** A server that sends a bare sentence still leaves somebody knowing where to go. */
  it("says where to grant it itself when the server did not", async () => {
    backups = { backups: [], problem: "permission denied" };
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /My iPhone is backed up to this computer/);

    expect(
      await screen.findByText(/Privacy & Security, then Full Disk Access/),
    ).toBeInTheDocument();
  });
});

describe("while the work is happening", () => {
  it("says which part of it, and that nothing is being uploaded", async () => {
    states = [
      {
        stage: "working",
        workspace,
        step: "decrypting",
        detail: "Reading the header.",
      },
    ];
    render(<App language="en" />);

    expect(
      await screen.findByText("Unlocking the backup."),
    ).toBeInTheDocument();
    expect(screen.getByText("Reading the header.")).toBeInTheDocument();
    expect(screen.getByText(/Nothing is being uploaded/)).toBeInTheDocument();
  });

  /**
   * The one thing in this program that changes while somebody watches it. Nothing
   * tells the page the work has ended, so it asks again until the stage moves.
   */
  it("keeps asking until the stage moves, and then shows the archive", async () => {
    states = [
      {
        stage: "working",
        workspace,
        step: "preparing",
        detail: "Reading the message table.",
      },
      { stage: "ready", workspace },
    ];
    render(<App language="en" />);

    expect(
      await screen.findByText("Reading the message table."),
    ).toBeInTheDocument();

    await waitFor(
      () => {
        expect(
          screen.getByRole("button", { name: "Close this archive" }),
        ).toBeInTheDocument();
      },
      { timeout: 4000 },
    );
  });

  /** A stage a newer server invented is still a program that is working, not a blank. */
  it("says something plain about a step this build has never heard of", async () => {
    states = [{ stage: "working", workspace, detail: "Something new." }];
    render(<App language="en" />);

    expect(await screen.findByText("Working on it.")).toBeInTheDocument();
  });
});

describe("when the work fails", () => {
  const failure: Setup = {
    stage: "failed",
    workspace,
    detail: "the key did not decrypt this backup",
    guidance:
      "Check that the key is the one this phone showed you.\nKeys are not shared between phones.",
  };

  it("shows what went wrong and the advice underneath it", async () => {
    states = [failure];
    render(<App language="en" />);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "the key did not decrypt this backup",
    );
    expect(
      screen.getByText(/Keys are not shared between phones/),
    ).toBeInTheDocument();
  });

  /** It arrives with its line breaks in it and is written to be read as it is. */
  it("keeps the line breaks the advice was written with", async () => {
    states = [failure];
    render(<App language="en" />);

    const advice = await screen.findByText(/Keys are not shared/);
    expect(advice.tagName).toBe("PRE");
    expect(advice.textContent).toContain("\n");
  });

  /**
   * The regression this cost a browser to find.
   *
   * Work replaces the form with the working screen, so a form holding its own answers
   * loses them on the way through — which is to say it loses them exactly when
   * something goes wrong. Being handed back an empty field and told to correct what
   * was wrong is not an instruction anybody can follow.
   */
  it("still holds what was typed, so the next attempt is a correction", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I already have a file/);
    await user.type(
      await screen.findByLabelText("The full path to the file"),
      "/tmp/holiday.jpg",
    );

    // Through the working screen, which replaces the form, and out the other side.
    leadsTo = {
      stage: "working",
      workspace,
      step: "opening",
      detail: "Reading the file.",
    };
    await user.click(screen.getByRole("button", { name: "Open it" }));
    expect(await screen.findByText("Reading the file.")).toBeInTheDocument();

    states = [
      {
        stage: "failed",
        workspace,
        detail: "not a database",
        guidance: "Try another.",
      },
    ];
    await waitFor(
      () => {
        expect(screen.getByRole("alert")).toHaveTextContent("not a database");
      },
      { timeout: 4000 },
    );
    expect(screen.getByLabelText("The full path to the file")).toHaveValue(
      "/tmp/holiday.jpg",
    );
  });

  /**
   * The same, five steps in. Being thrown back to the first of them for mistyping a
   * key is how people give up.
   */
  it("leaves somebody where they were in the Android walkthrough", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I have an Android phone/);
    await walkThePhone(user);
    await user.type(
      await screen.findByLabelText(/Where msgstore.db.crypt15 is/),
      "/tmp/one.crypt15",
    );
    await user.type(screen.getByLabelText("The 64-digit key"), key);

    leadsTo = {
      stage: "working",
      workspace,
      step: "decrypting",
      detail: "Unlocking it.",
    };
    await user.click(screen.getByRole("button", { name: "Unlock the file" }));
    expect(await screen.findByText("Unlocking it.")).toBeInTheDocument();

    states = [
      {
        stage: "failed",
        workspace,
        detail: "the key did not decrypt this backup",
      },
    ];
    await waitFor(
      () => {
        expect(screen.getByRole("alert")).toHaveTextContent(
          "the key did not decrypt this backup",
        );
      },
      { timeout: 4000 },
    );

    // Still on the fifth step with the file still named, rather than back at the
    // first of five. The key is gone on purpose: it is sent and forgotten in the
    // same breath, and typing it again is the price of it existing in one fewer place.
    expect(screen.getByLabelText(/Where msgstore.db.crypt15 is/)).toHaveValue(
      "/tmp/one.crypt15",
    );
    expect(screen.getByLabelText("The 64-digit key")).toHaveValue("");
  });

  it("offers a way back, and asks the server to let go on the way", async () => {
    states = [failure];
    const user = userEvent.setup();
    render(<App language="en" />);

    await user.click(
      await screen.findByRole("button", { name: "Go back and try again" }),
    );

    expect(
      await screen.findByText("Where is your WhatsApp history?"),
    ).toBeInTheDocument();
    expect(posted.map((one) => one.at)).toEqual(["/api/close"]);
  });

  /**
   * The way back is not the server's to refuse. A wizard that can strand somebody is
   * worse than no wizard, and the person meeting this screen has already been let
   * down once today.
   */
  it("goes back even when the server will not let go", async () => {
    states = [failure];
    closing = new Response("cannot close", { status: 500 });
    const user = userEvent.setup();
    render(<App language="en" />);

    await user.click(
      await screen.findByRole("button", { name: "Go back and try again" }),
    );

    expect(
      await screen.findByText("Where is your WhatsApp history?"),
    ).toBeInTheDocument();
  });
});

describe("the decryption key", () => {
  /** It is read off a phone screen, often in a room with other people in it. */
  it("is typed into a password field", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I have an Android phone/);
    await walkThePhone(user);

    const box = await screen.findByLabelText("The 64-digit key");
    expect(box).toHaveAttribute("type", "password");
    expect(box).toHaveAttribute("autocomplete", "off");
  });

  it("never appears in the address of any request", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I have an Android phone/);
    await walkThePhone(user);
    await user.type(
      await screen.findByLabelText(/Where msgstore.db.crypt15 is/),
      "/tmp/one.crypt15",
    );
    await user.type(screen.getByLabelText("The 64-digit key"), key);
    await user.click(screen.getByRole("button", { name: "Unlock the file" }));

    await waitFor(() => {
      expect(posted[0]?.body).toMatchObject({ key });
    });
    for (const at of addressesAsked()) {
      expect(at).not.toContain(key);
      expect(at).not.toContain("key");
    }
  });

  it("is gone from the screen as soon as it has been sent", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I have an Android phone/);
    await walkThePhone(user);
    await user.type(
      await screen.findByLabelText(/Where msgstore.db.crypt15 is/),
      "/tmp/one.crypt15",
    );
    await user.type(screen.getByLabelText("The 64-digit key"), key);

    // A refusal is the one answer that leaves the form on screen to be looked at.
    refusing = new Response("into is required\n", { status: 400 });
    await user.click(screen.getByRole("button", { name: "Unlock the file" }));

    await waitFor(() => {
      expect(screen.getByLabelText("The 64-digit key")).toHaveValue("");
    });
    expect(posted[0]?.body).toMatchObject({ key });
  });

  /** The groups WhatsApp shows it in are for reading it aloud, not part of the key. */
  it("arrives without the spaces and line breaks it was written with", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I have an Android phone/);
    await walkThePhone(user);
    await user.type(
      await screen.findByLabelText(/Where msgstore.db.crypt15 is/),
      "/tmp/one.crypt15",
    );
    await user.type(
      screen.getByLabelText("The 64-digit key"),
      key.replace(/(.{8})/g, "$1 "),
    );
    await user.click(screen.getByRole("button", { name: "Unlock the file" }));

    await waitFor(() => {
      expect(posted[0]?.body).toMatchObject({ key });
    });
  });

  /**
   * The mistake everybody makes, caught before the key is carried anywhere: WhatsApp
   * offers a password beside the key, and a password cannot be used here at all.
   */
  const refusals: { name: string; typed: string; said: RegExp }[] = [
    {
      name: "a password typed where a key belongs",
      typed: "correcthorsebattery",
      said: /not a 64-digit key/,
    },
    {
      name: "a key with a character that is not a digit",
      typed: `${"0".repeat(63)}z`,
      said: /not a 64-digit key/,
    },
    { name: "nothing at all", typed: "", said: /Type the key before going on/ },
  ];

  it.each(refusals)(
    "$name is refused without a request",
    async ({ typed, said }) => {
      const user = userEvent.setup();
      render(<App language="en" />);

      await chooseRoute(user, /I have an Android phone/);
      await walkThePhone(user);
      await user.type(
        await screen.findByLabelText(/Where msgstore.db.crypt15 is/),
        "/tmp/one.crypt15",
      );
      if (typed !== "")
        await user.type(screen.getByLabelText("The 64-digit key"), typed);
      await user.click(screen.getByRole("button", { name: "Unlock the file" }));

      expect(await screen.findByText(said)).toBeInTheDocument();
      expect(posted).toEqual([]);
    },
  );
});

describe("walking through an Android phone", () => {
  it("says a password cannot be used, and why the key cannot be recovered from one", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I have an Android phone/);

    expect(
      await screen.findByText(/choose the 64-digit key/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/the key never leaves the phone/),
    ).toBeInTheDocument();
  });

  it("says which file to copy and which ones are no use", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I have an Android phone/);
    for (let screens = 0; screens < 3; screens++) {
      await user.click(await screen.findByRole("button", { name: "Next" }));
    }

    expect(
      await screen.findByText(
        /Android\/media\/com.whatsapp\/WhatsApp\/Databases/,
      ),
    ).toBeInTheDocument();
    expect(screen.getByText(/msgstore-increment/)).toBeInTheDocument();
  });

  it("counts the screens so somebody knows how much is left", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I have an Android phone/);

    expect(await screen.findByText("Step 2 of 6")).toBeInTheDocument();
  });

  it("goes back a screen at a time, and out of the route from the first one", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I have an Android phone/);
    await user.click(await screen.findByRole("button", { name: "Next" }));
    await user.click(screen.getByRole("button", { name: "Back" }));
    expect(await screen.findByText("Step 2 of 6")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Back" }));
    expect(
      await screen.findByText("Where is your WhatsApp history?"),
    ).toBeInTheDocument();
  });
});

describe("the promise the whole program rests on", () => {
  const screens: {
    name: string;
    arrive: (user: UserEvent) => Promise<void>;
    before?: () => void;
  }[] = [
    { name: "the first screen", arrive: () => Promise.resolve() },
    {
      name: "choosing an iPhone backup",
      arrive: (user) =>
        chooseRoute(user, /My iPhone is backed up to this computer/),
    },
    {
      name: "walking through an Android phone",
      arrive: (user) => chooseRoute(user, /I have an Android phone/),
    },
    {
      name: "opening a file somebody already has",
      arrive: (user) => chooseRoute(user, /I already have a file/),
    },
    {
      name: "while the work is happening",
      before: () => {
        states = [{ stage: "working", workspace, step: "opening" }];
      },
      arrive: () => Promise.resolve(),
    },
    {
      name: "after it has failed",
      before: () => {
        states = [{ stage: "failed", workspace, detail: "no" }];
      },
      arrive: () => Promise.resolve(),
    },
  ];

  it.each(screens)(
    "$name says nothing leaves this computer",
    async ({ before, arrive }) => {
      before?.();
      const user = userEvent.setup();
      render(<App language="en" />);
      await arrive(user);

      expect(
        await screen.findByText("Nothing here leaves this computer."),
      ).toBeInTheDocument();
    },
  );
});

describe("a request the server would not accept", () => {
  it("says so rather than looking as though nothing happened", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I already have a file/);
    await user.type(
      await screen.findByLabelText("The full path to the file"),
      "/tmp/msgstore.db",
    );

    refusing = new Response("into is required\n", { status: 400 });
    await user.click(screen.getByRole("button", { name: "Open it" }));

    expect(await screen.findByText(/into is required/)).toBeInTheDocument();
  });
});

describe("when the server cannot be reached at all", () => {
  it("says it may have been closed, and offers to ask again", async () => {
    fetching.mockImplementation(() =>
      Promise.resolve(new Response("", { status: 502 })),
    );
    render(<App language="en" />);

    expect(await screen.findByText(/could not be reached/)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Try again" }),
    ).toBeInTheDocument();
  });
});

describe("closing an archive", () => {
  it("asks the server to let go and comes back to the first screen", async () => {
    states = [{ stage: "ready", workspace }];
    const user = userEvent.setup();
    render(<App language="en" />);

    await user.click(
      await screen.findByRole("button", { name: "Close this archive" }),
    );

    expect(
      await screen.findByText("Where is your WhatsApp history?"),
    ).toBeInTheDocument();
    expect(posted.map((one) => one.at)).toEqual(["/api/close"]);
  });
});

describe("speaking Spanish", () => {
  it("offers the three routes in the reader's own language", async () => {
    render(<App language="es" />, "es");

    expect(
      await screen.findByText("¿Dónde está tu historial de WhatsApp?"),
    ).toBeInTheDocument();
    expect(screen.getByText("Tengo un teléfono Android.")).toBeInTheDocument();
  });
});

describe("what gets written", () => {
  /**
   * Opening an archive builds a search index beside it. Saying so before a path is
   * typed is not politeness: a program that writes a file somebody was not told
   * about has spent the only thing it has.
   */
  it("says what appears beside a file that is only being opened", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I already have a file/);

    expect(
      await screen.findByText(/A search index is written beside it/),
    ).toBeInTheDocument();
  });

  it("says what will be created before an iPhone backup is touched", async () => {
    backups = {
      backups: [
        { path: "/Backups/one", device_name: "Ana's iPhone", encrypted: false },
      ],
    };
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /My iPhone is backed up to this computer/);

    expect(
      await screen.findByText(/as ChatStorage.sqlite, with a search index/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Nothing in the backup itself is changed/),
    ).toBeInTheDocument();
  });
});

describe("an address book", () => {
  /** Without one every conversation is a phone number, which is barely a history. */
  it("travels with the file when somebody has one, and not when they do not", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I already have a file/);
    await user.type(
      await screen.findByLabelText("The full path to the file"),
      "/tmp/msgstore.db",
    );
    await user.type(
      screen.getByLabelText(/An address book/),
      "/tmp/contacts.vcf",
    );
    await user.click(screen.getByRole("button", { name: "Open it" }));

    await waitFor(() => {
      expect(posted[0]?.body).toEqual({
        path: "/tmp/msgstore.db",
        contacts: "/tmp/contacts.vcf",
      });
    });
  });

  /** One field, either kind of file, and never a question about which. */
  it("is one field that takes either kind of file", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I already have a file/);

    expect(
      await screen.findByText(/A .vcf file exported from your contacts, or/),
    ).toHaveTextContent(/wa.db/);
  });
});

describe("a build without the importer", () => {
  // Not that it matters here: the whole stand-in is replaced below so that asking
  // for the backups answers 501, which is how a build without the importer says so.
  beforeEach(() => {
    backups = { backups: [] };
  });

  /** A choice that can only answer with an error is worse than no choice at all. */
  it("does not offer the two routes that would only fail", async () => {
    fetching.mockImplementation((input, init) => {
      const at = fetchTarget(input);
      if (at.startsWith("/api/backups")) {
        return Promise.resolve(
          new Response("no importer in this build\n", { status: 501 }),
        );
      }
      if (init?.method === "POST") return Promise.resolve(answer({}));
      if (at.startsWith("/api/state"))
        return Promise.resolve(answer(nextState()));
      return Promise.resolve(new Response("not found", { status: 404 }));
    });

    render(<App language="en" />);

    expect(
      await screen.findByRole("button", { name: /I already have a file/ }),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: /My iPhone is backed up/ }),
      ).not.toBeInTheDocument();
    });
    expect(
      screen.queryByRole("button", { name: /I have an Android phone/ }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText(/only open a file you already have/),
    ).toBeInTheDocument();
  });
});

describe("work that is already running", () => {
  /**
   * The server answers a second request with 409 because the first one is still
   * going. There is nothing to correct and nothing to retry, so it must not be
   * dressed up as a request somebody got wrong.
   */
  it("is not reported as a request the server would not accept", async () => {
    refusing = new Response("an import is already running\n", { status: 409 });
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I already have a file/);
    await user.type(
      await screen.findByLabelText("The full path to the file"),
      "/tmp/msgstore.db",
    );

    states = [
      {
        stage: "working",
        workspace,
        step: "opening",
        detail: "Reading the schema.",
      },
    ];
    await user.click(screen.getByRole("button", { name: "Open it" }));

    await waitFor(() => {
      expect(screen.getByText("Reading the schema.")).toBeInTheDocument();
    });
    expect(screen.queryByText(/would not accept that/)).not.toBeInTheDocument();
  });
});

describe("a failure with the form still behind it", () => {
  const failed: Setup = {
    stage: "failed",
    workspace,
    detail: "no such file",
    guidance:
      "Check the path.\nDrag the file into a terminal to see its full path.",
  };

  /** Failure is not terminal here: the next attempt is taken without a reset. */
  it("is shown above what was typed, which is still there to correct", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I already have a file/);
    const box = await screen.findByLabelText("The full path to the file");
    await user.type(box, "/tmp/wrong.db");

    leadsTo = failed;
    await user.click(screen.getByRole("button", { name: "Open it" }));

    expect(await screen.findByText("no such file")).toBeInTheDocument();
    expect(
      screen.getByText(/Drag the file into a terminal/),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("The full path to the file")).toHaveValue(
      "/tmp/wrong.db",
    );
  });

  it("takes the corrected answer without anything being reset first", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I already have a file/);
    await user.type(
      await screen.findByLabelText("The full path to the file"),
      "/tmp/wrong.db",
    );

    leadsTo = failed;
    await user.click(screen.getByRole("button", { name: "Open it" }));
    await screen.findByText("no such file");

    const box = screen.getByLabelText("The full path to the file");
    await user.clear(box);
    await user.type(box, "/tmp/right.db");
    await user.click(screen.getByRole("button", { name: "Open it" }));

    await waitFor(() => {
      expect(posted.map((one) => one.body)).toEqual([
        { path: "/tmp/wrong.db" },
        { path: "/tmp/right.db" },
      ]);
    });
  });
});

describe("a key kept in a file", () => {
  /** Somebody who saved the key into a file did exactly what they were told to. */
  it("is sent as the path it is, rather than being refused for not being 64 digits", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await chooseRoute(user, /I have an Android phone/);
    await walkThePhone(user);
    await user.type(
      await screen.findByLabelText(/Where msgstore.db.crypt15 is/),
      "/tmp/one.crypt15",
    );
    await user.type(screen.getByLabelText("The 64-digit key"), "/tmp/key.txt");
    await user.click(screen.getByRole("button", { name: "Unlock the file" }));

    await waitFor(() => {
      expect(posted[0]?.body).toMatchObject({ key: "/tmp/key.txt" });
    });
  });
});

describe("what the wizard has to say about itself", () => {
  /** Somebody who never gets as far as an archive still sees both notices. */
  it("carries the same two notices as the archive does", async () => {
    render(<App language="en" />);

    expect(
      await screen.findByText(
        /Not affiliated with, endorsed by, or connected to/,
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/free software under the AGPL-3.0/),
    ).toBeInTheDocument();
  });
});
