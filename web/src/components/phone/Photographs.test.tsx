import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { PhoneMedia } from "@/api/types";
import { Photographs } from "@/components/phone/Photographs";
import { fetchTarget } from "@/test/fetchTarget";
import { render } from "@/test/render";

/**
 * Offering to bring the phone's photographs across.
 *
 * The thing worth testing is what a person is told before they agree to it. This is
 * the longest wait in the whole program — several gigabytes over a cable, before any
 * messages appear — so the size has to be on the label, and a phone with no such
 * folder has to be offered nothing at all rather than a tick box that would do
 * nothing.
 */

let fetching: ReturnType<typeof vi.fn<typeof fetch>>;
let media: PhoneMedia;
let on: boolean;

function answer(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  on = false;
  media = {
    path: "/sdcard/Android/media/com.whatsapp/WhatsApp/Media",
    bytes: 5_700_000_000,
    kinds: ["WhatsApp Images", "WhatsApp Video", "WhatsApp Voice Notes"],
  };

  fetching = vi.fn<typeof fetch>((input) => {
    const at = fetchTarget(input);
    if (at.includes("/media")) return Promise.resolve(answer(media));
    return Promise.resolve(new Response("not found", { status: 404 }));
  });
  vi.stubGlobal("fetch", fetching);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

function show() {
  render(
    <Photographs
      serial="R5CT30"
      language="en"
      on={on}
      onChange={(next) => {
        on = next;
      }}
    />,
  );
}

describe("bringing the photographs", () => {
  it("says how many gigabytes before anybody agrees to wait for them", async () => {
    show();

    // The size is on the label itself, not buried underneath: "this may take a
    // while" is what every program says before taking an hour.
    expect(await screen.findByLabelText(/Bring the photographs too \(5.3 GB\)/)).toBeInTheDocument();
  });

  it("starts off, because it happens before any messages appear", async () => {
    show();
    expect(await screen.findByRole("checkbox")).not.toBeChecked();
  });

  it("says what is lost by leaving it off", async () => {
    show();
    await screen.findByRole("checkbox");

    expect(
      screen.getByText(/small recovered copies of pictures and a line of text/i),
    ).toBeInTheDocument();
  });

  it("is what gets turned on", async () => {
    const user = userEvent.setup();
    show();

    await user.click(await screen.findByRole("checkbox"));
    expect(on).toBe(true);
  });

  it("offers nothing at all when the phone has no such folder", async () => {
    media = { path: "this phone has no WhatsApp media folder", bytes: 0, kinds: [] };

    show();
    // Settled rather than merely un-rendered: the request has been made and answered.
    await vi.waitFor(() => {
      expect(fetching).toHaveBeenCalled();
    });
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  it("offers it without a size when the phone would not say how large it is", async () => {
    media = { ...media, bytes: 0 };

    show();
    const box = await screen.findByLabelText("Bring the photographs too");
    expect(box).toBeInTheDocument();
  });
});
