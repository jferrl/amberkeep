import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { App } from "@/App";
import { markClose, markOpen } from "@/api/marks";
import { render } from "@/test/render";
import { fetchTarget } from "@/test/fetchTarget";

/**
 * The whole page, over a small archive.
 *
 * It talks to a stand-in for the server rather than to a real one, but it talks to
 * it exactly as it would: the same addresses, the same shapes, and the same two
 * control characters marking a search result. What this proves is that the pieces
 * fit together, which no test of one component can.
 */

/**
 * The conversations, in the order the server sends them.
 *
 * It sorts them by when each was last used, once, when it starts. The page must
 * show them in that order rather than sorting again: two sorts that disagree is a
 * list that changes under the reader for no reason they can see.
 */
const conversations = [
  {
    id: 2,
    address: "120363001@g.us",
    kind: "group",
    name: "Vermut del sabado",
    message_count: 1,
    last_message_at: "2019-06-15T12:00:00Z",
    participants: [{ address: "34600111222@s.whatsapp.net", name: "Ana Lopez" }],
  },
  {
    id: 1,
    address: "34600111222@s.whatsapp.net",
    kind: "direct",
    name: "Ana Lopez",
    message_count: 2,
    last_message_at: "2019-06-14T09:20:00Z",
  },
];

const messages = [
  {
    id: 1,
    sent_at: "2019-06-14T09:12:00Z",
    from_me: false,
    sender_name: "Ana Lopez",
    kind: "text",
    text: "are you up?",
    rendered: "are you up?",
    source_type: 0,
  },
  {
    id: 2,
    sent_at: "2019-06-14T09:20:00Z",
    from_me: true,
    kind: "text",
    text: "just about",
    rendered: "just about",
    source_type: 0,
  },
];

let fetching: ReturnType<typeof vi.fn<typeof fetch>>;

/** answer builds what the stand-in server replies with. */
function answer(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  fetching = vi.fn<typeof fetch>((input) => {
    const at = fetchTarget(input);

    if (at.startsWith("/api/archive")) {
      return Promise.resolve(
        answer({
          title: "Archive",
          layout: "modern",
          conversations: 2,
          messages: 3,
          by_kind: { direct: 1, group: 1 },
          people: 2,
          named: 2,
          searchable: true,
          time_zone: "Europe/Madrid",
        }),
      );
    }
    if (at.startsWith("/api/chats?")) {
      return Promise.resolve(answer({ total: conversations.length, chats: conversations }));
    }
    if (at.includes("/messages")) {
      return Promise.resolve(answer({ chat: conversations[1], messages }));
    }
    if (at.startsWith("/api/search")) {
      return Promise.resolve(
        answer({
          total: 1,
          hits: [
            {
              chat_name: "Ana Lopez",
              chat_address: "34600111222@s.whatsapp.net",
              sender: "Ana Lopez",
              from_me: false,
              sent_at: "2019-06-14T09:12:00Z",
              kind: "text",
              snippet: `are you ${markOpen}up${markClose}?`,
            },
          ],
        }),
      );
    }
    return Promise.resolve(new Response("not found", { status: 404 }));
  });

  vi.stubGlobal("fetch", fetching);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("opening an archive", () => {
  it("says what it holds", async () => {
    render(<App language="en" />);
    expect(await screen.findByText(/2 conversations/)).toBeInTheDocument();
  });

  it("lists them in the order the server sent them", async () => {
    render(<App language="en" />);

    const list = await screen.findByRole("list");
    const names = within(list)
      .getAllByRole("button")
      .map((button) => button.textContent);

    expect(names[0]).toContain("Vermut del sabado");
    expect(names[1]).toContain("Ana Lopez");
  });

  it("asks the reader to choose one", async () => {
    render(<App language="en" />);
    expect(await screen.findByText(/Choose a conversation/)).toBeInTheDocument();
  });

  it("speaks Spanish to a Spanish reader", async () => {
    render(<App language="es" />, "es");
    expect(await screen.findByText(/2 conversaciones/)).toBeInTheDocument();
  });
});

describe("reading a conversation", () => {
  it("opens the one that was chosen", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    await user.click(await screen.findByRole("button", { name: /Ana Lopez/ }));

    // The name is now in the reading pane's heading as well as the list.
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Ana Lopez" })).toBeInTheDocument();
    });
    expect(screen.getByText(/2 messages/)).toBeInTheDocument();
  });
});

describe("searching", () => {
  it("finds a message and marks the words that matched", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    const box = await screen.findByRole("searchbox", { name: /Search everything/ });
    await user.type(box, "up");

    // The marks are two control characters, and what the reader sees is an element.
    const marked = await screen.findByText("up", { selector: "mark" });
    expect(marked).toBeInTheDocument();
    expect(screen.getByText(/1 match/)).toBeInTheDocument();
  });

  it("says plainly when nothing matched", async () => {
    fetching.mockImplementation((input) => {
      const at = fetchTarget(input);
      if (at.startsWith("/api/search")) {
        return Promise.resolve(answer({ total: 0, hits: [] }));
      }
      if (at.startsWith("/api/archive")) {
        return Promise.resolve(answer({ conversations: 0, messages: 0, searchable: true }));
      }
      return Promise.resolve(answer({ total: 0, chats: [] }));
    });

    const user = userEvent.setup();
    render(<App language="en" />);

    await user.type(await screen.findByRole("searchbox"), "tortuga");
    expect(await screen.findByText("Nothing found")).toBeInTheDocument();
  });

  it("clearing the box goes back to the conversations", async () => {
    const user = userEvent.setup();
    render(<App language="en" />);

    const box = await screen.findByRole("searchbox");
    await user.type(box, "up");
    await screen.findByText(/1 match/);

    await user.click(screen.getByRole("button", { name: "Clear" }));
    await waitFor(() => {
      expect(screen.getByRole("list")).toBeInTheDocument();
    });
  });

  /**
   * Without an index the server can only match conversation names, and a box that
   * looks as though it searches everything and does not is worse than one that says
   * what it searches.
   */
  it("offers to search only names when the archive has no index", async () => {
    fetching.mockImplementation((input) => {
      const at = fetchTarget(input);
      if (at.startsWith("/api/archive")) {
        return Promise.resolve(answer({ conversations: 2, messages: 3, searchable: false }));
      }
      return Promise.resolve(answer({ total: conversations.length, chats: conversations }));
    });

    render(<App language="en" />);
    expect(
      await screen.findByRole("searchbox", { name: /Search conversation names/ }),
    ).toBeInTheDocument();
  });
});

describe("when the archive cannot be read", () => {
  it("does not pretend it is empty", async () => {
    fetching.mockImplementation(() =>
      Promise.resolve(new Response("the disk gave up", { status: 500 })),
    );

    render(<App language="en" />);

    // Nothing is invented: no conversation list appears.
    await waitFor(() => {
      expect(screen.queryByRole("list")).not.toBeInTheDocument();
    });
  });
});
