import type { Chat, Hit } from "@/api/types";
import { ConversationList } from "@/components/ConversationList";
import { SearchResults } from "@/components/SearchResults";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";
import { count } from "@/lib/format";

/**
 * What the archive holds, and how to find something in it.
 *
 * The same box does two different things depending on what the server can do. With
 * a search index it searches every message; without one it filters the conversation
 * names, because a box that looks like it searches and does not is worse than one
 * that says what it searches.
 */
export function Sidebar({
  title,
  conversations,
  messages,
  searchable,
  term,
  onTerm,
  matches,
  chats,
  hits,
  selected,
  onSelect,
  onOpenHit,
  language,
}: {
  title: string | undefined;
  conversations: number | undefined;
  messages: number | undefined;
  searchable: boolean;
  term: string;
  onTerm: (term: string) => void;
  matches: number | undefined;
  chats: readonly Chat[];
  hits: readonly Hit[];
  selected: string | undefined;
  onSelect: (chat: Chat) => void;
  onOpenHit: (hit: Hit) => void;
  language: Language;
}) {
  const t = useT();
  const label = searchable ? t("searchEverything") : t("searchNames");
  const showingResults = searchable && term.trim() !== "";

  return (
    <>
      <header className="border-b border-[var(--color-line)] px-3.5 py-3">
        <h1 className="m-0 mb-0.5 text-base font-semibold">{title ?? t("archive")}</h1>
        <p className="m-0 text-xs text-[var(--color-muted)]">
          {conversations === undefined || messages === undefined
            ? t("loading")
            : `${count(conversations, language)} ${t("conversations")} · ${count(messages, language)} ${t("messages")}`}
        </p>

        <div className="mt-2.5 flex items-center gap-2">
          <Input
            type="search"
            value={term}
            spellCheck={false}
            autoComplete="off"
            placeholder={label}
            aria-label={label}
            onChange={(event) => {
              onTerm(event.target.value);
            }}
          />
          <Button
            size="sm"
            onClick={() => {
              onTerm("");
            }}
          >
            {t("clear")}
          </Button>
        </div>

        <p className="m-0 mt-1 h-4 text-xs text-[var(--color-muted)]" aria-live="polite">
          <FoundCount showing={showingResults} matches={matches} language={language} />
        </p>
      </header>

      {showingResults ? (
        <SearchResults hits={hits} onOpen={onOpenHit} language={language} />
      ) : (
        <ConversationList
          chats={chats}
          selected={selected}
          onSelect={onSelect}
          language={language}
        />
      )}
    </>
  );
}

/** How many messages a search matched, which is usually far more than are shown. */
function FoundCount({
  showing,
  matches,
  language,
}: {
  showing: boolean;
  matches: number | undefined;
  language: Language;
}) {
  const t = useT();
  if (!showing || matches === undefined) return null;
  if (matches === 1) return <>{t("oneMatch")}</>;
  return <>{t("matches", { count: count(matches, language) })}</>;
}
