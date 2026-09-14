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
  useQueryClient,
} from "@tanstack/react-query";
import { useCallback, useState } from "react";

import type { Cancellable } from "./client";
import {
  closeArchive,
  getArchive,
  getBackups,
  getChats,
  getMessages,
  getState,
  isArchiveError,
  search,
} from "./client";
import type { Chat, ChatList, SearchPage, Setup } from "./types";

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
 * How often the server is asked whether the work has moved on.
 *
 * Twice a second. Slower and a step that takes two seconds looks like a program that
 * has hung; faster buys nothing, because the steps being reported are minutes long
 * and the server only changes what it says when one of them ends.
 */
const pollEvery = 500;

/** The server saying it is already doing this, which is not a mistake to correct. */
const alreadyRunning = 409;

/** The server saying it was built without the importer, so none of this exists. */
const noImporter = 501;

/**
 * The addresses of everything in the cache.
 *
 * Tuples rather than strings, so that invalidating every conversation or every search
 * is a prefix rather than a guess at how the pieces were joined.
 */
export const queryKeys = {
  state: ["state"] as const,
  backups: ["backups"] as const,
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

/**
 * What the server is doing, asked again while it is doing something.
 *
 * This is the one query in the program that is allowed to go stale, and the comment
 * at the top of this file is why that is worth saying: everything else was read once
 * when the server started and cannot change underneath a reader. This can. It is the
 * only thing that polls, and it stops polling the moment the work ends, so a page
 * sitting on the chooser makes no requests at all.
 */
export function setupQuery() {
  return queryOptions({
    queryKey: queryKeys.state,
    queryFn: ({ signal }) => getState({ signal }),
    staleTime: 0,
    gcTime: 0,
    refetchInterval: (query) => (query.state.data?.stage === "working" ? pollEvery : false),
  });
}

/**
 * The iPhone backups on this computer.
 *
 * Not cached beyond the screen that shows it: somebody who is told no backup was
 * found goes away, makes one, and comes back, and a cached empty list would tell
 * them it had not worked.
 */
export function backupsQuery(enabled: boolean) {
  return queryOptions({
    queryKey: queryKeys.backups,
    queryFn: ({ signal }) => getBackups({ signal }),
    staleTime: 0,
    gcTime: 0,
    // Minutes of decrypting is no time to be rummaging through somebody's backup
    // folder, and the answer would be thrown away before anybody saw it.
    enabled,
  });
}

/** useSetupState is what the server is doing, and where it will write what it makes. */
export function useSetupState() {
  return useQuery(setupQuery());
}

/** useBackups lists the iPhone backups this computer has already made. */
export function useBackups(enabled = true) {
  return useQuery(backupsQuery(enabled));
}

/**
 * isMissingImporter is the server saying it was built without any of this.
 *
 * The engine can be built as a reader alone, and then there is nothing to import
 * with and no point offering it. What that must not become is a wizard that offers
 * two routes and answers both with an error.
 */
export function isMissingImporter(error: unknown): boolean {
  return isArchiveError(error) && error.status === noImporter;
}

/**
 * Starting a piece of work, and what to say if the request itself was wrong.
 *
 * The two failures here are different and must not be shown the same way. A refusal
 * is the server saying the request made no sense, which is a bug in this page or a
 * field somebody left blank; the work failing arrives later as a stage of `failed`
 * with the server's own explanation and advice attached.
 */
export interface Action {
  /** True between asking and the server accepting. */
  busy: boolean;
  /** Why the request was refused, as opposed to why the work failed. */
  refused: string | undefined;
  start: (work: () => Promise<Setup>) => void;
}

/**
 * useSetupAction runs one of the four requests that start work.
 *
 * It deliberately holds nothing about the request it made. React Query's mutations
 * keep the variables they were called with until they are reset, and one of these
 * requests carries a decryption key: the way to be sure a key is not sitting in a
 * cache is for nothing to have put it there.
 */
export function useSetupAction(): Action {
  const queries = useQueryClient();
  const [busy, setBusy] = useState(false);
  const [refused, setRefused] = useState<string | undefined>(undefined);

  const start = useCallback(
    (work: () => Promise<Setup>) => {
      setBusy(true);
      setRefused(undefined);
      void work().then(
        (state) => {
          queries.setQueryData(queryKeys.state, state);
          setBusy(false);
        },
        (cause: unknown) => {
          setBusy(false);
          // Already running is not something anybody can correct. The work exists,
          // it is the work that was asked for, and the next poll shows it; saying
          // "would not accept that" about it would be a lie that invites a retry.
          if (isArchiveError(cause) && cause.status === alreadyRunning) {
            void queries.refetchQueries({ queryKey: queryKeys.state });
            return;
          }
          setRefused(saidBy(cause));
        },
      );
    },
    [queries],
  );

  return { busy, refused, start };
}

/** saidBy is the best sentence available for something that went wrong. */
function saidBy(cause: unknown): string {
  if (isArchiveError(cause)) return cause.message;
  if (cause instanceof Error) return cause.message;
  return String(cause);
}

/**
 * useClose puts the server back to holding nothing, and forgets the archive it held.
 *
 * The state is read again rather than assumed, and everything else is dropped: every
 * other query in this program is about the archive that has just been closed, and a
 * conversation list left in the cache would be the old archive's the moment a new one
 * is opened. It goes by exclusion rather than by a list of keys so that a query added
 * later is forgotten without anybody remembering to add it here.
 *
 * The order is the part worth stating. The state is re-read first and the rest is
 * dropped once it has answered, so that the panel showing the archive is on its way
 * out before its data is taken away rather than after.
 *
 * A close that fails is answered rather than swallowed: the state is read again
 * either way, and if the server still holds the archive the reader is put back in
 * front of it, which is the truth.
 */
export function useClose(): () => void {
  const queries = useQueryClient();
  return useCallback(() => {
    void closeArchive()
      .catch(() => undefined)
      .then(() => queries.refetchQueries({ queryKey: queryKeys.state }))
      .finally(() => {
        queries.removeQueries({
          predicate: (query) => query.queryKey[0] !== queryKeys.state[0],
        });
      });
  }, [queries]);
}
