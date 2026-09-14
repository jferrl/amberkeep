/**
 * Everything the archive server sends, written down.
 *
 * These types are not a description of the API, they are the Go encoder's output:
 * `internal/export/json.go` decides the shape of a conversation and a message, and
 * `internal/api/api.go` wraps them in the envelopes below. The same code writes the
 * files an export produces, so a page and a file cannot disagree about what an
 * archive contains.
 *
 * Optionality is the part worth being pedantic about. A field Go marks `omitempty`
 * is absent when it is empty, and is optional here; a field without it is always
 * present, even when it is an empty string. That is what stops a component reading
 * something the server never sent, and it is why a boolean that is false, a number
 * that is zero and a piece of text that is empty all arrive as nothing at all.
 *
 * Arrays are `readonly` because a page of messages is what the server said, not a
 * list to sort or splice in place.
 *
 * Nothing here is checked at runtime. The program that serves this page is the
 * program that built it, so a reply that does not match these types is a bug in
 * this repository rather than something a browser should defend against.
 */

/**
 * An instant, as Go writes one: RFC 3339 in universal time.
 *
 * A timestamp is never absent when its field has no `omitempty`, which means a
 * message whose date the source database lost arrives as the year one rather than
 * as nothing. Anything rendering one has to survive that.
 */
export type Timestamp = string;

/**
 * A position in a conversation, opaque to everything but the server.
 *
 * It is two numbers joined by an underscore rather than a dash, because a message
 * dated before 1970 has a negative timestamp and a dash would take the sign off
 * instead of the halves apart.
 */
export type Cursor = string;

/** Bytes carried inside the message itself, so there is no second request. */
export type Base64 = string;

/**
 * What kind of conversation this is.
 *
 * The Go side maps anything it does not recognise onto `unknown`, so this union is
 * closed at the point the page and the binary ship together. A page served by a
 * newer binary could still meet a kind that is not here, which is why anything
 * switching on one wants a branch for the rest.
 */
export type ChatKind =
  | "direct"
  | "group"
  | "broadcast"
  | "status"
  | "newsletter"
  | "unknown";

/** What a message was: words, a file that is no longer here, or something that happened. */
export type MessageKind =
  | "text"
  | "image"
  | "video"
  | "audio"
  | "voice"
  | "document"
  | "sticker"
  | "gif"
  | "contact"
  | "location"
  | "poll"
  | "event"
  | "deleted"
  | "system"
  | "call"
  | "view-once"
  | "payment"
  | "album"
  | "invite"
  | "interactive"
  | "ignored"
  | "unknown";

/** How a call ended. */
export type CallOutcome = "connected" | "missed" | "declined" | "failed" | "unknown";

/** Somebody who belongs to a group. */
export interface Participant {
  address: string;
  /** Absent when the archive holds no name for this address, only the number. */
  name?: string;
  admin?: boolean;
}

/** One conversation. */
export interface Chat {
  id: number;
  address: string;
  kind: ChatKind;
  /** The name to show, which falls back to the address when nobody named it. */
  name: string;
  participants?: readonly Participant[];
  description?: string;
  created_at?: Timestamp;
  last_message_at?: Timestamp;
  archived?: boolean;
  message_count: number;
}

/**
 * A file that was sent, described rather than included.
 *
 * Every field is omitted when empty, so an attachment whose details the database no
 * longer holds arrives as an empty object: the message says a file was sent and
 * nothing more. The preview is the exception worth having, being often the only
 * copy of a photograph that survived.
 */
export interface Attachment {
  media_type?: string;
  file_name?: string;
  size?: number;
  duration?: string;
  width?: number;
  height?: number;
  caption?: string;
  preview_base64?: Base64;
}

/** The message this one was a reply to, as much of it as was kept. */
export interface Quote {
  sender?: string;
  sender_name?: string;
  from_me?: boolean;
  kind: MessageKind;
  text?: string;
  attachment?: Attachment;
}

