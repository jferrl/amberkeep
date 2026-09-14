import { describe, expect, it } from "vitest";

import archiveReply from "./contract/archive";
import backupsRefusedReply from "./contract/backups-refused";
import backupsReply from "./contract/backups";
import chatsReply from "./contract/chats";
import loadedMessageReply from "./contract/message-with-everything";
import messagesReply from "./contract/messages";
import searchReply from "./contract/search";
import stateEmptyReply from "./contract/state-empty";
import stateFailedReply from "./contract/state-failed";
import stateReadyReply from "./contract/state-ready";
import stateWorkingReply from "./contract/state-working";
import type { Archive, BackupList, ChatList, MessagePage, SearchPage, Setup } from "./types";

/**
 * What the server actually sends, checked against what this page believes it sends.
 *
 * `types.ts` is written by hand and nothing verifies it at runtime, because the
 * program that serves this page is the program that built it. That is the right
 * trade right up until the two halves drift apart, and they did: a backup's date
 * became an instant in universal time while this page still expected something
 * already formatted, and a list that is never absent was written down as possibly
 * absent. Both were invisible until somebody looked.
 *
 * So the files in `contract/` are recordings of the real handlers, written by
 * `internal/api/contract_test.go`, one for every reply this page can meet. Nothing
 * here is hand-edited: after changing a handler, run
 *
 *   go test ./internal/api -run TestTheRecordedRepliesStillMatch -update
 *
 * and the diff is this page's contract changing.
 *
 * Most of the work here happens in `tsc`, not in the test runner. A recording that
 * no longer satisfies its type is a compile error, which is why `npm run typecheck`
 * is the gate that matters and why the assertions below look thin: they are there so
 * the runner has something to run, and so a recording that went missing fails loudly
 * instead of quietly passing.
 */

/**
 * Fields `Actual` has that `Expected` never declared, at any depth.
 *
 * Assignment alone catches the direction that breaks this page at runtime — a field
 * removed, renamed, or changed type. It does not catch the other direction, because
 * TypeScript lets a value carry more than its type declares; and a field the server
 * started sending that nothing here knows about is how a feature quietly fails to
 * be used. So it is worth the type gymnastics to have both.
 */
type Unexpected<Expected, Actual> = Actual extends readonly (infer Element)[]
  ? Expected extends readonly (infer Declared)[]
    ? Unexpected<Declared, Element>
    : never
  : Actual extends object
    ? Expected extends object
      ?
          | Exclude<keyof Actual, keyof Expected>
          | {
              [K in Extract<keyof Actual, keyof Expected>]: Unexpected<
                NonNullable<Expected[K]>,
                NonNullable<Actual[K]>
              >;
            }[Extract<keyof Actual, keyof Expected>]
      : never
    : never;

/**
 * Asserts a recording is exactly one of our types: nothing missing, nothing extra,
 * nothing of the wrong shape, all of it at compile time.
 *
 * The constraint on `Actual` catches everything missing or of the wrong type, at
 * every depth, unions included: a conversation kind the server invented and this page
 * has never heard of fails here rather than falling through a switch at runtime.
 *
 * The rest parameter catches the other direction. It exists only when the recording
 * carries something this page never declared, so the compiler says arguments for
 * `_theServerAlsoSends` were not provided. Which field that is, is in the same
 * commit: the recording's own diff.
 */
function records<Expected>() {
  return <Actual extends Expected>(
    recording: Actual,
    // Named for the error message and never read: its presence in the signature is
    // the whole check, and reading it would mean this was called with one.
    ..._theServerAlsoSends: Unexpected<Expected, Actual> extends never
      ? []
      : [Unexpected<Expected, Actual>]
  ): Expected => recording;
}

