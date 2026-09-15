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
  "direct" | "group" | "broadcast" | "status" | "newsletter" | "unknown";

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
export type CallOutcome =
  "connected" | "missed" | "declined" | "failed" | "unknown";

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
  /**
   * What this archive is called, when the server has a name for it.
   *
   * Usually absent. It used to arrive as the word "Archive" whatever the reader's
   * language, because the server defaulted it; now a server that has a real name
   * sends one and a server that does not says nothing, which lets this page use its
   * own word.
   */
  title?: string;
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
  | "opening"
  /** Producing files from an archive that is already open. */
  | "writing";

/**
 * A number the work has reached, for this page to phrase.
 *
 * The sentence used to arrive from the server already written, which made every
 * progress line English and every number unformatted: a Spanish reader watching an
 * index build was told about "595236 messages" rather than 595.236. The server knows
 * the number, this page knows the reader.
 */
export interface Count {
  of: string;
  n: number;
}

/**
 * A sentence the server wants said, named rather than only written out.
 *
 * `note` is what this page looks the sentence up by, so somebody who has been
 * reading Spanish for twenty minutes is not handed English at the one point where
 * the program goes quiet for several minutes over their whole history. `values` is
 * what fills its holes — a device, a date, a size — and `counts` are the numbers in
 * it, left as numbers, because 595236 is not how a Spanish reader writes 595.236.
 *
 * `detail` is the same sentence in English, already filled in. It is not dead
 * weight: the command prints exactly that, and a page meeting a name from a newer
 * server has to say something rather than nothing.
 */
/**
 * What to say about a failure somebody can act on.
 *
 * Every failure worth advising on carries an identifier from the part of the program
 * that raised it, and this is what that identifier means. Fetched rather than written
 * here: the same words are printed by the command, and a copy on this side would
 * drift from that one the first time anybody corrected a sentence.
 */
export interface Advice {
  /** title is the one line, read at a glance, instead of the error's own words. */
  title: string;
  /** body is the several lines under it, line breaks already in them. */
  body: string;
}

/**
 * A sentence the program named rather than wrote out.
 *
 * `note` is what the guide's catalogue is keyed by, `values` fills its holes, and
 * `text` is the same sentence in English for a name this build has never met. The
 * words themselves arrive with the guide, in the language this page asked for.
 */
export interface Spoken {
  note?: string;
  text?: string;
  values?: Readonly<Record<string, string>>;
}

/** AdviceOnFailure is every piece of advice, by the identifier a failure carries. */
export type AdviceOnFailure = Readonly<Record<string, Advice>>;

export interface Said {
  note?: string;
  detail?: string;
  values?: Readonly<Record<string, string>>;
  counts?: readonly Count[];
}

/**
 * What the server is doing, and where it will put what it makes.
 *
 * `workspace` is present at every stage, including before anything has been chosen,
 * so that somebody can be told where files will be written before any are.
 * `guidance` names the advice that goes with a failure rather than carrying its
 * words: the words come from /api/advice, in the language this page asked for.
 */
export interface Setup extends Said {
  stage: Stage;
  workspace: string;
  step?: SetupStep;
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

  /**
   * Where the program looked, which is how an empty list is read correctly.
   *
   * Empty means there was nowhere to look rather than nothing to find: Apple ships
   * no Finder, iTunes or Apple Devices for Linux, so a page told only that the list
   * was empty would go on to explain how to make a backup in Finder.
   */
  looked?: readonly string[];
}

/**
 * How far along a migration is.
 *
 * Nothing moves from one of these to the next on its own. Checking does not begin
 * planning, and a plan does not begin writing: each ends and waits to be asked for
 * the next, because the thing at the end is somebody restoring a backup onto a phone
 * they depend on, and a flow that carries people along is one they can reach the end
 * of without having read anything.
 */
export type MigrationStage =
  | "idle"
  | "checking"
  | "checked"
  | "planning"
  | "planned"
  | "working"
  | "done"
  | "failed";

/**
 * One thing that was checked before anything else happens.
 *
 * `blocking` separates what has to be put right from what is worth knowing. The one
 * that matters most — whether a safety backup was made and archived — can never pass,
 * because nothing on this computer can see whether somebody did it. It is raised every
 * time rather than left out for being uncheckable.
 */
export interface Finding {
  /** The guided step that says what to do about it. */
  step: string;
  /**
   * `check` names the sentence saying what was looked at and `note` the one saying
   * what was found; `values` fills their holes. The guide carries both sentences in
   * the reader's own language, and `title` and `detail` are the same two in English,
   * for a name this build has never met.
   */
  check?: string;
  note?: string;
  values?: Readonly<Record<string, string>>;
  title: string;
  passed: boolean;
  blocking: boolean;
  detail?: string;
}

/** Everything that was checked, and how much room a copy of the backup will take. */
export interface Readiness {
  findings: readonly Finding[];
  needs?: number;
}

/**
 * What would happen to one conversation.
 *
 * `adding`, `already_there` and `untranslatable` account for every message in it: the
 * three add up to its total, so a missing message is a bug rather than a rounding.
 * `into` is the conversation on the iPhone this would join, and its absence means one
 * would be created.
 */
