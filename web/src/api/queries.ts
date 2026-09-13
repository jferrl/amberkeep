/**
 * The archive as the page asks for it: cached, cancelled and paged.
 *
 * Almost everything here is allowed to go stale never. The server reads the archive
 * once when it starts and holds it, and no part of it can change while the page is
 * open, so an answer is good until somebody reloads. That is unusual enough to be
 * worth saying out loud, because the usual reason for a short stale time is a world
 * that moves underneath you, and this one does not.
 *
 * The one thing that is not cached forever is searching, because every word somebody
 * types leaves a set of results behind and keeping all of them would grow without
 * end.
 */

import {
  infiniteQueryOptions,
  keepPreviousData,
  queryOptions,
  useInfiniteQuery,
  useQuery,
} from "@tanstack/react-query";

import type { Cancellable } from "./client";
import { getArchive, getChats, getMessages, search } from "./client";
import type { Chat, ChatList, SearchPage } from "./types";

/** An answer that cannot go out of date while the server that gave it is running. */
const neverStale = Number.POSITIVE_INFINITY;

/** Results are let go of once nothing has shown them for a while. */
const searchesKeptFor = 5 * 60 * 1000;

/** The server's own page size for a conversation, and the one the viewer was built around. */
const messagesPerPage = 60;

/** The most conversations the server will return at once. */
const chatsPerRequest = 1000;

/**
 * A stop on how many times the conversation list is asked for.
 *
 * Twenty thousand conversations is far beyond anything a phone has held, so reaching
 * this means a server reporting a total it does not send, and a page that keeps
 * asking would hang rather than fail.
 */
const chatRequestCeiling = 20;

const resultsPerSearch = 100;

/**
 * The addresses of everything in the cache.
 *
 * Tuples rather than strings, so that invalidating every conversation or every search
 * is a prefix rather than a guess at how the pieces were joined.
 */
export const queryKeys = {
  archive: ["archive"] as const,
  chats: (q: string) => ["chats", q] as const,
  messages: (jid: string) => ["messages", jid] as const,
  search: (term: string, chat: string) => ["search", term, chat] as const,
} as const;

/**
 * allChats gathers the whole conversation list, however many requests that takes.
 *
 * The server will not return more than a thousand at once and a real archive holds
 * several thousand, so a single request quietly loses the rest. The list is shown
 * whole and virtualised, so the pages are collected here rather than made the
 * caller's problem.
 */
export async function allChats(
  q: string,
  { limit = chatsPerRequest, signal }: Cancellable & { limit?: number | undefined } = {},
): Promise<ChatList> {
  const chats: Chat[] = [];
  let total = 0;

  for (let request = 0; request < chatRequestCeiling; request++) {
    const page = await getChats({ q, limit, offset: chats.length, signal });
    total = page.total;
    for (const chat of page.chats) chats.push(chat);
    if (page.chats.length === 0 || chats.length >= total) break;
  }

  return { total, chats };
}

/** What the archive holds, which is settled before the page ever loads. */
export function archiveQuery() {
  return queryOptions({
    queryKey: queryKeys.archive,
    queryFn: ({ signal }) => getArchive({ signal }),
    staleTime: neverStale,
    gcTime: neverStale,
  });
}

/** The conversation list, narrowed by what somebody typed into the filter. */
export function chatsQuery(q: string) {
  return queryOptions({
    queryKey: queryKeys.chats(q),
    queryFn: ({ signal }) => allChats(q, { signal }),
    staleTime: neverStale,
    // The filter narrows a letter at a time. Holding the previous list while the next
    // one arrives is what stops it emptying between keystrokes.
    placeholderData: keepPreviousData,
  });
}

/**
 * One conversation, read from its end backwards.
 *
 * What this query calls the next page is the conversation's previous one: a reader
 * scrolls upwards into the past. Each answer carries the position to continue from,
 * and its absence is how the beginning of a conversation is recognised.
 */
export function messagesQuery(jid: string) {
  return infiniteQueryOptions({
    queryKey: queryKeys.messages(jid),
    queryFn: ({ pageParam, signal }) =>
      getMessages(jid, { before: pageParam, limit: messagesPerPage, signal }),
    // No cursor is where a conversation opens: at the end, where the last thing said
    // is. It is the same emptiness the server reads as "the most recent page".
    initialPageParam: "",
    getNextPageParam: (page) => page.before,
    staleTime: neverStale,
    enabled: jid !== "",
  });
}

/** Everything in the archive matching what somebody typed. */
export function searchQuery(term: string, chat?: string) {
  const words = term.trim();
  return queryOptions({
    queryKey: queryKeys.search(words, chat ?? ""),
    // An empty box is not a question. It is answered here rather than asked of the
    // server, which refuses an empty search with a status the page would then have to
    // explain away.
    queryFn: ({ signal }): Promise<SearchPage> =>
      words === ""
        ? Promise.resolve({ total: 0, hits: [] })
        : search(words, { chat, limit: resultsPerSearch, signal }),
    staleTime: neverStale,
    gcTime: searchesKeptFor,
    placeholderData: keepPreviousData,
  });
}

/** useArchive reports what this archive holds, for the page that introduces it. */
export function useArchive() {
  return useQuery(archiveQuery());
}

/** useChats is the conversation list, whole, narrowed by a name. */
export function useChats(q = "") {
  return useQuery(chatsQuery(q));
}

/** useMessages is one conversation, growing upwards as somebody scrolls back. */
export function useMessages(jid: string) {
  return useInfiniteQuery(messagesQuery(jid));
}

/** useSearch answers nothing for an empty box, and the whole archive otherwise. */
export function useSearch(term: string, chat?: string) {
  return useQuery(searchQuery(term, chat));
}
