import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { UserEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type {
  BackupList,
  Finding,
  Guide,
  Migration as State,
  MigrationPlan,
} from "@/api/types";
import { Migration } from "@/components/migration/Migration";
import { ThanksProvider } from "@/lib/thanks";
import { fetchTarget } from "@/test/fetchTarget";
import { render } from "@/test/render";

/**
 * Moving a history onto a phone, against a stand-in for the server it will meet.
 *
 * What is tested here is the thing that makes this screen safe rather than the way it
 * looks: that nothing advances on its own, that the word gates the only step which
 * writes, and that a refusal leaves what somebody typed where they can correct it.
 * The bodies posted are asserted by name, because they are the contract with
 * `internal/api` and a field renamed on either side becomes a 400 nobody can act on.
 */

/** One request that asked the server to do something, and what it carried. */
interface Posted {
  at: string;
  body: Record<string, unknown>;
}

const backupPath = "/Backups/00008110-aaa";
const androidPath = "/tmp/msgstore.db";

let fetching: ReturnType<typeof vi.fn<typeof fetch>>;
let posted: Posted[];
let now: State;
/** The backups the computer has made, in the shape the server really sends. */
let backups: BackupList;
/** What each address replies with next, if it should not reply with the state. */
let replies: Partial<Record<string, () => Response>>;
let left: number;

function answer(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/**
 * A refusal, in the shape the server actually sends one.
 *
 * Go's `http.Error` writes plain text, not JSON. A stand-in that refused in JSON
 * would put the braces on screen and the test would still pass, which is the kind of
 * agreement between a test and its own fiction that proves nothing.
 */
function refuses(said: string, status = 400): Response {
  return new Response(said, {
    status,
    headers: { "Content-Type": "text/plain" },
  });
}

/** A finding, with the defaults that make one that passed. */
function finding(over: Partial<Finding> = {}): Finding {
  return {
    step: "safety-backup",
    title: "A check",
    passed: true,
    blocking: false,
    ...over,
  };
}

/** A plan with one conversation in it, which is enough to have something to agree to. */
function planned(over: Partial<MigrationPlan> = {}): State {
  return {
    stage: "planned",
    plan: {
      conversations: [
        {
          address: "34600000001@s.whatsapp.net",
          name: "Someone",
          kind: "person",
          destination: "34600000001@s.whatsapp.net",
          adding: 12,
          already_there: 0,
          untranslatable: 0,
          as_placeholders: 3,
          on_phone_already: 0,
        },
        {
          address: "34600000002@s.whatsapp.net",
          name: "Nobody",
          kind: "person",
          destination: "34600000002@s.whatsapp.net",
          adding: 0,
          already_there: 0,
          untranslatable: 0,
          as_placeholders: 0,
          on_phone_already: 0,
          skipped: { note: "groups-not-included", text: "groups were not included" },
        },
      ],
      adding: 12,
      already_there: 0,
      untranslatable: 0,
      as_placeholders: 3,
      merging: 1,
      creating: 0,
      untouched: 1,
      ...over,
    },
  };
}

const guide: Guide = {
  // The sentences a finding or a conversation names, as the program serves them:
  // in whichever language the page asked for, with the holes still in them.
  sentences: {
    "not-encrypted": "La copia de seguridad no está cifrada",
    "is-encrypted": "está cifrada, y una copia cifrada está sellada con una clave que nunca sale del teléfono",
    "days-ago": "hecha hace {days} días",
    "groups-not-included": "no se han incluido los grupos",
    "warn-placeholders": "{messages} mensajes llegarán como una línea de texto.",
  },
  stages: [
    {
      stage: "before",
      heading: "Before you start",
      steps: [
        {
          id: "safety-backup",
          title: "Make a safety backup and archive it",
          body: "Finder keeps one backup per phone and overwrites it.",
          critical: true,
        },
      ],
    },
  ],
};

beforeEach(() => {
  posted = [];
  now = { stage: "idle" };
  backups = { backups: [] };
  replies = {};
  left = 0;

  fetching = vi.fn<typeof fetch>((input, init) => {
    const at = fetchTarget(input);

    if (init?.method === "POST") {
      const body: unknown =
        typeof init.body === "string" ? JSON.parse(init.body) : {};
      posted.push({ at, body: body as Record<string, unknown> });

      const said = replies[at];
      if (said !== undefined) return Promise.resolve(said());
      return Promise.resolve(answer(now));
    }

    if (at.startsWith("/api/backups")) return Promise.resolve(answer(backups));
    if (at.startsWith("/api/migration/guide"))
      return Promise.resolve(answer(guide));
    if (at.startsWith("/api/migration")) return Promise.resolve(answer(now));

    return Promise.resolve(new Response("not found", { status: 404 }));
  });

  vi.stubGlobal("fetch", fetching);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

/** Every address this page asked for, in order. */
function addressesAsked(): string[] {
  return fetching.mock.calls.map((call) => fetchTarget(call[0]));
}

/** What a POST to one address carried, the first time it was sent. */
function sentTo(at: string): Record<string, unknown> | undefined {
  return posted.find((one) => one.at === at)?.body;
}

/** show puts the screen up and waits for the first thing it draws. */
function show(onLeave = () => (left += 1), thanks?: string) {
  render(
    <ThanksProvider value={thanks}>
      <Migration language="en" onLeave={onLeave} />
    </ThanksProvider>,
  );
}

/** fillIn names the two halves and presses the button that looks at them. */
async function fillIn(
  user: UserEvent,
  extras: { pairing?: string } = {},
): Promise<void> {
  await user.type(
    await screen.findByLabelText("The iPhone backup folder"),
    backupPath,
  );
  await user.type(
    screen.getByLabelText("The decrypted Android database"),
    androidPath,
  );
  if (extras.pairing !== undefined) {
    await user.type(screen.getByLabelText(/LID.sqlite/), extras.pairing);
  }
  await user.click(screen.getByRole("button", { name: "Look at the backup" }));
}

describe("the journey", () => {
  it("goes from two paths to a backup on disk, one press at a time", async () => {
    const user = userEvent.setup();

    replies["/api/migration/check"] = () =>
      answer({
        stage: "checked",
        checks: { findings: [finding({ title: "Not encrypted" })] },
      });
    replies["/api/migration/plan"] = () => answer(planned());
    replies["/api/migration/carry-out"] = () =>
      answer({
        stage: "done",
        result: {
          backup: "/Backups/00008110-aaa.amberkeep",
          added: 12,
          merged: 1,
          created: 0,
          checks: 21,
          files: 27352,
        },
      });

    show();
    await fillIn(user, { pairing: "/tmp/LID.sqlite" });

    expect(sentTo("/api/migration/check")).toEqual({ backup: backupPath });

    await user.click(
      await screen.findByRole("button", { name: "Work out what would move" }),
    );
    expect(sentTo("/api/migration/plan")).toEqual({
      backup: backupPath,
      android: androidPath,
      pairing: "/tmp/LID.sqlite",
    });

    await user.type(await screen.findByLabelText("Type migrate"), "migrate");
    await user.click(
      screen.getByRole("button", { name: "Write the changed backup" }),
    );
    expect(sentTo("/api/migration/carry-out")).toEqual({ confirm: "migrate" });

    expect(
      await screen.findByText("There is a backup to restore"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("/Backups/00008110-aaa.amberkeep"),
    ).toBeInTheDocument();
  });

  it("asks for nothing until somebody presses something", async () => {
    const user = userEvent.setup();
    replies["/api/migration/check"] = () =>
      answer({ stage: "checked", checks: { findings: [finding()] } });

    show();
    await fillIn(user);
    await screen.findByRole("button", { name: "Work out what would move" });

    // Checking finished and the plan is one press away. Nothing has asked for it.
    expect(addressesAsked()).not.toContain("/api/migration/plan");
    expect(addressesAsked()).not.toContain("/api/migration/carry-out");
  });

  it("sends the two switches only when they are on", async () => {
    const user = userEvent.setup();
    replies["/api/migration/check"] = () =>
      answer({ stage: "checked", checks: { findings: [finding()] } });
    replies["/api/migration/plan"] = () => answer(planned());

    show();
    await user.type(
      await screen.findByLabelText("The iPhone backup folder"),
      backupPath,
    );
    await user.type(
      screen.getByLabelText("The decrypted Android database"),
      androidPath,
    );
    await user.click(screen.getByLabelText("Include group conversations"));
    await user.click(
      screen.getByRole("button", { name: "Look at the backup" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Work out what would move" }),
    );

    expect(sentTo("/api/migration/plan")).toEqual({
      backup: backupPath,
      android: androidPath,
      groups: true,
    });
  });
});

describe("the word", () => {
  /**
   * Nothing but the word itself sends anything.
   *
   * The button is reachable whatever is typed, and refuses with a sentence. It used
   * to sit dimmed instead, which meant a capital letter produced a screen where
   * nothing happened and nothing said why — the exact thing this program argues
   * against two files away, where it refuses to draw a button it cannot honour.
   */
  const attempts: { name: string; typed: string; sends: boolean }[] = [
    { name: "nothing typed sends nothing", typed: "", sends: false },
    { name: "half of it sends nothing", typed: "migr", sends: false },
    { name: "another word sends nothing", typed: "yes", sends: false },
    { name: "the wrong case sends nothing", typed: "Migrate", sends: false },
    { name: "the word sends it", typed: "migrate", sends: true },
    {
      name: "the word with spaces round it sends it",
      typed: "  migrate  ",
      sends: true,
    },
  ];

  it.each(attempts)("$name", async ({ typed, sends }) => {
    const user = userEvent.setup();
    now = planned();
    replies["/api/migration/carry-out"] = () => answer({ stage: "working" });

    show();
    const box = await screen.findByLabelText("Type migrate");
    if (typed !== "") await user.type(box, typed);

    // Always reachable. What it does depends on what was typed.
    const button = screen.getByRole("button", {
      name: "Write the changed backup",
    });
    expect(button).toBeEnabled();
    await user.click(button);

    if (sends) {
      expect(sentTo("/api/migration/carry-out")).toEqual({
        confirm: "migrate",
      });
    } else {
      expect(addressesAsked()).not.toContain("/api/migration/carry-out");
      // And it says so, rather than doing nothing visible.
      expect(
        await screen.findByText(/That is not the word/),
      ).toBeInTheDocument();
    }
  });

  it("writes nothing while the word is wrong, however hard the form is pushed", async () => {
    const user = userEvent.setup();
    now = planned();

    show();
    await user.type(await screen.findByLabelText("Type migrate"), "no");
    await user.keyboard("{Enter}");

    expect(addressesAsked()).not.toContain("/api/migration/carry-out");
  });
});

describe("when something is wrong", () => {
  it("will not look at a backup nobody has named", async () => {
    const user = userEvent.setup();

    show();
    await user.click(
      await screen.findByRole("button", { name: "Look at the backup" }),
    );

    expect(
      await screen.findAllByText("Say where the file is before going on."),
    ).toHaveLength(2);
    expect(addressesAsked()).not.toContain("/api/migration/check");
  });

  it("keeps what was typed when the server refuses, and takes the next attempt", async () => {
    const user = userEvent.setup();
    let first = true;
    replies["/api/migration/check"] = () => {
      if (!first)
        return answer({ stage: "checked", checks: { findings: [finding()] } });
      first = false;
      return refuses("that folder is not a backup");
    };

    show();
    await fillIn(user);

    expect(
      await screen.findByText(
        /Amberkeep would not accept that: that folder is not a backup/,
      ),
    ).toBeInTheDocument();
    // The form is still there, with both paths in it, ready to be corrected.
    expect(screen.getByLabelText("The iPhone backup folder")).toHaveValue(
      backupPath,
    );

    await user.click(
      screen.getByRole("button", { name: "Look at the backup" }),
    );
    expect(
      await screen.findByRole("button", { name: "Work out what would move" }),
    ).toBeEnabled();
  });

  it("will not let a blocked check be planned past", async () => {
    now = {
      stage: "checked",
      checks: {
        findings: [
          finding({ title: "Not encrypted" }),
          finding({
            step: "safety-backup",
            title: "Make a safety backup and archive it",
            passed: false,
            blocking: true,
          }),
        ],
      },
    };

    show();
    expect(
      await screen.findByRole("button", { name: "Work out what would move" }),
    ).toBeDisabled();
    // And the step that says what to do about it is on screen, not just the complaint.
    expect(
      screen.getByText("Finder keeps one backup per phone and overwrites it."),
    ).toBeVisible();
  });

  /**
   * A finding names its two sentences and the program carries both of them in
   * whichever language the page asked for. This is the screen where somebody is told
   * why their migration cannot go on, and it used to say it in English whatever they
   * were reading.
   */
  it("says what was checked and what was found in the reader's own language", async () => {
    now = {
      stage: "checked",
      checks: {
        findings: [
          finding({
            check: "not-encrypted",
            title: "The backup is not encrypted",
            note: "is-encrypted",
            detail: "it is encrypted, and an encrypted backup is sealed with a key",
            passed: false,
            blocking: true,
          }),
          finding({
            check: "unheard-of",
            title: "Something a newer program checks",
            note: "days-ago",
            detail: "taken 8 days ago",
            values: { days: "8" },
          }),
        ],
      },
    };

    show();

    expect(
      await screen.findByText("La copia de seguridad no está cifrada"),
    ).toBeVisible();
    expect(
      screen.getByText(/está cifrada, y una copia cifrada está sellada/),
    ).toBeVisible();

    // The numbers go into the sentence the program sent, not the one this page has.
    expect(screen.getByText("hecha hace 8 días")).toBeVisible();

    // And a name this build's guide has never heard of falls back to the English
    // beside it rather than to a blank line.
    expect(
      screen.getByText("Something a newer program checks"),
    ).toBeVisible();
  });

  /**
   * The screen where somebody types a word to agree to this. What they are agreeing
   * to was described in English whatever they were reading, which is the worst place
   * in the program to do that.
   */
  it("says what would be left out, and what to read first, in the reader's language", async () => {
    now = planned({
      warnings: [
        {
          note: "warn-placeholders",
          text: "9 messages will arrive as a line of text.",
          values: { messages: "9" },
        },
        { note: "unheard-of", text: "Something a newer program warns about." },
      ],
    });

    show();

    expect(
      await screen.findByText("9 mensajes llegarán como una línea de texto."),
    ).toBeVisible();
    // A name this build's guide does not know keeps the English rather than going blank.
    expect(
      screen.getByText("Something a newer program warns about."),
    ).toBeVisible();

    // And the reason a conversation is not moving, which is inside the fold.
    await userEvent.setup().click(screen.getByText(/Se quedan fuera|Left out/));
    expect(screen.getByText(/no se han incluido los grupos/)).toBeVisible();
  });

  /** The other of the two moments, and the other half of the same wiring. */
  it("offers the coffee once the backup exists", async () => {
    now = {
      stage: "done",
      result: {
        backup: "/backups/00008030-0011.amberkeep",
        added: 34,
        merged: 1,
        created: 0,
        checks: 31,
        files: 27352,
      },
    };

    show(undefined, "https://ko-fi.com/jferrl");

    const link = await screen.findByRole("link", { name: /coffee/i });
    expect(link).toHaveAttribute("href", "https://ko-fi.com/jferrl");
  });

  it("says so rather than offering the word when there is nothing to move", async () => {
    now = planned({ adding: 0 });

    show();
    expect(
      await screen.findByText(
        "There is nothing to move: every message is already on the iPhone.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Do it" }),
    ).not.toBeInTheDocument();
  });

  it("shows a failure above the screen that caused it, with the way back", async () => {
    now = {
      stage: "failed",
      detail: "the disk filled up",
      guidance: "Free some room and try again",
    };

    show();
    expect(await screen.findByText(/the disk filled up/)).toBeInTheDocument();
    // Still the first screen: a failure is not a stage somebody is stuck in.
    expect(
      screen.getByLabelText("The iPhone backup folder"),
    ).toBeInTheDocument();
  });
});

describe("what it says out loud", () => {
  /** A tick and a cross carry the whole meaning of the check screen and neither reads aloud. */
  const verdicts: {
    name: string;
    passed: boolean;
    blocking: boolean;
    said: string;
  }[] = [
    { name: "one that passed", passed: true, blocking: false, said: "Passed:" },
    {
      name: "one that has to be put right",
      passed: false,
      blocking: true,
      said: "Has to be put right:",
    },
    {
      name: "one worth reading",
      passed: false,
      blocking: false,
      said: "Worth reading:",
    },
  ];

  it.each(verdicts)(
    "$name is said in words, not only in a mark",
    async ({ passed, blocking, said }) => {
      now = {
        stage: "checked",
        checks: {
          findings: [
            finding({ title: "Backup encryption is off", passed, blocking }),
          ],
        },
      };

      show();
      const row = (await screen.findByText("Backup encryption is off")).closest(
        "span",
      );
      expect(row).not.toBeNull();
      expect(within(row as HTMLElement).getByText(said)).toBeInTheDocument();
    },
  );

  /**
   * Once, not three times.
   *
   * This screen used to make the claim twice in its own lead, under a permanent
   * strip that already made it. Saying it three times in one viewport does not make
   * it truer; it makes all three read as filler. The strip is not part of this
   * component, so what is checked here is that the screen no longer repeats it.
   */
  it("does not repeat the promise the page already makes", async () => {
    show();
    await screen.findByLabelText("The iPhone backup folder");

    expect(screen.queryByText(/nothing is sent anywhere/i)).not.toBeInTheDocument();
    // The one it does make is about where the files are, which is this screen's own.
    expect(screen.getByText(/already on this computer/)).toBeInTheDocument();
  });

  it("never stops saying that no backup it made has been restored to a phone", async () => {
    now = {
      stage: "done",
      result: {
        backup: "/x",
        added: 1,
        merged: 0,
        created: 1,
        checks: 21,
        files: 9,
      },
    };

    show();
    expect(
      await screen.findAllByText(
        "No backup Amberkeep has produced has yet been restored to a phone.",
      ),
    ).not.toHaveLength(0);
  });
});

describe("leaving", () => {
  it("goes back where it came from from the first screen", async () => {
    const user = userEvent.setup();

    show();
    await user.click(await screen.findByRole("button", { name: /Back/ }));
    expect(left).toBe(1);
  });

  it("forgets the migration rather than the page when there is one to forget", async () => {
    const user = userEvent.setup();
    now = { stage: "checked", checks: { findings: [finding()] } };
    replies["/api/migration/forget"] = () => answer({ stage: "idle" });

    show();
    await user.click(
      await screen.findByRole("button", { name: "Change something" }),
    );

    expect(sentTo("/api/migration/forget")).toEqual({});
    expect(left).toBe(0);
    expect(
      await screen.findByLabelText("The iPhone backup folder"),
    ).toBeInTheDocument();
  });
});

describe("choosing a backup rather than typing one", () => {
  /** Two backups as the server lists them: one usable, one locked. */
  function onThisComputer(): BackupList {
    return {
      backups: [
        {
          path: backupPath,
          device_name: "Ana's iPhone",
          ios_version: "17.4",
          last_backup: "2024-03-02T21:04:00Z",
          encrypted: false,
        },
        {
          path: "/Backups/00008110-bbb",
          device_name: "The old one",
          ios_version: "16.1",
          last_backup: "2023-01-09T08:00:00Z",
          encrypted: true,
        },
      ],
    };
  }

  it("offers the ones this computer has, so nobody has to go and find a folder", async () => {
    backups = onThisComputer();

    show();
    expect(await screen.findByText("Ana's iPhone")).toBeInTheDocument();
    // Enough to tell two apart without opening either.
    expect(screen.getByText(/iOS 17.4/)).toBeInTheDocument();
  });

  it("fills the field when one is picked, so the two ways agree", async () => {
    const user = userEvent.setup();
    backups = onThisComputer();

    show();
    await user.click(
      await screen.findByRole("button", { name: "Use this backup" }),
    );

    expect(screen.getByLabelText("The iPhone backup folder")).toHaveValue(
      backupPath,
    );
    // And it says which one, rather than leaving somebody to compare paths.
    expect(screen.getAllByText("Chosen").length).toBeGreaterThan(0);
  });

  it("sends the picked path when the backup is looked at", async () => {
    const user = userEvent.setup();
    backups = onThisComputer();
    replies["/api/migration/check"] = () =>
      answer({ stage: "checked", checks: { findings: [finding()] } });

    show();
    await user.click(
      await screen.findByRole("button", { name: "Use this backup" }),
    );
    await user.type(
      screen.getByLabelText("The decrypted Android database"),
      androidPath,
    );
    await user.click(
      screen.getByRole("button", { name: "Look at the backup" }),
    );

    expect(sentTo("/api/migration/check")).toEqual({ backup: backupPath });
  });

  /**
   * An encrypted backup is listed rather than hidden. Somebody who cannot see the
   * backup they know exists concludes the program is broken, or that the backup is.
   */
  it("shows a locked backup and says why it cannot be used, with no button to press", async () => {
    backups = onThisComputer();

    show();
    const locked = (await screen.findByText("The old one")).closest("li");
    expect(locked).not.toBeNull();

    const inside = within(locked as HTMLElement);
    expect(
      inside.getByText("Encrypted, so it cannot be opened"),
    ).toBeInTheDocument();
    expect(inside.queryByRole("button")).not.toBeInTheDocument();
  });

  /**
   * The list is a convenience, not the only way in. A backup on an external disk is
   * not in it, and on macOS the folder is often unreadable until somebody grants a
   * permission — neither is a reason to be unable to go on.
   */
  const withoutAList: {
    name: string;
    list: () => BackupList | undefined;
    says: RegExp;
  }[] = [
    {
      name: "when the folder could not be read",
      list: () => ({
        backups: [],
        looked: ["/somewhere"],
        problem: "Amberkeep may not read that folder.",
      }),
      says: /may not read that folder/,
    },
    {
      name: "when there are none",
      list: () => ({
        backups: [],
        looked: ["/Users/someone/…/MobileSync/Backup"],
      }),
      says: /No iPhone backup was found/,
    },
    {
      // Apple ships no Finder, iTunes or Apple Devices for Linux, so "no backup was
      // found, here is how to make one in Finder" is an instruction nobody there can
      // follow. Same empty list, different thing to say.
      name: "when this computer has nowhere for one to be",
      list: () => ({ backups: [], looked: [] }),
      says: /cannot make or restore an iPhone backup/,
    },
  ];

  /**
   * The hint under the field has to match what is actually above it. "Pick one
   * above" with nothing above is the kind of small lie that makes somebody distrust
   * the rest of the screen.
   */
  const hints: { name: string; list: BackupList; says: RegExp }[] = [
    {
      name: "when backups are offered, it says to pick one",
      list: {
        backups: [
          { path: backupPath, device_name: "Ana's iPhone", encrypted: false },
        ],
      },
      says: /Pick one above/,
    },
    {
      name: "when none are offered, it does not point at a list that is not there",
      list: { backups: [] },
      says: /Where Finder keeps the backup/,
    },
  ];

  it.each(hints)("$name", async ({ list, says }) => {
    backups = list;

    show();
    // findByText rather than getByText: the field is drawn before the list has
    // arrived, so the hint settles a moment after the screen does.
    expect(await screen.findByText(says)).toBeInTheDocument();
  });

  /**
   * The permission instructions are several paragraphs of System Settings
   * navigation. On the wizard's own screen they are the only way forward and are
   * shown; here the field below works, so they are folded away rather than put in
   * front of the thing somebody can actually do.
   */
  it("folds the permission instructions away, since typing a path still works", async () => {
    backups = {
      backups: [],
      problem: "Amberkeep may not read that folder.\n\nOpen System Settings.",
    };

    show();
    expect(
      await screen.findByText(/may not read that folder/),
    ).toBeInTheDocument();

    const fold = screen.getByText("How to put that right").closest("details");
    expect(fold).not.toBeNull();
    expect(fold).not.toHaveAttribute("open");
  });

  it.each(withoutAList)(
    "$name, the path can still be typed",
    async ({ list, says }) => {
      const user = userEvent.setup();
      const answered = list();
      if (answered !== undefined) backups = answered;
      replies["/api/migration/check"] = () =>
        answer({ stage: "checked", checks: { findings: [finding()] } });

      show();
      expect(await screen.findByText(says)).toBeInTheDocument();

      await fillIn(user);
      expect(sentTo("/api/migration/check")).toEqual({ backup: backupPath });
    },
  );
});