export interface MigrationConversation {
  address: string;
  name: string;
  kind: string;
  /** The address the iPhone will file this under, which is not always its own. */
  destination: string;
  into?: string;
  session?: number;
  /** Other conversations that turn out to be the same person and are written into this one. */
  folded?: readonly string[];

  adding: number;
  already_there: number;
  untranslatable: number;
  as_placeholders: number;
  on_phone_already: number;

  earliest?: Timestamp;
  latest?: Timestamp;
  /** Why nothing would happen to it. Empty when something would. */
  skipped?: Spoken;
}

/**
 * What a migration would do, worked out without doing any of it.
 *
 * This is the thing somebody reads and agrees to, so every number in it is one they
 * could act on. `warnings` are the limitations in words rather than in counts, and
 * include the one failure that looks exactly like success: somebody the two phones
 * know by different names arriving twice.
 */
export interface MigrationPlan {
  conversations: readonly MigrationConversation[];

  adding: number;
  already_there: number;
  untranslatable: number;
  as_placeholders: number;

  merging: number;
  creating: number;
  untouched: number;

  warnings?: readonly Spoken[];
  earliest?: Timestamp;
  latest?: Timestamp;
}

/** A finished migration: a backup on disk, and a phone that has not been touched. */
export interface Migrated {
  /** The changed copy, ready for Finder to restore. */
  backup: string;
  added: number;
  merged: number;
  created: number;
  /** How many consistency checks the result passed. */
  checks: number;
  /** How many files the copied backup holds, unchanged from the original. */
  files: number;
}

/** How far along a migration is, and everything it has worked out so far. */
export interface Migration extends Said {
  stage: MigrationStage;
  step?: SetupStep;
  guidance?: string;
  backup?: string;
  checks?: Readiness;
  plan?: MigrationPlan;
  result?: Migrated;
}

/**
 * One thing somebody has to do or to know, and when.
 *
 * `critical` marks a step that loses something irreversibly if it is skipped. Three of
 * them do, and they are shown differently for that reason; marking anything else would
 * leave the mark meaning nothing. `body` and `expect` carry their own line breaks and
 * are rendered as text, never as markup.
 */
export interface GuideStep {
  id: string;
  title: string;
  body: string;
  expect?: string;
  minutes?: number;
  critical?: boolean;
}

/** The steps of one part of the business, in the order they are needed. */
export interface GuideStage {
  stage: "before" | "restoring" | "after" | "wrong";
  heading: string;
  steps: readonly GuideStep[];
}

/**
 * What somebody has to be told, served by the program rather than written into this
 * page. Those sentences live in one place; a copy here would drift from the copy in Go
 * the first time anybody corrected one.
 */
export interface Guide {
  stages: readonly GuideStage[];
  /**
   * Everything the migration screens name rather than write out: what a check
   * looked at and what it found, why a conversation is not moving, what has to be
   * read before agreeing.
   *
   * Sent with the steps because they are read on the same screens and come from the
   * same catalogue.
   */
  sentences?: Readonly<Record<string, string>>;
}

/** How far along writing an archive out is. */
export type ExportStage = "idle" | "writing" | "done" | "failed";

/** What an export produced. */
export interface Exported {
  /** The folder to go and look in. */
  into: string;
  conversations: number;
  messages: number;
  bytes: number;
  formats: readonly string[];
}

/**
 * Writing the archive out.
 *
 * The same shape as the import and the migration: work runs on the server, one thing
 * says how far along it is, and this page asks that one thing. An archive of a
 * million messages takes about a minute, which is long enough that silence would
 * look like a program that had stopped.
 */
export interface Export extends Said {
  stage: ExportStage;
  step?: SetupStep;
  guidance?: string;
  result?: Exported;
}

/** The ways an archive can be written out. */
export type Format = "html" | "text" | "json";

/** A phone this computer can see. */
export interface Phone {
  serial: string;
  name: string;
  /**
   * Whether it can be read yet.
   *
   * A phone that is plugged in but has not had the prompt accepted is visible and
   * useless. Saying which it is, is the difference between somebody tapping "allow"
   * and concluding the program does not work.
   */
  ready: boolean;
  trouble?: string;
}

/** An encrypted message store on a phone. */
export interface PhoneBackup {
  path: string;
  name: string;
  size: number;
  /**
   * A fragment of a later backup: useless on its own, and sitting in the same folder
   * looking almost identical to the one that is not. The guide spends a paragraph
   * warning about these; naming them is cheaper.
   */
  partial: boolean;
}

/**
 * Why no phone can be seen.
 *
 * Two different situations. Tools that were never installed are something somebody
 * can go and fix; tools that are installed and will not run are not fixed by
 * installing them again, and telling them to would waste their evening.
 */
export type Why = "no-tools" | "unusable";

/** What this computer can see, and why it can see nothing when it cannot. */
export interface Phones {
  phones: readonly Phone[];
  /** Why there is no list rather than an empty one — usually no Android tools. */
  trouble?: string;
  why?: Why;
  /** Which system this is, so the instruction is the one that belongs to it. */
  platform?: string;
}