export interface Reaction {
  emoji: string;
  sender?: string;
  sender_name?: string;
  from_me?: boolean;
  at?: Timestamp;
}

/**
 * Somewhere that was shared.
 *
 * The coordinates are omitted when they are zero, so a place on the equator or the
 * prime meridian loses one of them. Absent means "zero or unrecorded" here, not
 * "unrecorded".
 */
export interface Place {
  latitude?: number;
  longitude?: number;
  name?: string;
  address?: string;
  url?: string;
  live?: boolean;
}

export interface PollOption {
  name: string;
  votes: number;
}

/** A poll. Its options are null, not absent, when the reader recovered none. */
export interface Poll {
  question: string;
  options: readonly PollOption[] | null;
  closed?: boolean;
}

export interface Call {
  video: boolean;
  group?: boolean;
  outcome: CallOutcome;
  duration?: string;
}

/** A preview of a link, all of which may be missing while the message stands. */
export interface Link {
  url?: string;
  title?: string;
  description?: string;
}

export interface ContactCard {
  name?: string;
  vcard: string;
  addresses?: readonly string[];
}

export interface GroupInvite {
  group_name?: string;
  address?: string;
  expires_at?: Timestamp;
}

/**
 * Housekeeping WhatsApp did rather than anything somebody said.
 *
 * `identified` says whether the build that wrote this knew what the action code
 * meant. A false is an admission, not a guess, and the code is kept either way so
 * an unrecognised notice can be investigated rather than merely noticed.
 */
export interface Notice {
  action: number;
  text: string;
  actor?: string;
  targets?: readonly string[];
  old_value?: string;
  new_value?: string;
  identified: boolean;
}

export interface Deletion {
  at?: Timestamp;
  by?: string;
  by_admin?: boolean;
}

/**
 * One message.
 *
 * `rendered` is the message as the text export would write it, so anything that
 * only wants a readable line does not have to reimplement the wording of a missed
 * call or a withdrawn message. `text` is what somebody typed, and is absent when
 * they typed nothing.
 */
export interface Message {
  id: number;
  key?: string;
  sent_at: Timestamp;
  from_me: boolean;
  /** Absent on a message of the archive's owner, and in a conversation of two. */
  sender?: string;
  sender_name?: string;
  kind: MessageKind;
  text?: string;
  rendered: string;

  attachment?: Attachment;
  reply_to?: Quote;
  reactions?: readonly Reaction[];
  mentions?: readonly string[];
  place?: Place;
  poll?: Poll;
  call?: Call;
  link?: Link;
  contact_cards?: readonly ContactCard[];
  group_invite?: GroupInvite;
  notice?: Notice;
  deleted?: Deletion;

  album_size?: number;
  expires_after?: string;
  starred?: boolean;
  forwarded?: boolean;
  forward_score?: number;
  edited_at?: Timestamp;

  /**
   * The number the source database used for this message's type. It is carried so
   * that a kind this build does not recognise can still be traced back to what the
   * phone actually stored.
   */
  source_type: number;
}

/**
 * What the archive holds, counted once when the server started.
 *
 * `layout` names the shape of the database the archive was read from, which is the
 * reader's own vocabulary rather than a fixed set: today `modern`, `legacy`,
 * `core-data` or `unknown`, and a new reader may add to that without anything here
 * changing. `by_kind` counts conversations, not messages, despite standing beside
 * the message count.
 */
export interface Archive {
  title: string;
  layout: string;
  conversations: number;
  messages: number;
  by_kind: Partial<Record<ChatKind, number>>;
  people: number;
  named: number;
  /** False when the archive was served without an index, and searching will refuse. */
  searchable: boolean;
  time_zone: string;
  /** Absent when no conversation recorded when it began. */
  earliest?: Timestamp;
  latest?: Timestamp;
}

