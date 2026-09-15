import { describe, expect, it } from "vitest";

import type { Said } from "@/api/types";
import { translator } from "@/i18n";
import { carried, said } from "@/lib/said";

/**
 * What somebody watching a long piece of work is told.
 *
 * This is the screen a person stares at for minutes while their whole history is
 * decrypted and indexed, and it used to be the one place the program dropped back
 * into English mid-sentence: the server wrote the words and the page showed them.
 * The server now names the sentence and sends the numbers, and these tests are
 * about the two things that can go wrong with that — a name this build does not
 * know, and a number written the way the wrong language writes it.
 */
describe("saying what is happening", () => {
  const es = translator("es");
  const en = translator("en");

  it("says a named sentence in the reader's language", () => {
    const state: Said = { note: "checkingWritten", detail: "Checking every message that was written." };

    expect(said(state, "es", es)).toBe("Comprobando todos los mensajes escritos.");
    expect(said(state, "en", en)).toBe("Checking every message that was written.");
  });

  it("fills the holes in it with what the server sent", () => {
    const state: Said = {
      note: "readingBackupOf",
      detail: "Reading the backup of Ana's iPhone, made 2026-08-01.",
      values: { device: "Ana's iPhone", when: "2026-08-01" },
    };

    expect(said(state, "es", es)).toBe("Leyendo la copia de Ana's iPhone, hecha el 2026-08-01.");
  });

  /**
   * The reason the numbers travel as numbers. A Spanish reader writes 595.236 where
   * an English one writes 595,236, and the server knows neither.
   */
  it("writes the numbers the way the reader writes them", () => {
    const state: Said = {
      note: "indexedSoFar",
      detail: "Indexed 1200 conversations and 595236 messages so far.",
      values: { conversations: "1200", messages: "595236" },
      counts: [
        { of: "conversations", n: 1200 },
        { of: "messages", n: 595236 },
      ],
    };

    expect(said(state, "es", es)).toContain("595.236");
    expect(said(state, "en", en)).toContain("595,236");
  });

  /** "Moviendo 1 mensajes" is the tell of a program that was translated. */
  it("has a different sentence for one of something", () => {
    const one: Said = {
      note: "movingCount",
      detail: "Moving 1 message. Nothing is being uploaded.",
      values: { messages: "1 message" },
      counts: [{ of: "messages", n: 1 }],
    };
    const many = { ...one, counts: [{ of: "messages", n: 4 }] };

    expect(said(one, "es", es)).toBe("Moviendo un mensaje. No se sube nada.");
    expect(said(many, "es", es)).toBe("Moviendo 4 mensajes. No se sube nada.");
  });

  /**
   * A server newer than this page. Showing its English is worse than showing
   * Spanish and better than showing nothing, which is what a page that insisted on
   * knowing every name would do.
   */
  it("falls back to the server's own words for a name it does not know", () => {
    const state: Said = { note: "somethingAddedLater", detail: "Doing something new." };

    expect(said(state, "es", es)).toBe("Doing something new.");
  });

  it("shows a line that has no sentence to look up, such as a tool's own output", () => {
    expect(said({ detail: "[ 45%] /sdcard/msgstore.db.crypt15" }, "es", es)).toBe(
      "[ 45%] /sdcard/msgstore.db.crypt15",
    );
  });

  it("has nothing to say when the server has said nothing", () => {
    expect(said({}, "es", es)).toBeUndefined();
  });
});

/**
 * The migration and the export hand their progress to the wizard's working screen.
 * What is copied across is the whole sentence, not the parts somebody remembered.
 */
describe("carrying a sentence between screens", () => {
  it("takes the name, the words, the values and the numbers", () => {
    const from: Said = {
      note: "writtenSoFar",
      detail: "25 of 60 conversations written.",
      values: { written: "25", conversations: "60" },
      counts: [{ of: "written", n: 25 }],
    };

    expect(carried(from)).toEqual(from);
  });

  it("leaves absent things absent rather than passing undefined", () => {
    expect(Object.keys(carried({ detail: "Starting." }))).toEqual(["detail"]);
    expect(Object.keys(carried({}))).toEqual([]);
  });
});
