import { useVirtualizer } from "@tanstack/react-virtual";
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef } from "react";

import type { Chat, Message as ArchivedMessage } from "@/api/types";
import { useMessages } from "@/api/queries";
import { Message } from "@/components/Message";
import { Button } from "@/components/ui/button";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";
import { count, dayOf } from "@/lib/format";

/**
 * One conversation, read the way somebody reads one: it opens at the end and grows
 * upwards.
 *
 * The largest conversation in a real archive is ninety thousand messages. Two things
 * make that work. The server is asked for sixty at a time rather than all of them,
 * paging backwards on a cursor. And only the rows near the scroll position exist in
 * the document, which is what the virtualiser is for: without it the browser lays
 * out ninety thousand bubbles and stops responding.
 */
export function Thread({ chat, language }: { chat: Chat; language: Language }) {
  const t = useT();
  const viewport = useRef<HTMLDivElement>(null);
  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isPending, isError } =
    useMessages(chat.address);

  // The server returns each page newest-last and each subsequent page older, so the
  // pages are reversed to read the conversation forwards.
  const rows = useMemo(() => toRows(data?.pages ?? [], language), [data?.pages, language]);

  // The compiler declines to memoise a component that calls this, because the
  // virtualiser hands back functions it cannot prove are stable, and memoising
  // around them would show stale rows. That is the right call and it costs nothing
  // here: the expensive component is Message, which is memoised by hand, and this
  // one re-renders only when the rows it is given change.
  // eslint-disable-next-line react-hooks/incompatible-library -- see above
  const virtualiser = useVirtualizer({
    count: rows.length,
    getScrollElement: () => viewport.current,
    estimateSize: () => 64,
    overscan: 12,
    getItemKey: (at) => rows[at]?.key ?? at,
  });

  // Opening a conversation puts the reader at its end, where the last thing said is.
  const started = useRef(false);
  useLayoutEffect(() => {
    started.current = false;
  }, [chat.address]);
  useLayoutEffect(() => {
    if (started.current || rows.length === 0) return;
    started.current = true;
    virtualiser.scrollToIndex(rows.length - 1, { align: "end" });
  }, [rows.length, virtualiser]);

  /**
   * Where the reader was, measured from the bottom.
   *
   * Older messages arrive above the ones on screen, so the distance from the top
   * changes and the distance from the bottom does not. Anchoring on the wrong one of
   * those throws the reader to the top of the conversation every time a page loads,
   * and then nothing more loads at all, because the trigger is a scroll event and
   * there is no longer anywhere to scroll. A browser found that; jsdom cannot.
   */
  const anchor = useRef<number | null>(null);

  const loadOlder = useCallback(() => {
    const element = viewport.current;
    if (element === null || !hasNextPage || isFetchingNextPage) return;
    anchor.current = element.scrollHeight - element.scrollTop;
    void fetchNextPage();
  }, [fetchNextPage, hasNextPage, isFetchingNextPage]);

  // Put the reader back where they were, before the browser has painted, so the
  // conversation does not visibly jump.
  useLayoutEffect(() => {
    const element = viewport.current;
    if (element === null || anchor.current === null) return;
    element.scrollTop = element.scrollHeight - anchor.current;
    anchor.current = null;
  }, [rows.length]);

  // Reaching the top asks for the messages before these.
  const onScroll = useCallback(() => {
    const element = viewport.current;
    if (element === null) return;
    if (element.scrollTop < 200) loadOlder();
  }, [loadOlder]);

  useEffect(() => {
    const element = viewport.current;
    if (element === null) return;
    element.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      element.removeEventListener("scroll", onScroll);
    };
  }, [onScroll]);

  if (isError) {
    return <Empty>{t("couldNotRead")}</Empty>;
  }
  if (isPending) {
    return <Empty>{t("loading")}</Empty>;
  }

  const items = virtualiser.getVirtualItems();

  return (
    <div ref={viewport} className="min-h-0 flex-1 overflow-y-auto px-4 pb-8">
      <div className="mx-auto max-w-3xl">
        {hasNextPage && (
          <div className="flex justify-center py-3">
            <Button
              size="sm"
              disabled={isFetchingNextPage}
              onClick={loadOlder}
            >
              {isFetchingNextPage ? t("loading") : t("earlierMessages")}
            </Button>
          </div>
        )}

        <div
          className="relative w-full"
          style={{ height: `${String(virtualiser.getTotalSize())}px` }}
        >
          <div
            className="absolute top-0 left-0 w-full"
            style={{ transform: `translateY(${String(items[0]?.start ?? 0)}px)` }}
          >
            {items.map((item) => {
              const row = rows[item.index];
              if (row === undefined) return null;
              return (
                <div key={item.key} data-index={item.index} ref={virtualiser.measureElement}>
                  {row.kind === "day" ? (
                    <DaySeparator label={row.label} />
                  ) : (
                    <Message message={row.message} language={language} />
                  )}
                </div>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}

/** A row is either a message or the date above a run of them. */
type Row =
  | { kind: "day"; key: string; label: string }
  | { kind: "message"; key: string; message: ArchivedMessage };

/**
 * toRows flattens the pages into what the list actually shows.
 *
 * The separators are computed here rather than while rendering because the
 * virtualiser addresses rows by index, and a row that appears only sometimes would
 * make an index mean different things at different moments.
 */
function toRows(pages: readonly { messages: readonly ArchivedMessage[] }[], language: Language): Row[] {
  const rows: Row[] = [];
  let previous = "";

  // Pages arrive newest first, each one older than the last.
  for (let at = pages.length - 1; at >= 0; at--) {
    for (const message of pages[at]?.messages ?? []) {
      const day = dayOf(message.sent_at, language);
      if (day !== previous) {
        previous = day;
        rows.push({ kind: "day", key: `day-${day}`, label: day });
      }
      rows.push({ kind: "message", key: `m-${String(message.id)}`, message });
    }
  }
  return rows;
}

function DaySeparator({ label }: { label: string }) {
  const t = useT();
  return (
    <div className="my-3 text-center">
      <span className="rounded-full border border-[var(--color-line)] bg-[var(--color-panel)] px-3 py-0.5 text-xs text-[var(--color-muted)]">
        {label === "" ? t("unknownDate") : label}
      </span>
    </div>
  );
}

function Empty({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex-1 p-12 text-center text-[var(--color-muted)]">{children}</div>
  );
}

/** ThreadHeader names the conversation and says how much of it there is. */
export function ThreadHeader({ chat, language }: { chat: Chat; language: Language }) {
  const t = useT();
  const about = [`${count(chat.message_count, language)} ${t("messages")}`];
  if (chat.participants !== undefined && chat.participants.length > 0) {
    about.push(`${String(chat.participants.length)} ${t("members")}`);
  }

  return (
    <header className="flex items-baseline justify-between gap-4 border-b border-[var(--color-line)] bg-[var(--color-panel)] px-4 py-2.5">
      <h2 className="m-0 min-w-0 break-words text-base font-semibold">{chat.name}</h2>
      <span className="whitespace-nowrap text-xs text-[var(--color-muted)]">
        {about.join(" · ")}
      </span>
    </header>
  );
}