/** A page of the conversation list. `total` counts every match, not this page. */
export interface ChatList {
  total: number;
  chats: readonly Chat[];
}

/**
 * A page of one conversation, newest last, each page older than the one before.
 *
 * `before` is where the page before this one starts. Its absence is how the
 * beginning of a conversation is recognised.
 */
export interface MessagePage {
  chat: Chat;
  messages: readonly Message[];
  before?: Cursor;
}

/**
 * One message the search found.
 *
 * Every field is present, including a sender that is an empty string when the
 * archive recorded none. That is the opposite convention from a message, where an
 * absent sender means the archive's owner; a hit is assembled field by field in the
 * handler rather than reusing the message shape.
 */
export interface Hit {
  chat_name: string;
  chat_address: string;
  sender: string;
  from_me: boolean;
  sent_at: Timestamp;
  kind: MessageKind;
  /** The words around the match, with the matched ones wrapped in marks. See marks.ts. */
  snippet: string;
}

/** A page of search results. `total` is how many the archive holds, not how many are here. */
export interface SearchPage {
  total: number;
  hits: readonly Hit[];
}

/**
 * How far the server has got towards having an archive to read.
 *
 * `empty` is a program that has been started and pointed at nothing yet, which is
 * how somebody with a dead phone finds it. `working` is the only stage that changes
 * on its own, and the only reason anything in this page polls.
 */
export type Stage = "empty" | "working" | "ready" | "failed";

/**
 * Which part of the work is happening.
 *
 * These are stages of one job rather than a percentage, because none of them can be
 * measured honestly while it runs: decrypting knows the size of the file and nothing
 * about how far through it is, and building indexes knows neither.
 *
 * A server newer than this page may report a step this build has never heard of,
 * which is why anything turning one into a sentence has an answer for the rest.
 */
export type SetupStep =
  | "extracting"
  | "decrypting"
  | "preparing"
  | "indexing"
  | "opening";

/**
 * What the server is doing, and where it will put what it makes.
 *
 * `workspace` is present at every stage, including before anything has been chosen,
 * so that somebody can be told where files will be written before any are.
 * `detail` is one sentence and `guidance` is several lines with the line breaks
 * already in them: both are the server's own words and are rendered as text.
 */
export interface Setup {
  stage: Stage;
  workspace: string;
  step?: SetupStep;
  detail?: string;
  guidance?: string;
  archive?: Archive;
}

/**
 * One iPhone backup this computer has already made.
 *
 * Everything but the path is optional here, and that is about what an old backup
 * knows rather than about what the server sends. Apple's own index records a device
 * name, a model, an iOS version and a date, and a backup written years ago by an old
 * iTunes is missing about half of them. The server sends an empty string for each
 * one it could not read, and Go leaves an empty string out, so absent and empty are
 * the same thing and every reader treats them as one.
 *
 * `last_backup` is an instant in universal time, not a date somebody already
 * formatted. It is turned into words here, in the reader's own language and zone, by
 * the same helpers the rest of the viewer uses.
 *
 * `encrypted` is the exception, and is always sent even when it is false. It decides
 * whether a backup can be used at all, and silence is a poor way to say no.
 */
export interface Backup {
  path: string;
  device_name?: string;
  product_type?: string;
  ios_version?: string;
  last_backup?: Timestamp;
  encrypted: boolean;
}

/**
 * The backups on this computer, or why they could not be looked at.
 *
 * A `problem` is the difference between "there are none" and "I was not allowed to
 * look", and those two have to be said differently: showing an empty list for the
 * second is how somebody concludes their backups are gone. On macOS that second case
 * is the ordinary state of affairs until Full Disk Access is granted.
 *
 * The list itself is always a list, empty rather than absent, so nothing has to check
 * before iterating it. Both of those are recorded in contract/backups-refused.json.
 */
export interface BackupList {
  backups: readonly Backup[];
  problem?: string;
}
