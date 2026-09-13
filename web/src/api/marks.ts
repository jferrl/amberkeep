/**
 * The marks a search puts around the words that matched.
 *
 * The index wraps a match in two control characters rather than in markup, and this
 * turns them into pieces a component renders as elements. That indirection is the
 * whole point: a message is words somebody else wrote, and if a snippet arrived as
 * HTML then a message containing markup would become markup. Here it cannot, because
 * nothing that leaves this file is anything but text.
 *
 * The characters are the ones `search.MarkOpen` and `search.MarkClose` name in Go.
 * They are control characters because no message contains one, and deliberately not
 * the null character: SQLite builds its snippets with C string handling and silently
 * drops a null mark, which would lose the opening of every match while leaving the
 * closing in place.
 */

export const markOpen = "";
export const markClose = "";

/** A run of a snippet that either matched what was searched for or did not. */
export interface SnippetPiece {
  text: string;
  marked: boolean;
}

/**
 * splitSnippet breaks a snippet into its matched and unmatched runs.
 *
 * A snippet is read rather than split on a separator so that marks which do not pair
 * up are survivable, which matters because a snippet is a fragment of a longer message
 * and a fragment can begin or end inside a match. An opening with no closing marks the
 * rest of the snippet; a closing with no opening is dropped, having nothing to close.
 * Empty runs are never returned, so a mark around nothing costs a reader nothing.
 *
 * Iteration is by code point rather than by index, so an emoji at the edge of a match
 * is not cut in half.
 */
export function splitSnippet(snippet: string): SnippetPiece[] {
  const pieces: SnippetPiece[] = [];
  let text = "";
  let marked = false;

  const keep = (): void => {
    if (text !== "") pieces.push({ text, marked });
    text = "";
  };

  for (const character of snippet) {
    if (character === markOpen) {
      keep();
      marked = true;
      continue;
    }
    if (character === markClose) {
      keep();
      marked = false;
      continue;
    }
    text += character;
  }
  keep();

  return pieces;
}
