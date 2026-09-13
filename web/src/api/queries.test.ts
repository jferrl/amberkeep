import { QueryClient } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { fetchTarget } from "@/test/fetchTarget";

import {
  allChats,
  archiveQuery,
  chatsQuery,
  messagesQuery,
  queryKeys,
  searchQuery,
} from "./queries";
import type { Chat, MessagePage } from "./types";

const fetching = vi.fn<typeof fetch>();
const client = new QueryClient();

function answering(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json; charset=utf-8" },
  });
}

/** reading is the context TanStack Query hands a query function. */
function reading<K extends readonly unknown[]>(queryKey: K, pageParam: string) {
  return {
    client,
    queryKey,
    pageParam,
    signal: new AbortController().signal,
    meta: undefined,
    direction: "forward" as const,
  };
}

/**
 * present narrows away an absence the option types allow and these options never
 * have: a query without a function is a query nothing would ever fetch.
 */
function present<T>(value: T | undefined): T {
  if (value === undefined) throw new Error("these options were expected to carry a query function");
  return value;
}

function conversation(address: string): Chat {
  return { id: 1, address, kind: "direct", name: address, message_count: 0 };
}

function page(before?: string): MessagePage {
  const chat = conversation("34600123456@s.whatsapp.net");
  return before === undefined ? { chat, messages: [] } : { chat, messages: [], before };
}

/**
 * requested is the address of one of the requests that were made.
 *
 * fetch takes three different things as its first argument and each says where it is
 * going differently, so the one that was passed is asked rather than stringified.
 */
function requested(at = 0): string {
  const target = fetching.mock.calls[at]?.[0];
  if (typeof target === "string") return target;
  if (target instanceof URL) return target.href;
  if (target instanceof Request) return target.url;
  return "";
}

/** asked is how many times the server was asked anything. */
function asked(): number {
  return fetching.mock.calls.length;
}

