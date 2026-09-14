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

import type {
  Archive,
  BackupList,
  ChatList,
  Cursor,
  MessagePage,
  SearchPage,
  Setup,
} from "./types";

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

/** addressOf joins a path to its parameters, while there is still a secret to attach. */
function addressOf(path: string, params: URLSearchParams): string {
  if (launchSecret !== "") params.set(secretParam, launchSecret);
  const query = params.toString();
  return query === "" ? path : `${path}?${query}`;
}

/**
 * answer turns anything but a readable reply into an ArchiveError.
 *
 * The reply is not validated against the type it is asked for. The program that
 * serves this page is the program that built it, so a reply that does not match is a
 * bug in this repository rather than something to defend a browser against.
 */
async function answer<T>(response: Response, options: Cancellable): Promise<T> {
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

/** ask reads something the server already knows. */
async function ask<T>(path: string, params: URLSearchParams, options: Cancellable): Promise<T> {
  const response = await fetch(addressOf(path, params), {
    credentials: "same-origin",
    headers: { Accept: "application/json" },
    signal: options.signal ?? null,
  });
  // The reply carries the cookie the rest of this session travels on, so the copy
  // held here has done its work whether the server liked it or not.
  launchSecret = "";

  return answer<T>(response, options);
}

/**
 * tell starts a piece of work and hands back the reply without reading it.
 *
 * Everything it sends travels in a JSON body, never in the query string. One of
 * these requests carries a decryption key, and a query string is the part of a
 * request that survives: in a server log, in the browser's history, in the address
 * bar of a screenshot somebody sends asking for help. Nothing secret is ever a
 * parameter here, and keeping that true of all four is what stops the fifth one
 * being written the other way.
 */
async function tell(path: string, body: unknown, options: Cancellable): Promise<Response> {
  const response = await fetch(addressOf(path, new URLSearchParams()), {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(body),
    signal: options.signal ?? null,
  });
  launchSecret = "";
  return response;
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

/**
 * getState reports what the server is doing and where it will write what it makes.
 *
 * This is the one thing here that changes while somebody watches it, and the only
 * reason the page polls anything at all.
 */
export function getState(options: Cancellable = {}): Promise<Setup> {
  return ask<Setup>(`${api}/state`, narrow({}), options);
}

/** getBackups lists the iPhone backups this computer has already made. */
export function getBackups(options: Cancellable = {}): Promise<BackupList> {
  return ask<BackupList>(`${api}/backups`, narrow({}), options);
}

/**
 * What every one of the three ways in may carry besides the file itself.
 *
 * Both are left out rather than sent empty. `into` absent means the workspace the
 * server already showed, and sending one changes it for good, so a page that sent
 * the value it was displaying would be reasserting a choice nobody made. `contacts`
 * absent means the archive keeps the numbers it has.
 */
export interface Extras {
  /** Where files should be written. Absent accepts the workspace `state` showed. */
  into?: string | undefined;
  /**
   * One address book, as a path: a .vcf, or WhatsApp's own wa.db.
   *
   * Which of the two it is, is the server's problem and not the reader's. Nobody
   * whose phone has died should have to know what wa.db is to get their mother's
   * name back onto a conversation, so this is one field and never a choice.
   */
  contacts?: string | undefined;
}

/**
 * filled keeps the fields somebody actually gave and drops the rest.
 *
 * Written out a field at a time rather than walked over, because walking an
 * interface loses the types of its values and the whole point of this file is that
 * nothing untyped is put into a request body.
 */
function filled({ into, contacts }: Extras): Record<string, string> {
  const body: Record<string, string> = {};
  if (into !== undefined && into !== "") body.into = into;
  if (contacts !== undefined && contacts !== "") body.contacts = contacts;
  return body;
}

/**
 * openFile points the server at a file somebody already has.
 *
 * The server works out whether it is an Android database or an iPhone one; this
 * page does not guess from the name, because a file somebody copied and renamed is
 * exactly the case that has to keep working.
 */
export async function openFile(
  path: string,
  extras: Extras = {},
  options: Cancellable = {},
): Promise<Setup> {
  return answer<Setup>(await tell(`${api}/open`, { path, ...filled(extras) }, options), options);
}

/** extract takes the messages out of an iPhone backup and puts them in a folder. */
export async function extract(
  backup: string,
  extras: Extras = {},
  options: Cancellable = {},
): Promise<Setup> {
  return answer<Setup>(
    await tell(`${api}/extract`, { backup, ...filled(extras) }, options),
    options,
  );
}

/** What decrypt needs, named rather than ordered so the key cannot be passed by mistake. */
export interface Decryption extends Extras {
  /** Where the msgstore.db.crypt15 copied off the phone is. */
  file: string;
  /**
   * The 64-digit key, or the path of a file holding it.
   *
   * Either way it exists in this program for the length of one request. It is never
   * a query parameter, never stored, and never put anywhere an error message could
   * pick it up. The component that collected it drops it the moment this is called.
   */
  key: string;
}

/** decrypt unlocks an Android backup into a folder. */
export async function decrypt(
  { file, key, ...extras }: Decryption,
  options: Cancellable = {},
): Promise<Setup> {
  return answer<Setup>(
    await tell(`${api}/decrypt`, { file, key, ...filled(extras) }, options),
    options,
  );
}

/**
 * closeArchive puts the server back to holding nothing.
 *
 * The reply is not read. What matters is that the server has let go, and what is
 * true afterwards is whatever the next `getState` says rather than whatever this
 * one claimed.
 */
export async function closeArchive(options: Cancellable = {}): Promise<void> {
  const response = await tell(`${api}/close`, {}, options);
  if (!response.ok) {
    throw new ArchiveError(response.status, await explanation(response));
  }
}
