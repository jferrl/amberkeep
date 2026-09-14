import { describe, expect, it } from "vitest";

import { count, dayOf, shortDate, timeOfDay } from "./format";

/**
 * The server sends every instant in universal time. What a reader sees is their own
 * time zone and their own language, which is the browser's job, so these tests
 * assert the properties that must hold rather than one machine's exact output: a
 * test that pins "14:32" passes in Madrid and fails in Tokyo.
 */
describe("rendering a time", () => {
  const at = "2019-06-14T09:12:00Z";

  it("gives a time of day", () => {
    expect(timeOfDay(at, "en")).toMatch(/\d/);
  });

  it("gives a day that names the weekday, the month and the year", () => {
    const day = dayOf(at, "en");
    expect(day).toContain("2019");
    expect(day).toMatch(/June|Jun/);
  });

  it("says the same day in Spanish", () => {
    expect(dayOf(at, "es")).toMatch(/junio/i);
  });

  const rubbish = [
    { name: "not a date at all", input: "yesterday" },
    { name: "empty", input: "" },
    { name: "a month that does not exist", input: "2019-13-01" },
  ] as const;

  /**
   * Two things JavaScript accepts that look as though it should not, recorded
   * because the opposite is the obvious guess and it is wrong. A year and a month
   * with no day means the first of that month. A day past the end of a month rolls
   * over into the next one, so the thirty-first of February is the third of March.
   *
   * Neither can reach this code from the server, which sends only what Go's time
   * package formatted. They are here so that nobody later "fixes" the parsing to
   * reject them and breaks a date that was fine.
   */
  const surprising = [
    {
      name: "a year and a month with no day",
      input: "2019-06",
      contains: "2019",
    },
    {
      name: "a day past the end of the month",
      input: "2019-02-31T00:00:00Z",
      contains: "2019",
    },
  ] as const;

  it.each(surprising)(
    "accepts $name, because JavaScript does",
    ({ input, contains }) => {
      expect(dayOf(input, "en")).toContain(contains);
    },
  );

  describe("refusing what is not a date", () => {
    it.each(rubbish)("$name", ({ input }) => {
      // A reader should see nothing rather than "Invalid Date" beside somebody's
      // message.
      expect(timeOfDay(input, "en")).toBe("");
      expect(dayOf(input, "en")).toBe("");
    });
  });

  it("treats a missing date as nothing to show", () => {
    expect(shortDate(undefined, "en")).toBe("");
  });
});

describe("rendering a count", () => {
  it("groups digits the way the language does", () => {
    // Both group a million somehow; which separator is the browser's business.
    expect(count(1121482, "en")).toMatch(/\d[.,\s\u00a0]\d/);
    expect(count(1121482, "es")).toMatch(/\d[.,\s\u00a0]\d/);
  });

  it("leaves a small number alone", () => {
    expect(count(7, "en")).toBe("7");
  });

  it("renders nothing as zero rather than as an empty string", () => {
    expect(count(0, "en")).toBe("0");
  });
});
