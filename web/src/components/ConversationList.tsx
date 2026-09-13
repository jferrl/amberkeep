import { useVirtualizer } from "@tanstack/react-virtual";
import { useRef } from "react";

import type { Chat } from "@/api/types";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";
import { count, shortDate } from "@/lib/format";
import { cn } from "@/lib/utils";

/**
 * The conversation list.
 *
 * It is virtualised for the same reason the thread is: a real archive holds nearly
 * four thousand conversations, and a list that long is slow to lay out even though
 * each row is small.
 */
export function ConversationList({
  chats,
  selected,
  onSelect,
  language,
}: {
  chats: readonly Chat[];
  selected: string | undefined;
  onSelect: (chat: Chat) => void;
  language: Language;
}) {
  const t = useT();
  const viewport = useRef<HTMLDivElement>(null);

  // The compiler declines to memoise a component that calls this, because the
  // virtualiser hands back functions it cannot prove are stable, and memoising
  // around them would show stale rows. That is the right call and it costs nothing
  // here: the expensive component is Message, which is memoised by hand, and this
  // one re-renders only when the rows it is given change.
  // eslint-disable-next-line react-hooks/incompatible-library -- see above
  const virtualiser = useVirtualizer({
    count: chats.length,
    getScrollElement: () => viewport.current,
    estimateSize: () => 58,
    overscan: 10,
    getItemKey: (at) => chats[at]?.address ?? at,
  });

  if (chats.length === 0) {
    return (
      <p className="p-8 text-center text-sm text-[var(--color-muted)]">
        {t("noConversationsNamed")}
      </p>
    );
  }

  return (
    <div ref={viewport} className="min-h-0 flex-1 overflow-y-auto">
      <ul
        className="relative m-0 list-none p-0"
        style={{ height: `${String(virtualiser.getTotalSize())}px` }}
      >
        {virtualiser.getVirtualItems().map((item) => {
          const chat = chats[item.index];
          if (chat === undefined) return null;
          return (
            <li
              key={item.key}
              data-index={item.index}
              ref={virtualiser.measureElement}
              className="absolute top-0 left-0 w-full"
              style={{ transform: `translateY(${String(item.start)}px)` }}
            >
              <ConversationRow
                chat={chat}
                current={chat.address === selected}
                onSelect={onSelect}
                language={language}
              />
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function ConversationRow({
  chat,
  current,
  onSelect,
  language,
}: {
  chat: Chat;
  current: boolean;
  onSelect: (chat: Chat) => void;
  language: Language;
}) {
  const about: string[] = [];
  if (chat.kind !== "direct") about.push(chat.kind);
  const when = shortDate(chat.last_message_at, language);
  if (when !== "") about.push(when);

  return (
    <button
      type="button"
      onClick={() => {
        onSelect(chat);
      }}
      aria-current={current ? "true" : undefined}
      className={cn(
        "flex w-full items-baseline justify-between gap-3 border-b border-[var(--color-line)]",
        "px-3.5 py-2 text-left hover:bg-[var(--color-paper)]",
        current && "bg-[var(--color-paper)] shadow-[inset_3px_0_0_var(--color-accent)]",
      )}
    >
      <span className="min-w-0 break-words font-semibold">
        {chat.name}
        {about.length > 0 && (
          <span className="block text-[0.78rem] font-normal text-[var(--color-muted)]">
            {about.join(" · ")}
          </span>
        )}
      </span>
      <span className="whitespace-nowrap text-[0.8rem] text-[var(--color-muted)]">
        {count(chat.message_count, language)}
      </span>
    </button>
  );
}
