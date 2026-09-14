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
  it("says nothing at all on a computer with no Android tools", async () => {
    phones = { phones: [], trouble: "adb is not installed on this computer" };

    const { container } = render(
      <Plugged language="en" busy={false} chosen="" onChoose={() => undefined} />,
    );
    // Not an error and not a warning. Just absent, so the screen reads as though
    // this was never on offer.
    await vi.waitFor(() => {
      expect(container).toBeEmptyDOMElement();
    });
  });

  it("says nothing when no phone is plugged in", async () => {
    phones = { phones: [] };

    const { container } = render(
      <Plugged language="en" busy={false} chosen="" onChoose={() => undefined} />,
    );
    await vi.waitFor(() => {
      expect(container).toBeEmptyDOMElement();
    });
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
