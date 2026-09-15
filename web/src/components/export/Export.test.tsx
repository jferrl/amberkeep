import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Export as State } from "@/api/types";
import { Export } from "@/components/export/Export";
import { ThanksProvider } from "@/lib/thanks";
import { fetchTarget } from "@/test/fetchTarget";
import { render } from "@/test/render";

/**
 * Keeping a copy, against a stand-in for the server.
 *
 * What is tested is what a person gets: that they cannot ask for nothing, that what
 * they picked is what is sent, and that the end of it tells them where their history
 * actually is. Nothing here is dangerous — it reads an archive and writes files — so
 * unlike the migration there is no word to type and nothing to refuse.
 */

interface Posted {
  at: string;
  body: Record<string, unknown>;
}

let fetching: ReturnType<typeof vi.fn<typeof fetch>>;
let posted: Posted[];
let now: State;
let replies: Partial<Record<string, () => Response>>;
let left: number;

function answer(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** A refusal in the shape the server really sends one: plain text, not JSON. */
function refuses(said: string, status = 409): Response {
  return new Response(said, {
    status,
    headers: { "Content-Type": "text/plain" },
  });
}

beforeEach(() => {
  posted = [];
  now = { stage: "idle" };
  replies = {};
  left = 0;

  fetching = vi.fn<typeof fetch>((input, init) => {
    const at = fetchTarget(input);
    if (init?.method === "POST") {
      const body: unknown =
        typeof init.body === "string" ? JSON.parse(init.body) : {};
      posted.push({ at, body: body as Record<string, unknown> });
      const said = replies[at];
      return Promise.resolve(said === undefined ? answer(now) : said());
    }
    if (at.startsWith("/api/export")) return Promise.resolve(answer(now));
    return Promise.resolve(new Response("not found", { status: 404 }));
  });
  vi.stubGlobal("fetch", fetching);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

function show(thanks?: string) {
  render(
    <ThanksProvider value={thanks}>
      <Export
        language="en"
        onLeave={() => {
          left += 1;
        }}
      />
    </ThanksProvider>,
  );
}

function sentTo(at: string): Record<string, unknown> | undefined {
  return posted.find((one) => one.at === at)?.body;
}

describe("choosing what to keep", () => {
  it("offers web pages first, and starts with them chosen", async () => {
    show();
    const pages = await screen.findByLabelText("Web pages");
    expect(pages).toBeChecked();
    // The other two are there, and off, because most people want the readable one.
    expect(screen.getByLabelText("Plain text")).not.toBeChecked();
    expect(screen.getByLabelText("Structured data")).not.toBeChecked();
  });

  it("sends exactly what was ticked", async () => {
    const user = userEvent.setup();
    replies["/api/export"] = () => answer({ stage: "writing" }, 202);

    show();
    await user.click(await screen.findByLabelText("Structured data"));
    await user.click(screen.getByRole("button", { name: "Write it out" }));

    expect(sentTo("/api/export")).toEqual({
      formats: ["html", "json"],
      groups: true,
      notices: false,
    });
  });

  it("carries the folder somebody named", async () => {
    const user = userEvent.setup();
    replies["/api/export"] = () => answer({ stage: "writing" }, 202);

    show();
    await user.type(
      await screen.findByLabelText(/Where to put it/),
      "/Users/someone/Archive",
    );
    await user.click(screen.getByRole("button", { name: "Write it out" }));

    expect(sentTo("/api/export")).toMatchObject({
      into: "/Users/someone/Archive",
    });
  });

  /** Asking for nothing is a mistake worth naming, not a button that does nothing. */
  it("will not write nothing, and says why", async () => {
    const user = userEvent.setup();

    show();
    await user.click(await screen.findByLabelText("Web pages"));
    await user.click(screen.getByRole("button", { name: "Write it out" }));

    expect(
      await screen.findByText("Choose at least one thing to write."),
    ).toBeInTheDocument();
    // The poll uses the same address, so only a POST counts as writing.
    expect(posted.filter((one) => one.at === "/api/export")).toHaveLength(0);
  });

  it("says nothing is uploaded, on the screen where the history leaves the program", async () => {
    show();
    expect(await screen.findByText(/nothing is uploaded/)).toBeInTheDocument();
  });
});

describe("while it is running", () => {
  it("shows what it is doing rather than a still screen", async () => {
    now = {
      stage: "writing",
      step: "writing",
      detail: "25 of 60 conversations written.",
    };

    show();
    expect(
      await screen.findByText("25 of 60 conversations written."),
    ).toBeInTheDocument();
  });
});

describe("when it is done", () => {
  beforeEach(() => {
    now = {
      stage: "done",
      result: {
        into: "/Users/someone/Amberkeep/archive",
        conversations: 412,
        messages: 1121482,
        bytes: 98_000_000,
        formats: ["html"],
      },
    };
  });

  /**
   * The one screen in this program that mentions money, and the only reason it is
   * here: somebody's history has just been written out and it worked.
   */
  it("offers the coffee, and only when the program sent somewhere to point at", async () => {
    show();
    expect(
      await screen.findByText(/conversations written|conversaciones escritas/),
    ).toBeVisible();
    // Nothing yet: this build knows of nowhere to point at.
    expect(screen.queryByRole("link", { name: /ko-fi/i })).toBeNull();

    show("https://ko-fi.com/jferrl");

    const link = await screen.findByRole("link", { name: /ko-fi.com\/jferrl/ });
    expect(link).toHaveAttribute("href", "https://ko-fi.com/jferrl");
  });

  it("says where the history is, which is the only thing that matters now", async () => {
    show();
    expect(
      await screen.findByText("/Users/someone/Amberkeep/archive"),
    ).toBeInTheDocument();
    expect(screen.getByText(/1,121,482/)).toBeInTheDocument();
  });

  it("tells somebody to copy it somewhere else", async () => {
    show();
    expect(
      await screen.findByText(/A backup on the same computer is not a backup/),
    ).toBeInTheDocument();
  });

  it("goes back to the archive", async () => {
    const user = userEvent.setup();

    show();
    await user.click(
      await screen.findByRole("button", { name: "Back to the archive" }),
    );
    expect(left).toBe(1);
  });

  it("forgets the last one before writing another", async () => {
    const user = userEvent.setup();
    replies["/api/export/forget"] = () => answer({ stage: "idle" });

    show();
    await user.click(
      await screen.findByRole("button", { name: "Write another copy" }),
    );
    expect(sentTo("/api/export/forget")).toEqual({});
  });
});

describe("when the server refuses", () => {
  it("says so in the server's own words, and keeps the form", async () => {
    const user = userEvent.setup();
    replies["/api/export"] = () => refuses("an export is already running");

    show();
    await user.click(
      await screen.findByRole("button", { name: "Write it out" }),
    );

    expect(
      await screen.findByText(/an export is already running/),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Web pages")).toBeInTheDocument();
  });
});
