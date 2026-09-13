import { describe, expect, it } from "vitest";

import type { SnippetPiece } from "./marks";
import { markClose, markOpen, splitSnippet } from "./marks";

/**
 * A snippet is a fragment of a longer message, so it can begin or end inside a
 * match. Every case below that is not simply "a match in the middle" exists because
 * of that, and the marks arriving in pairs is the one thing that cannot be assumed.
 */
const cases: { name: string; snippet: string; want: SnippetPiece[] }[] = [
  {
    name: "a snippet with nothing marked is one run of ordinary words",
    snippet: "we walked to the beach",
    want: [{ text: "we walked to the beach", marked: false }],
  },
  {
    name: "a matched word becomes a run of its own, keeping the spaces around it",
    snippet: `we walked to the ${markOpen}beach${markClose} again`,
    want: [
      { text: "we walked to the ", marked: false },
      { text: "beach", marked: true },
      { text: " again", marked: false },
    ],
  },
  {
    name: "a snippet beginning inside a match opens with the match",
    snippet: `${markOpen}beach${markClose} again`,
    want: [
      { text: "beach", marked: true },
      { text: " again", marked: false },
    ],
  },
  {
    name: "an opening with no closing marks the rest of the snippet",
    snippet: `we walked to the ${markOpen}beach again`,
    want: [
      { text: "we walked to the ", marked: false },
      { text: "beach again", marked: true },
    ],
  },
  {
    name: "a closing with no opening ends a run rather than marking backwards",
    snippet: `we walked to the beach${markClose} again`,
    want: [
      { text: "we walked to the beach", marked: false },
      { text: " again", marked: false },
    ],
  },
  {
    name: "a second opening inside a match starts another matched run",
    snippet: `${markOpen}beach${markOpen}front${markClose}`,
    want: [
      { text: "beach", marked: true },
      { text: "front", marked: true },
    ],
  },
  {
    name: "two matches in one snippet stay apart",
    snippet: `${markOpen}beach${markClose} at ${markOpen}dawn${markClose}`,
    want: [
      { text: "beach", marked: true },
      { text: " at ", marked: false },
      { text: "dawn", marked: true },
    ],
  },
  {
    name: "a mark around nothing produces nothing to render",
    snippet: `${markOpen}${markClose}`,
    want: [],
  },
  {
    name: "an empty snippet has nothing to render",
    snippet: "",
    want: [],
  },
  {
    name: "the ellipsis the index puts around a fragment is ordinary text",
    snippet: `…to the ${markOpen}beach${markClose}…`,
    want: [
      { text: "…to the ", marked: false },
      { text: "beach", marked: true },
      { text: "…", marked: false },
    ],
  },
  {
    name: "an emoji at the edge of a match is not cut in half",
    snippet: `${markOpen}🏖️${markClose} tomorrow`,
    want: [
      { text: "🏖️", marked: true },
      { text: " tomorrow", marked: false },
    ],
  },
];

describe("splitSnippet", () => {
  it.each(cases)("$name", ({ snippet, want }) => {
    expect(splitSnippet(snippet)).toEqual(want);
  });

  it.each(cases)("keeps every word of: $name", ({ snippet }) => {
    const withoutMarks = snippet.replaceAll(markOpen, "").replaceAll(markClose, "");
    expect(splitSnippet(snippet).map((piece) => piece.text).join("")).toBe(withoutMarks);
  });

  it.each(cases)("lets no mark reach a reader: $name", ({ snippet }) => {
    for (const piece of splitSnippet(snippet)) {
      expect(piece.text).not.toContain(markOpen);
      expect(piece.text).not.toContain(markClose);
    }
  });
});
