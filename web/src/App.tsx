import { useCallback, useDeferredValue, useState } from "react";

import type { Chat, Hit } from "@/api/types";
import { useArchive, useChats, useSearch } from "@/api/queries";
import { Sidebar } from "@/components/Sidebar";
import { Thread, ThreadHeader } from "@/components/Thread";
import { Button } from "@/components/ui/button";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";
import { cn } from "@/lib/utils";

/**
 * The archive, in two panes: what there is on the left, what is in it on the right.
 *
 * Everything is read from the program that served this page and from nowhere else.
 * There is no account, nothing is uploaded, and closing the program ends the
 * server, which is the whole reason it can be trusted with somebody's entire
 * message history.
 */
export function App({ language }: { language: Language }) {
  const [term, setTerm] = useState("");
  const [chat, setChat] = useState<Chat | undefined>(undefined);

  // Typing must not wait on a search across a million messages. The results follow
  // the deferred value, so the box stays responsive while they catch up.
  const searching = useDeferredValue(term);

  const archive = useArchive();
  const searchable = archive.data?.searchable ?? false;

  // Without a search index only the names can be searched, so the term filters the
  // conversation list instead of querying the archive.
  const chats = useChats(searchable ? "" : searching);
  const results = useSearch(searchable ? searching : "");

  const openHit = useCallback(
    (hit: Hit) => {
      const found = chats.data?.chats.find((c) => c.address === hit.chat_address);
      if (found !== undefined) setChat(found);
    },
    [chats.data?.chats],
  );

  const reading = chat !== undefined;

  return (
    <div className="grid h-dvh grid-cols-1 md:grid-cols-[20rem_1fr]">
      <nav
        className={cn(
          "flex min-h-0 flex-col border-r border-[var(--color-line)] bg-[var(--color-panel)]",
          reading && "hidden md:flex",
        )}
      >
        <Sidebar
          title={archive.data?.title}
          conversations={archive.data?.conversations}
          messages={archive.data?.messages}
          searchable={searchable}
          term={term}
          onTerm={setTerm}
          matches={results.data?.total}
          chats={chats.data?.chats ?? []}
          hits={results.data?.hits ?? []}
          selected={chat?.address}
          onSelect={setChat}
          onOpenHit={openHit}
          language={language}
        />
      </nav>

      <section className={cn("flex min-h-0 min-w-0 flex-col", !reading && "hidden md:flex")}>
        {chat === undefined ? (
          <Nothing />
        ) : (
          <Reading
            chat={chat}
            language={language}
            onBack={() => {
              setChat(undefined);
            }}
          />
        )}
      </section>
    </div>
  );
}

/** What the reading pane shows before a conversation has been chosen. */
function Nothing() {
  const t = useT();
  return (
    <>
      <header className="border-b border-[var(--color-line)] bg-[var(--color-panel)] px-4 py-2.5">
        <h2 className="m-0 text-base font-semibold">{t("pickAConversation")}</h2>
      </header>
      <p className="flex-1 p-12 text-center text-[var(--color-muted)]">{t("pickToStart")}</p>
    </>
  );
}

/**
 * One conversation, with the way back that a narrow screen needs.
 *
 * On a wide screen both panes are visible and the way back is the list itself, so
 * the button is hidden rather than moved: a control that changes position between
 * sizes is one people have to find twice.
 */
function Reading({
  chat,
  language,
  onBack,
}: {
  chat: Chat;
  language: Language;
  onBack: () => void;
}) {
  const t = useT();

  return (
    <>
      <div className="flex items-center gap-2 md:contents">
        <Button variant="quiet" size="sm" className="ml-2 md:hidden" onClick={onBack}>
          {t("backToList")}
        </Button>
        <div className="min-w-0 flex-1">
          <ThreadHeader chat={chat} language={language} />
        </div>
      </div>
      <Thread key={chat.address} chat={chat} language={language} />
    </>
  );
}
