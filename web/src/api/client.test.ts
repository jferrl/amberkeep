import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { fetchTarget } from "@/test/fetchTarget";

import type { ArchiveError } from "./client";
import {
  getArchive,
  getChats,
  getMessages,
  isArchiveError,
  launchSecretIn,
  search,
} from "./client";

const fetching = vi.fn<typeof fetch>();

/** answering is the server replying with JSON, as every handler that succeeds does. */
function answering(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json; charset=utf-8" },
  });
}

/**
 * refusing is the server saying no, which it does in plain text with a newline on
 * the end because that is what Go's http.Error writes.
 */
function refusing(status: number, said: string): Response {
  return new Response(`${said}\n`, {
    status,
    headers: { "Content-Type": "text/plain; charset=utf-8" },
  });
}

/** requested is the address of one of the requests that were made. */
function requested(at = 0): string {
  return fetchTarget(fetching.mock.calls[at]?.[0]);
}

/** refusalFrom runs something expected to fail and hands back how it failed. */
async function refusalFrom(run: () => Promise<unknown>): Promise<ArchiveError> {
  try {
    await run();
  } catch (error) {
    if (isArchiveError(error)) return error;
    throw error;
  }
  throw new Error("the request was expected to be refused and was not");
}

beforeEach(() => {
  fetching.mockReset();
  // A fresh reply per call: a body can only be read once.
  fetching.mockImplementation(() => Promise.resolve(answering({})));
  vi.stubGlobal("fetch", fetching);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

/**
 * The addresses these functions build are the contract with `internal/api`, and the
 * awkward ones are the point: a conversation's address is not a word, it is a phone
 * number with an at sign and a domain after it, and somebody's filter is whatever
 * they typed.
 */
const addresses: { name: string; ask: () => Promise<unknown>; want: string }[] =
  [
    {
      name: "the archive is asked for with nothing attached",
      ask: () => getArchive(),
      want: "/api/archive",
    },
    {
      name: "the conversation list carries no parameters when nothing narrows it",
      ask: () => getChats(),
      want: "/api/chats",
    },
    {
      name: "a name to filter by is escaped into the query rather than pasted in",
      ask: () => getChats({ q: "Ana & Co" }),
      want: "/api/chats?q=Ana+%26+Co",
    },
    {
      name: "an empty filter is left out, so the whole list has one address",
      ask: () => getChats({ q: "" }),
      want: "/api/chats",
    },
    {
      name: "how many and how far in travel as numbers",
      ask: () => getChats({ limit: 1000, offset: 2000 }),
      want: "/api/chats?limit=1000&offset=2000",
    },
    {
      name: "an at sign in a conversation's address is encoded into one path segment",
      ask: () => getMessages("34600123456@s.whatsapp.net"),
      want: "/api/chats/34600123456%40s.whatsapp.net/messages",
    },
    {
      name: "a plus and spaces in an address survive the path",
      ask: () => getMessages("+34 600 123 456@s.whatsapp.net"),
      want: "/api/chats/%2B34%20600%20123%20456%40s.whatsapp.net/messages",
    },
    {
      name: "a group's address, which is a number and a dash, is untouched otherwise",
      ask: () => getMessages("34600123456-1234567890@g.us"),
      want: "/api/chats/34600123456-1234567890%40g.us/messages",
    },
    {
      name: "a conversation opens at its end, asking for no position",
      ask: () => getMessages("34600123456@s.whatsapp.net", { before: "" }),
      want: "/api/chats/34600123456%40s.whatsapp.net/messages",
    },
    {
      name: "the position the server gave back is what the page before is asked for",
      ask: () =>
        getMessages("34600123456@s.whatsapp.net", {
          before: "1712345678901_42",
        }),
      want: "/api/chats/34600123456%40s.whatsapp.net/messages?before=1712345678901_42",
    },
    {
      name: "a position from before 1970 keeps its sign",
      ask: () =>
        getMessages("34600123456@s.whatsapp.net", { before: "-86400000_7" }),
      want: "/api/chats/34600123456%40s.whatsapp.net/messages?before=-86400000_7",
    },
    {
      name: "a cursor and a page size travel together",
      ask: () =>
        getMessages("34600123456@s.whatsapp.net", {
          before: "1712345678901_42",
          limit: 60,
        }),
      want: "/api/chats/34600123456%40s.whatsapp.net/messages?before=1712345678901_42&limit=60",
    },
    {
      name: "searching sends the words as they were typed",
      ask: () => search("beach + sun"),
      want: "/api/search?q=beach+%2B+sun",
    },
    {
      name: "a search limited to one conversation names it by address",
      ask: () =>
        search("beach", { chat: "34600123456@s.whatsapp.net", limit: 100 }),
      want: "/api/search?q=beach&chat=34600123456%40s.whatsapp.net&limit=100",
    },
  ];

describe("the addresses the archive is asked at", () => {
  it.each(addresses)("$name", async ({ ask, want }) => {
    await ask();
    expect(requested()).toBe(want);
  });

  it.each(addresses)("stays on this machine: $name", async ({ ask }) => {
    await ask();
    expect(requested().startsWith("/api/")).toBe(true);
  });

  it("sends the browser's own credentials and nobody else's", async () => {
    await getArchive();
    expect(fetching.mock.calls[0]?.[1]?.credentials).toBe("same-origin");
  });
});

/**
 * What the server says when it refuses. These sentences are the server's own, and
 * carrying them unchanged is better than inventing worse ones: they are what
 * `internal/api` writes for each of these.
 */
const refusals: { name: string; status: number; said: string }[] = [
  {
    name: "a browser without the launch secret is turned away",
    status: 403,
    said: "this archive is open to this browser session only",
  },
  {
    name: "a conversation the archive does not hold is not found",
    status: 404,
    said: "no such conversation",
  },
  {
    name: "a position that is not a position is refused before anything is read",
    status: 400,
    said: `"nonsense" is not a position in a conversation`,
  },
  {
    name: "an archive served without an index says so rather than finding nothing",
    status: 503,
    said: "this archive was served without a search index",
  },
  {
    name: "an archive that cannot be read reports why",
    status: 500,
    said: "reading the conversation: database is locked",
  },
];

describe("a refusal", () => {
  it.each(refusals)("$name", async ({ status, said }) => {
    fetching.mockImplementation(() => Promise.resolve(refusing(status, said)));

    const error = await refusalFrom(() => getArchive());

    expect(error.status).toBe(status);
    expect(error.message).toBe(said);
  });

  it("drops the newline Go puts on the end of every one of them", async () => {
    fetching.mockImplementation(() =>
      Promise.resolve(refusing(404, "no such conversation")),
    );

    const error = await refusalFrom(() =>
      getMessages("34600123456@s.whatsapp.net"),
    );

    expect(error.message).not.toContain("\n");
  });

  it("says what it can when the server explains nothing", async () => {
    fetching.mockImplementation(() =>
      Promise.resolve(new Response("", { status: 502 })),
    );

    const error = await refusalFrom(() => getArchive());

    expect(error.status).toBe(502);
    expect(error.message).toContain("502");
  });

  it("does not try to read a refusal as JSON", async () => {
    fetching.mockImplementation(() =>
      Promise.resolve(refusing(403, "not for this browser")),
    );

    const error = await refusalFrom(() => getChats());

    expect(error.message).toBe("not for this browser");
  });

  it("reports a reply that arrived broken rather than throwing a syntax error", async () => {
    fetching.mockImplementation(() =>
      Promise.resolve(new Response('{"chats": [', { status: 200 })),
    );

    const error = await refusalFrom(() => getChats());

    expect(error.status).toBe(200);
    expect(error.message).toContain("could not read");
  });
});

describe("an answer", () => {
  it("is handed back as the server sent it", async () => {
    const page = {
      total: 1,
      chats: [{ address: "34600123456@s.whatsapp.net" }],
    };
    fetching.mockImplementation(() => Promise.resolve(answering(page)));

    await expect(getChats()).resolves.toEqual(page);
  });
});

/**
 * The launch secret. It arrives once in the address the browser was opened with, and
 * the server answers with a cookie every later request travels on.
 */
const secrets: { name: string; search: string; want: string }[] = [
  {
    name: "the address the browser was opened with carries it",
    search: "?t=abc123",
    want: "abc123",
  },
  {
    name: "it is found beside other parameters",
    search: "?chat=34600&t=abc123",
    want: "abc123",
  },
  { name: "an ordinary address carries none", search: "", want: "" },
  {
    name: "an address with other parameters carries none",
    search: "?chat=34600",
    want: "",
  },
  { name: "an empty one is no secret at all", search: "?t=", want: "" },
  {
    name: "the name is case sensitive, as the server reads it",
    search: "?T=abc123",
    want: "",
  },
  {
    name: "escaped characters are read back",
    search: "?t=a%2Bb%2Fc",
    want: "a+b/c",
  },
];

describe("the launch secret", () => {
  it.each(secrets)("$name", ({ search: query, want }) => {
    expect(launchSecretIn(query)).toBe(want);
  });

  it("is sent once and then left to the cookie the server set", async () => {
    const was = window.location.href;
    window.history.replaceState(null, "", "/?t=abc123");
    vi.resetModules();

    try {
      const fresh = await import("./client");
      await fresh.getArchive();
      await fresh.getArchive();

      expect(requested(0)).toBe("/api/archive?t=abc123");
      expect(requested(1)).toBe("/api/archive");
    } finally {
      window.history.replaceState(null, "", was);
      vi.resetModules();
    }
  });
});
