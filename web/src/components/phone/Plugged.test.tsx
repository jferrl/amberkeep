import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Phone, PhoneBackup, Phones } from "@/api/types";
import { Plugged } from "@/components/phone/Plugged";
import { fetchTarget } from "@/test/fetchTarget";
import { render } from "@/test/render";

/**
 * The phone, plugged in.
 *
 * This screen exists to replace a paragraph telling somebody to open
 * Android/media/com.whatsapp/WhatsApp/Databases and copy a file out of it. So what
 * is tested is mostly the cases where it cannot help: no Android tools, no cable, a
 * phone that will not be trusted, no backup yet. Each of those has something
 * different for a person to go and do, and an empty list for all four would send
 * them nowhere.
 */

let fetching: ReturnType<typeof vi.fn<typeof fetch>>;
let phones: Phones;
let backups: readonly PhoneBackup[];
let chose: { serial: string; path: string } | undefined;

function answer(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

const trusted: Phone = { serial: "R5CT30", name: "SM_A566B", ready: true };

const whole: PhoneBackup = {
  path: "/sdcard/Android/media/com.whatsapp/WhatsApp/Databases/msgstore.db.crypt15",
  name: "msgstore.db.crypt15",
  size: 247_382_016,
  partial: false,
};

const fragment: PhoneBackup = {
  path: "/sdcard/Android/media/com.whatsapp/WhatsApp/Databases/msgstore-increment-1.db.crypt15",
  name: "msgstore-increment-1.db.crypt15",
  size: 1_048_576,
  partial: true,
};

beforeEach(() => {
  phones = { phones: [trusted] };
  backups = [whole];
  chose = undefined;

  fetching = vi.fn<typeof fetch>((input) => {
    const at = fetchTarget(input);
    if (at.includes("/backups")) return Promise.resolve(answer({ backups }));
    if (at.startsWith("/api/phones")) return Promise.resolve(answer(phones));
    return Promise.resolve(new Response("not found", { status: 404 }));
  });
  vi.stubGlobal("fetch", fetching);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

function show(busy = false) {
  render(
    <Plugged
      language="en"
      busy={busy}
      chosen=""
      onChoose={(phone, backup) => {
        chose = { serial: phone.serial, path: backup.path };
      }}
    />,
  );
}

describe("when a phone is there", () => {
  it("offers the backup on it, with its size", async () => {
    show();
    // The row appears once the phone has been asked what is on it.
    expect(await screen.findByText(/msgstore/)).toBeInTheDocument();
    expect(screen.getByText(/SM_A566B/)).toBeInTheDocument();
    // 236 MB rather than 247382016, which is a number nobody reads.
    expect(screen.getByText(/236 MB/)).toBeInTheDocument();
  });

  it("hands back which phone and which file", async () => {
    const user = userEvent.setup();

    show();
    await user.click(await screen.findByRole("button", { name: "Use this backup" }));
    expect(chose).toEqual({ serial: "R5CT30", path: whole.path });
  });

  /**
   * The fragment is the trap this screen exists to remove. It sits in the same
   * folder, looks almost identical, and is useless alone — so it is never offered,
   * and its existence is said in words so that somebody who can see two files on
   * their phone and one here is not left wondering which was dropped.
   */
  it("never offers a part-file, and says the part-files are there", async () => {
    backups = [whole, fragment];

    show();
    await screen.findByRole("button", { name: "Use this backup" });

    expect(screen.getAllByRole("button", { name: "Use this backup" })).toHaveLength(1);
    expect(screen.queryByText(/msgstore-increment/)).not.toBeInTheDocument();
    expect(screen.getByText(/part-files/)).toBeInTheDocument();
  });

  it("offers nothing when the only thing there is a part-file", async () => {
    backups = [fragment];

    show();
    expect(await screen.findByText(/No backup on SM_A566B yet/)).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});

describe("when it cannot help", () => {
  /**
   * Each of these is an ordinary situation with a different thing to go and do, and
   * the screen underneath still works in all of them.
   */
  /**
   * Missing tooling is an offer, folded away.
   *
   * Not silence, because somebody who would happily install one small thing cannot
   * find out otherwise. Not a heading either: the path underneath needs nothing
   * installed and works just as well, and a person with a dead phone has no
   * appetite for a side quest presented as the main road.
   */
  it("offers to explain the tooling, without making it the main path", async () => {
    phones = {
      phones: [],
      trouble: "adb is not installed on this computer",
      why: "no-tools",
      platform: "darwin",
    };

    render(<Plugged language="en" busy={false} chosen="" onChoose={() => undefined} />);

    const fold = (await screen.findByText(/take it off the phone instead/i)).closest("details");
    expect(fold).not.toBeNull();
    expect(fold).not.toHaveAttribute("open");
    // The instruction is the one for this machine, not three and a choice.
    expect(screen.getByText(/brew install/)).toBeInTheDocument();
    expect(screen.queryByText(/winget/)).not.toBeInTheDocument();
  });

  /**
   * Installed and broken is not the same as never installed, and telling somebody to
   * install it again would send them round a loop that cannot end.
   */
  it("does not tell somebody to install what is already installed", async () => {
    phones = {
      phones: [],
      trouble: "the Android tools on this computer would not run: /usr/bin/adb",
      why: "unusable",
      platform: "darwin",
    };

    render(<Plugged language="en" busy={false} chosen="" onChoose={() => undefined} />);

    expect(await screen.findByText(/will not run/)).toBeInTheDocument();
    expect(screen.queryByText(/brew install/)).not.toBeInTheDocument();
    expect(screen.getByText(/installing it again will not help/)).toBeInTheDocument();
  });

  /** The promise this program is built on does not get an exception for convenience. */
  it("says plainly that it will not download anything itself", async () => {
    phones = { phones: [], trouble: "x", why: "no-tools", platform: "windows" };

    render(<Plugged language="en" busy={false} chosen="" onChoose={() => undefined} />);
    expect(await screen.findByText(/will not download or install anything itself/)).toBeInTheDocument();
  });

  /**
   * Nothing plugged in is not the same as nothing on offer.
   *
   * This used to render nothing, which meant somebody holding the phone this exists
   * for could not discover the option: it only appeared once they had already
   * guessed to plug the phone in. Being invisible until you no longer need it is
   * not a feature.
   */
  it("invites the phone in when this computer can look and sees none", async () => {
    phones = { phones: [] };

    render(<Plugged language="en" busy={false} chosen="" onChoose={() => undefined} />);

    expect(await screen.findByText(/Plug the phone in/)).toBeInTheDocument();
    expect(screen.getByText(/Many charging cables carry no data/)).toBeInTheDocument();
    // And nothing to press, because there is nothing there yet.
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  const stuck: { name: string; phone: Phone; says: RegExp }[] = [
    {
      name: "a phone that has not been trusted",
      phone: { serial: "R5CT30", name: "R5CT30", ready: false, trouble: "unauthorized" },
      says: /tap Allow/,
    },
    {
      name: "a phone that will not answer",
      phone: { serial: "R5CT30", name: "R5CT30", ready: false, trouble: "offline" },
      says: /different cable/,
    },
  ];

  it.each(stuck)("$name is told what to do about it", async ({ phone, says }) => {
    phones = { phones: [phone] };

    show();
    expect(await screen.findByText(says)).toBeInTheDocument();
    // And it is never offered as though it could be used.
    expect(screen.queryByRole("button", { name: "Use this backup" })).not.toBeInTheDocument();
  });

  it("does not ask an untrusted phone what is on it", async () => {
    phones = { phones: [{ serial: "R5CT30", name: "R5CT30", ready: false, trouble: "unauthorized" }] };

    show();
    await screen.findByText(/tap Allow/);

    // Asking would fail, and the failure would read as the program being broken.
    const asked = fetching.mock.calls.map((call) => fetchTarget(call[0]));
    expect(asked.some((at) => at.includes("/backups"))).toBe(false);
  });
});

describe("what it shows about a phone", () => {
  it("names the phone rather than its serial, when the phone will say", async () => {
    show();
    const row = (await screen.findByText("SM_A566B")).closest("li");
    expect(row).not.toBeNull();
    expect(within(row as HTMLElement).getByText(/msgstore/)).toBeInTheDocument();
  });
});
