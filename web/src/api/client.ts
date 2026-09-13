/**
 * Talking to the archive.
 *
 * Every address here is relative, and every request carries `same-origin`
 * credentials. That is not a style preference: the server binds to the loopback
 * address and holds somebody's entire message history, so a page that could be
 * persuaded to send a request anywhere else would be the one hole worth finding. An
 * absolute address is never built, not even from something the server sent.
 *
 * Errors come back as plain text from Go's `http.Error`, not as JSON, so a failure
 * is read as text and carried in an `ArchiveError` with the status beside it. The
 * server's own sentences are already the best explanation available, and repeating
 * them is better than inventing worse ones.
 */

import type { Archive, ChatList, Cursor, MessagePage, SearchPage } from "./types";

/** Where the archive answers. Rooted, never absolute. */
const api = "/api";

/** The parameter the launch secret arrives in, named `t` by `internal/api`. */
const secretParam = "t";

/**
 * A request the archive refused or could not answer.
 *
 * The status is what tells a page which of these it is: 403 when the launch secret
 * is not this browser's, 404 for a conversation the archive does not hold, 503 when
 * the archive was served without a search index, 500 when reading it failed.
 */
export class ArchiveError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ArchiveError";
    this.status = status;
  }
}

/** isArchiveError separates a refusal by the server from a failure to reach it. */
export function isArchiveError(error: unknown): error is ArchiveError {
  return error instanceof ArchiveError;
}

/**
 * launchSecretIn finds the secret in the address the browser was opened with.
 *
 * It is a function of the query string rather than of the window so that what it
 * does is something a test can state.
 */
export function launchSecretIn(search: string): string {
  return new URLSearchParams(search).get(secretParam) ?? "";
}

/**
 * The secret this page was launched with, held only until it has been used.
 *
 * It arrives once in the address the browser was opened with, and the server answers
 * the first request carrying it by setting a cookie every later request travels on.
 * So it is read once here and dropped as soon as a reply arrives: written to storage
 * it would outlive the archive being open, and put into a link it would end up in
 * whatever the browser remembers about where somebody has been.
 *
 * Usually there is nothing to find, because the server redirects the first page load
 * to an address without it. The case that matters is development, where Vite serves
 * the page and only the requests are proxied, so no redirect ever happens.
 */
let launchSecret = launchSecretIn(typeof window === "undefined" ? "" : window.location.search);

/** What every request accepts, so a page leaving can stop asking. */
export interface Cancellable {
  signal?: AbortSignal | undefined;
}

/**
 * narrow builds a query string, leaving out whatever the caller did not ask for.
 *
 * An empty value is left out rather than sent empty. The server reads a missing
 * parameter and an empty one as the same thing, and leaving it out is what makes the
 * address of a conversation's most recent page identical every time it is opened,
 * which is worth having in a cache key and in a network panel.
 */
function narrow(values: Record<string, string | number | undefined>): URLSearchParams {
  const params = new URLSearchParams();
  for (const [name, value] of Object.entries(values)) {
    if (value === undefined || value === "") continue;
    params.set(name, typeof value === "number" ? String(value) : value);
  }
  return params;
}

/**
 * ask makes the request and turns anything but an answer into an ArchiveError.
 *
 * The reply is not validated against the type it is asked for. The program that
 * serves this page is the program that built it, so a reply that does not match is a
 * bug in this repository rather than something to defend a browser against.
 */
async function ask<T>(path: string, params: URLSearchParams, options: Cancellable): Promise<T> {
  if (launchSecret !== "") params.set(secretParam, launchSecret);

  const query = params.toString();
  const response = await fetch(query === "" ? path : `${path}?${query}`, {
    credentials: "same-origin",
    headers: { Accept: "application/json" },
    signal: options.signal ?? null,
  });
  // The reply carries the cookie the rest of this session travels on, so the copy
  // held here has done its work whether the server liked it or not.
  launchSecret = "";

  if (!response.ok) {
    throw new ArchiveError(response.status, await explanation(response));
  }

  let body: unknown;
  try {
    body = await response.json();
  } catch (cause) {
    // A reader who has left is not a broken archive, and must stay a cancellation
    // rather than becoming an error a page would show somebody.
    if (options.signal?.aborted === true) throw cause;
    throw new ArchiveError(response.status, "the archive sent a reply this page could not read");
  }
  return body as T;
}

/** explanation is what the server said, or the honest absence of it. */
async function explanation(response: Response): Promise<string> {
  const said = (await response.text().catch(() => "")).trim();
  if (said !== "") return said;
  return `the archive answered ${String(response.status)} and said nothing`;
}

/** getArchive reports what the archive holds. */
export function getArchive(options: Cancellable = {}): Promise<Archive> {
  return ask<Archive>(`${api}/archive`, narrow({}), options);
}

export interface ChatsQuery extends Cancellable {
  /** Words to look for in a conversation's name. */
  q?: string | undefined;
  /** How many to return. The server sends 200 unasked and will not exceed 1000. */
  limit?: number | undefined;
  offset?: number | undefined;
}

/** getChats lists conversations, most recently used first. */
export function getChats({ q, limit, offset, signal }: ChatsQuery = {}): Promise<ChatList> {
  return ask<ChatList>(`${api}/chats`, narrow({ q, limit, offset }), { signal });
}

export interface MessagesQuery extends Cancellable {
  /** Where to read backwards from. Absent asks for the end of the conversation. */
  before?: Cursor | undefined;
  /** How many to return. The server sends 60 unasked and will not exceed 500. */
  limit?: number | undefined;
}

/** getMessages returns one page of a conversation, ending at a position. */
export function getMessages(
  jid: string,
  { before, limit, signal }: MessagesQuery = {},
): Promise<MessagePage> {
  // The address is a path segment, so it is encoded rather than pasted in. A direct
  // conversation's address holds an at sign, a number kept in international form
  // holds a plus, and neither survives a URL untouched.
  const path = `${api}/chats/${encodeURIComponent(jid)}/messages`;
  return ask<MessagePage>(path, narrow({ before, limit }), { signal });
}

export interface SearchQuery extends Cancellable {
  /** The address of a single conversation to look in, or nothing for all of them. */
  chat?: string | undefined;
  /** How many to return. The server sends 50 unasked and will not exceed 500. */
  limit?: number | undefined;
  offset?: number | undefined;
}

/** search finds messages anywhere in the archive. */
export function search(q: string, { chat, limit, offset, signal }: SearchQuery = {}): Promise<SearchPage> {
  return ask<SearchPage>(`${api}/search`, narrow({ q, chat, limit, offset }), { signal });
}