describe("the replies this page is written against", () => {
  it("describes an archive", () => {
    const archive = records<Archive>()(archiveReply);
    expect(archive.searchable).toBe(true);
    // An archive whose conversations recorded no start date has no `earliest`, so
    // this recording is the one that pins the field's type at all.
    expect(archive.earliest).toBeTypeOf("string");
  });

  it("lists conversations", () => {
    const list = records<ChatList>()(chatsReply);
    expect(list.chats.length).toBeGreaterThan(0);
    expect(list.total).toBeGreaterThanOrEqual(list.chats.length);
  });

  it("returns a page of one conversation, and says where the page before it starts", () => {
    const page = records<MessagePage>()(messagesReply);
    expect(page.messages.length).toBeGreaterThan(0);
    // Absent at the beginning of a conversation, which is how the beginning is
    // recognised; present here, or nothing would check its type.
    expect(page.before).toBeTypeOf("string");
  });

  /**
   * No real message carries a poll and a call and a deletion at once. This one does,
   * because every optional field has to appear in some recording or nothing checks
   * its shape, and one message costs one recording instead of twenty.
   */
  it("returns a message carrying every field a message can carry", () => {
    const page = records<MessagePage>()(loadedMessageReply);
    const [message] = page.messages;
    expect(message).toBeDefined();

    const declared = [
      "key",
      "sender",
      "sender_name",
      "text",
      "attachment",
      "reply_to",
      "reactions",
      "mentions",
      "place",
      "poll",
      "call",
      "link",
      "contact_cards",
      "group_invite",
      "notice",
      "deleted",
      "album_size",
      "expires_after",
      "starred",
      "forwarded",
      "forward_score",
      "edited_at",
    ] as const;
    for (const field of declared) {
      expect(message, `${field} is declared but appears in no recording`).toHaveProperty(field);
    }
  });

  it("returns search results", () => {
    const results = records<SearchPage>()(searchReply);
    expect(results.hits.length).toBeGreaterThan(0);
  });
});

describe("the states the wizard passes through", () => {
  it("has nothing open, and still says where it will write", () => {
    const setup = records<Setup>()(stateEmptyReply);
    expect(setup.stage).toBe("empty");
    expect(setup.workspace).not.toBe("");
    expect(setup.archive).toBeUndefined();
  });

  it("says which part of the work is running, in words worth showing", () => {
    const setup = records<Setup>()(stateWorkingReply);
    expect(setup.stage).toBe("working");
    expect(setup.step).toBe("decrypting");
    expect(setup.detail).toBeTypeOf("string");
  });

  it("describes the archive the moment there is one", () => {
    const setup = records<Setup>()(stateReadyReply);
    expect(setup.stage).toBe("ready");
    expect(setup.archive?.conversations).toBeGreaterThan(0);
  });

  /** Nobody should be left at a dead end with a sentence they cannot act on. */
  it("says what went wrong and what to do about it", () => {
    const setup = records<Setup>()(stateFailedReply);
    expect(setup.stage).toBe("failed");
    expect(setup.detail).toBeTruthy();
    expect(setup.guidance).toBeTruthy();
  });
});

describe("the backups on this computer", () => {
  it("lists them, however little an old one remembers about itself", () => {
    const list = records<BackupList>()(backupsReply);
    expect(list.problem).toBeUndefined();

    // A backup written years ago by an old iTunes knows almost nothing about
    // itself. Every field absent here is one this page has to survive without.
    const sparse = list.backups.at(-1);
    expect(sparse?.path).toBeTruthy();
    expect(sparse?.device_name).toBeUndefined();
    expect(sparse?.last_backup).toBeUndefined();

    // Whether a backup can be used at all is never left to silence.
    for (const backup of list.backups) {
      expect(backup.encrypted).toBeTypeOf("boolean");
    }
  });

  /**
   * "There are none" and "I was not allowed to look" are the same emptiness and
   * completely different situations. Showing the second as the first is how somebody
   * concludes their backups are gone.
   */
  it("says when it was not allowed to look, rather than showing nothing", () => {
    const list = records<BackupList>()(backupsRefusedReply);
    expect(list.backups).toEqual([]);
    expect(list.problem).toBeTruthy();
  });
});
