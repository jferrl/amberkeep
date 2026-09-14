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
          skipped: "no messages this could add",
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
function show(onLeave = () => (left += 1)) {
  render(<Migration language="en" onLeave={onLeave} />);
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
    await user.click(screen.getByRole("button", { name: "Do it" }));
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
  /** Nothing but the word itself opens the button that writes. */
  const attempts: { name: string; typed: string; opens: boolean }[] = [
    { name: "nothing typed leaves it shut", typed: "", opens: false },
    { name: "half of it leaves it shut", typed: "migr", opens: false },
    { name: "another word leaves it shut", typed: "yes", opens: false },
    { name: "the word opens it", typed: "migrate", opens: true },
    {
      name: "the word with spaces round it opens it",
      typed: "  migrate  ",
      opens: true,
    },
  ];

  it.each(attempts)("$name", async ({ typed, opens }) => {
    const user = userEvent.setup();
    now = planned();

    show();
    const box = await screen.findByLabelText("Type migrate");
    if (typed !== "") await user.type(box, typed);

    const button = screen.getByRole("button", { name: "Do it" });
    if (opens) expect(button).toBeEnabled();
    else expect(button).toBeDisabled();
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

  it("says the whole time that nothing leaves this computer", async () => {
    show();
    expect(
      await screen.findByText(
        /Nothing is uploaded and nothing is sent anywhere/,
      ),
    ).toBeInTheDocument();
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
