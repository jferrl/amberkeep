/**
 * What the program wrote, set as prose rather than as output.
 *
 * These sentences are written in a Go file and printed in a terminal, so they arrive
 * hard-wrapped at about seventy characters with blank lines between paragraphs. Shown
 * as they are, in a box, they read as a program's output rather than as something
 * written for the person in front of it — ragged right edges that have nothing to do
 * with the width of the screen they are on.
 *
 * So the wrapping is undone and the paragraphs are paragraphs. What is indented stays
 * as it is: a path, a command, a menu to follow, a list — the places where the line
 * breaks are the meaning rather than an artefact of the width somebody wrote at.
 */
export function Prose({ text }: { text: string }) {
  const blocks = text.split(/\n{2,}/).filter((block) => block.trim() !== "");

  return (
    <div className="flex flex-col gap-2.5">
      {blocks.map((block) =>
        laidOut(block) ? (
          <pre
            key={block}
            className="m-0 overflow-x-auto font-sans text-sm whitespace-pre"
          >
            {block}
          </pre>
        ) : (
          <p key={block} className="m-0 text-sm">
            {unwrapped(block)}
          </p>
        ),
      )}
    </div>
  );
}

/**
 * laidOut reports whether a block's own line breaks carry meaning.
 *
 * Indentation is the signal, because it is the only one these sentences use: a
 * command to type, a menu to follow, the items of a list. A paragraph never has it.
 */
function laidOut(block: string): boolean {
  return block.split("\n").some((line) => /^\s\s+\S/.test(line));
}

/** unwrapped joins the lines a paragraph was broken into to fit a terminal. */
function unwrapped(block: string): string {
  return block
    .split("\n")
    .map((line) => line.trim())
    .join(" ");
}
