import { useCallback, useDeferredValue, useEffect, useState } from "react";
import type { ReactNode } from "react";

import type { Chat, Hit } from "@/api/types";
import {
  useArchive,
  useChats,
  useClose,
  useSearch,
  useSetupState,
} from "@/api/queries";
import { Notices } from "@/components/Notices";
import { Thanks } from "@/components/Thanks";
import { Export } from "@/components/export/Export";
import { Framed } from "@/components/wizard/Frame";
import { ThanksProvider } from "@/lib/thanks";
import { asking } from "@/lib/desktop";
import { Sidebar } from "@/components/Sidebar";
import { Thread, ThreadHeader } from "@/components/Thread";
import { Button } from "@/components/ui/button";
import { Wizard } from "@/components/wizard/Wizard";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";
import { cn } from "@/lib/utils";

/**
 * The program, which is two programs depending on whether it holds anything.
 *
 * Started with a file it is an archive to read. Started with nothing — which is how
 * somebody whose phone has died finds it — it is a wizard that gets them to a file
 * they can read. The server says which, and it is the only thing that does: a page
 * that decided for itself would show a browser over an archive that is not open.
 */
export function App({ language }: { language: Language }) {
  const setup = useSetupState();
  const [keeping, setKeeping] = useState(false);
  const closing = useClose();

  // The window's menu, when there is a window. In a browser this subscribes to
  // nothing and unsubscribes from nothing.
  useEffect(
    () =>
      asking((what) => {
        if (what === "keep") setKeeping(true);
        if (what === "close") closing();
      }),
    [closing],
  );

  // Where somebody can say thanks, provided once for every screen under here. The
  // program sends it with the state it already polls, so nothing has to be handed it
  // and there is one address.
  const thanks = setup.data?.thanks;

  if (setup.data?.stage === "ready") {
    // Writing the archive out takes the whole window rather than a dialog over the
    // conversations. It is a job with its own progress and its own ending, and the
    // list underneath it would be a list nobody is reading.
    return wrapped(
      thanks,
      keeping ? (
        // Framed like the wizard: it takes the whole window, so it needs the whole
        // window's furniture. Without it this screen had no promise at the top, no
        // notices at the bottom, and its first line of text under the close, minimise
        // and zoom buttons.
        <Framed>
          <Export
            language={language}
            onLeave={() => {
              setKeeping(false);
            }}
          />
        </Framed>
      ) : (
        <Browser
          language={language}
          onKeep={() => {
            setKeeping(true);
          }}
        />
      ),
    );
  }
  if (setup.isPending) return <Waiting />;

  return wrapped(
    thanks,
    <Wizard
      state={setup.data}
      unreachable={setup.error?.message}
      onRetry={() => {
        void setup.refetch();
      }}
      language={language}
    />,
  );
}

/**
 * wrapped puts the address where every screen under it can find it.
 *
 * A function rather than three copies of the same provider, because the three things
 * this returns — the archive, the screen that writes it out, and the wizard — are
 * returned from three places and all of them need it.
 */
function wrapped(thanks: string | undefined, screen: ReactNode): ReactNode {
  return <ThanksProvider value={thanks}>{screen}</ThanksProvider>;
}

/** The moment before the server has said what it is holding. */
function Waiting() {
  const t = useT();
  return (
    <p className="p-12 text-center text-[var(--color-muted)]">{t("loading")}</p>
  );
}

/**
 * The archive, in two panes: what there is on the left, what is in it on the right.
 *
 * Everything is read from the program that served this page and from nowhere else.
 * There is no account, nothing is uploaded, and closing the program ends the
 * server, which is the whole reason it can be trusted with somebody's entire
 * message history.
 */
function Browser({
  language,
  onKeep,
}: {
  language: Language;
  onKeep: () => void;
}) {
  const t = useT();
  const [term, setTerm] = useState("");
  const [chat, setChat] = useState<Chat | undefined>(undefined);
  const letGo = useClose();

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
      const found = chats.data?.chats.find(
        (c) => c.address === hit.chat_address,
      );
      if (found !== undefined) setChat(found);
    },
    [chats.data?.chats],
  );

  const reading = chat !== undefined;

  return (
    <div className="grid h-dvh grid-cols-1 md:grid-cols-[20rem_1fr]">
      <nav
        className={cn(
          "flex min-h-0 flex-col border-r border-[var(--color-line)] bg-[var(--color-surface)]",
          reading && "hidden md:flex",
        )}
      >
        {/*
          The promise, and the two ways out of this archive.

          Stacked rather than one row. The sidebar is twenty rem, and a sentence
          beside two buttons in that width wrapped all three onto three lines each.

          The top padding is for the window: its title bar is hidden and inset, so
          the page begins at the very top of the frame and the close, minimise and
          zoom buttons are drawn over whatever is there — which was this sentence.
          --frame is how tall that band is, and it is zero in a browser, where there
          is nothing above to clear.
        */}
        <div className="flex flex-col gap-2 border-b border-[var(--color-line)] px-3.5 pt-[calc(var(--frame)+0.5rem)] pb-2">
          <span className="text-xs text-[var(--color-muted)]">
            {t("readOnly")}
          </span>
          <div className="flex flex-wrap items-center gap-1.5">
            {/* The thing most people came for. It was reachable only from a
                terminal, which is to say not reachable by anybody who needed it. */}
            <Button size="sm" onClick={onKeep}>
              {t("exportAction")}
            </Button>
            <Button variant="quiet" size="sm" onClick={letGo}>
              {t("closeArchive")}
            </Button>
          </div>
        </div>

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

        {/*
          The sidebar of an archive somebody is reading, which is where a person
          spends minutes rather than seconds. Above the notices, out of the way of
          the two controls at the top that actually do something.
        */}
        <div className="mt-auto px-3.5 pb-3">
          <Thanks />
        </div>

        <Notices />
      </nav>

      <section
        className={cn(
          "flex min-h-0 min-w-0 flex-col",
          !reading && "hidden md:flex",
        )}
      >
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
      <header className="border-b border-[var(--color-line)] bg-[var(--color-surface)] px-4 py-2.5">
        <h2 className="m-0 text-base font-semibold">
          {t("pickAConversation")}
        </h2>
      </header>
      <p className="flex-1 p-12 text-center text-[var(--color-muted)]">
        {t("pickToStart")}
      </p>
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
        <Button
          variant="quiet"
          size="sm"
          className="ml-2 md:hidden"
          onClick={onBack}
        >
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
