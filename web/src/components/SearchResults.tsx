import type { Hit } from "@/api/types";
import { splitSnippet } from "@/api/marks";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";
import { shortDate } from "@/lib/format";

/**
 * What a search across the whole archive found.
 *
 * The server marks the words that matched with two control characters rather than
 * with markup, and they are turned into elements here. That is the point: a message
 * is text somebody else wrote, and it never becomes markup on its way to a browser,
 * so a search result cannot be made to carry anything but words.
 */
export function SearchResults({
  hits,
  onOpen,
  language,
}: {
  hits: readonly Hit[];
  onOpen: (hit: Hit) => void;
  language: Language;
}) {
  const t = useT();

  if (hits.length === 0) {
    return (
      <p className="p-8 text-center text-sm text-[var(--color-muted)]">
        {t("nothingFound")}
      </p>
    );
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto py-1">
      {hits.map((hit) => (
        <button
          key={`${hit.chat_address}-${hit.sent_at}`}
          type="button"
          onClick={() => {
            onOpen(hit);
          }}
          className="w-full border-b border-[var(--color-line)] px-2 py-2 text-left hover:bg-[var(--color-surface)]"
        >
          <div>
            <span className="text-[0.8rem] font-semibold text-[var(--color-accent)]">
              {hit.chat_name}
            </span>
            <time
              className="ml-1.5 text-[0.72rem] text-[var(--color-muted)]"
              dateTime={hit.sent_at}
            >
              {shortDate(hit.sent_at, language)}
            </time>
          </div>
          <div className="mt-px break-words">
            <Snippet text={hit.snippet} />
          </div>
        </button>
      ))}
    </div>
  );
}

/** Snippet renders the matched words as elements, never as markup. */
function Snippet({ text }: { text: string }) {
  return (
    <>
      {splitSnippet(text).map((piece, at) => {
        const key = `${String(at)}-${piece.text.slice(0, 8)}`;
        return piece.marked ? (
          <mark
            key={key}
            className="rounded-sm bg-[var(--color-mark)] text-inherit"
          >
            {piece.text}
          </mark>
        ) : (
          <span key={key}>{piece.text}</span>
        );
      })}
    </>
  );
}