beforeEach(() => {
  fetching.mockReset();
  fetching.mockImplementation(() => Promise.resolve(answering({ total: 0, chats: [] })));
  vi.stubGlobal("fetch", fetching);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

const keys: { name: string; got: readonly unknown[]; want: readonly unknown[] }[] = [
  {
    name: "the archive has one address, because there is one archive",
    got: queryKeys.archive,
    want: ["archive"],
  },
  {
    name: "a filtered conversation list is a different question from an unfiltered one",
    got: queryKeys.chats("ana"),
    want: ["chats", "ana"],
  },
  {
    name: "every conversation is cached apart from every other",
    got: queryKeys.messages("34600123456@s.whatsapp.net"),
    want: ["messages", "34600123456@s.whatsapp.net"],
  },
  {
    name: "a search of the whole archive is not a search inside one conversation",
    got: queryKeys.search("beach", ""),
    want: ["search", "beach", ""],
  },
  {
    name: "the conversation a search is limited to is part of its address",
    got: queryKeys.search("beach", "34600123456@s.whatsapp.net"),
    want: ["search", "beach", "34600123456@s.whatsapp.net"],
  },
];

describe("what a query is cached under", () => {
  it.each(keys)("$name", ({ got, want }) => {
    expect(got).toEqual(want);
  });

  it("holds the words a search was trimmed to, not what was typed", () => {
    expect(searchQuery("  beach  ").queryKey).toEqual(["search", "beach", ""]);
  });
});

describe("how long an answer is good for", () => {
  it("keeps the archive forever, because the server read it once and holds it", () => {
    expect(archiveQuery().staleTime).toBe(Number.POSITIVE_INFINITY);
  });

  it("keeps the conversation list for the same reason", () => {
    expect(chatsQuery("").staleTime).toBe(Number.POSITIVE_INFINITY);
  });
});

describe("reading a conversation backwards", () => {
  it("opens at the end, asking for no position", async () => {
    const options = messagesQuery("34600123456@s.whatsapp.net");

    expect(options.initialPageParam).toBe("");
    await present(options.queryFn)(reading(options.queryKey, options.initialPageParam));

    expect(requested()).toBe("/api/chats/34600123456%40s.whatsapp.net/messages?limit=60");
  });

  it("continues from the position the server gave back", async () => {
    const options = messagesQuery("34600123456@s.whatsapp.net");
    const older = page("1712345678901_42");

    const next = options.getNextPageParam(older, [older], "", [""]);
    expect(next).toBe("1712345678901_42");

    await present(options.queryFn)(reading(options.queryKey, next ?? ""));

    expect(requested()).toBe(
      "/api/chats/34600123456%40s.whatsapp.net/messages?before=1712345678901_42&limit=60",
    );
  });

  it("stops at the beginning of a conversation, which is a page with no position", () => {
    const options = messagesQuery("34600123456@s.whatsapp.net");
    const first = page();

    expect(options.getNextPageParam(first, [first], "1712345678901_42", [""])).toBeUndefined();
  });

  it("asks nothing until there is a conversation to ask about", () => {
    expect(messagesQuery("").enabled).toBe(false);
    expect(messagesQuery("34600123456@s.whatsapp.net").enabled).toBe(true);
  });
});

describe("gathering the conversation list", () => {
  it("asks again until it holds as many as the server says there are", async () => {
    const everyone = ["a", "b", "c", "d", "e"].map((name) => conversation(name));
    fetching.mockImplementation((input) => {
      const query = new URLSearchParams(fetchTarget(input).split("?")[1] ?? "");
      const from = Number(query.get("offset") ?? "0");
      return Promise.resolve(
        answering({ total: everyone.length, chats: everyone.slice(from, from + 2) }),
      );
    });

    const list = await allChats("", { limit: 2 });

    expect(list.total).toBe(5);
    expect(list.chats.map((chat) => chat.address)).toEqual(["a", "b", "c", "d", "e"]);
    expect(requested(0)).toBe("/api/chats?limit=2&offset=0");
    expect(requested(1)).toBe("/api/chats?limit=2&offset=2");
    expect(requested(2)).toBe("/api/chats?limit=2&offset=4");
    expect(asked()).toBe(3);
  });

  it("carries the name being filtered by into every request", async () => {
    fetching.mockImplementation(() =>
      Promise.resolve(answering({ total: 1, chats: [conversation("a")] })),
    );

    await allChats("ana", { limit: 2 });

    expect(requested(0)).toBe("/api/chats?q=ana&limit=2&offset=0");
    expect(asked()).toBe(1);
  });

  it("stops when a server reports more conversations than it sends", async () => {
    fetching.mockImplementation(() =>
      Promise.resolve(answering({ total: 10_000, chats: [conversation("a")] })),
    );

    const list = await allChats("", { limit: 1 });

    expect(asked()).toBe(20);
    expect(list.chats).toHaveLength(20);
  });

  it("stops when a server sends nothing at all", async () => {
    fetching.mockImplementation(() => Promise.resolve(answering({ total: 10, chats: [] })));

    const list = await allChats("", { limit: 2 });

    expect(list.chats).toHaveLength(0);
    expect(asked()).toBe(1);
  });
});

describe("searching", () => {
  it("answers an empty box without asking the server", async () => {
    const options = searchQuery("   ");

    await expect(present(options.queryFn)(reading(options.queryKey, ""))).resolves.toEqual({
      total: 0,
      hits: [],
    });
    expect(asked()).toBe(0);
  });

  it("asks for the words and the conversation they are looked for in", async () => {
    fetching.mockImplementation(() => Promise.resolve(answering({ total: 0, hits: [] })));
    const options = searchQuery("beach", "34600123456@s.whatsapp.net");

    await present(options.queryFn)(reading(options.queryKey, ""));

    expect(requested()).toBe(
      "/api/search?q=beach&chat=34600123456%40s.whatsapp.net&limit=100",
    );
  });
});
