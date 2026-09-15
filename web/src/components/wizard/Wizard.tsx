import { useState } from "react";

import type { Extras } from "@/api/client";
import { decrypt, fetchFromPhone, extract, openFile } from "@/api/client";
import {
  isMissingImporter,
  useBackups,
  useClose,
  useSetupAction,
} from "@/api/queries";
import type { Setup } from "@/api/types";
import { Notices } from "@/components/Notices";
import { Button } from "@/components/ui/button";
import { Migration } from "@/components/migration/Migration";
import { Android } from "@/components/wizard/Android";
import { Backups } from "@/components/wizard/Backups";
import { Choose } from "@/components/wizard/Choose";
import { Existing } from "@/components/wizard/Existing";
import { Failure } from "@/components/wizard/Failure";
import { Say, Shell } from "@/components/wizard/Shell";
import { Working } from "@/components/wizard/Working";
import type { Route, Typed } from "@/components/wizard/route";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";

/**
 * Getting from a dead phone to an open archive.
 *
 * The state machine has two halves and they are deliberately not merged. The server
 * owns a stage — nothing, working, ready, failed — which this page cannot change
 * except by asking, and which is the same whichever route somebody took. The page
 * owns which route they chose and how far along it they are, which the server knows
 * nothing about and must not: a reload during a decryption should show the
 * decryption, not the form that started it.
 *
 * So the stage decides the screen whenever it has something to say, and the route
 * decides it otherwise, with one exception that matters. A failure is not the end of
 * anything here — the server will accept the next attempt without being reset — so
 * when there is a form behind the failure, the failure is shown above that form
 * rather than instead of it, and what was typed wrongly is still there to correct.
 *
 * `ready` never reaches this component at all; it is the whole archive, and App
 * shows that instead.
 */
export function Wizard({
  state,
  unreachable,
  onRetry,
  language,
}: {
  state: Setup | undefined;
  /** Why the server could not be asked at all, as opposed to what it answered. */
  unreachable: string | undefined;
  onRetry: () => void;
  language: Language;
}) {
  const t = useT();
  const action = useSetupAction();
  const letGo = useClose();

  const [route, setRoute] = useState<Route>("choose");
  const [edited, setEdited] = useState<string | undefined>(undefined);
  const [contacts, setContacts] = useState("");
  const [read, setRead] = useState(false);

  // Kept here rather than in the screens that ask for it; see Typed for why.
  const [typed, setTyped] = useState<Typed>({ at: 0, file: "", path: "" });
  const type = (change: Partial<Typed>) => {
    setTyped((was) => ({ ...was, ...change }));
  };

  const stage = state?.stage;

  /**
   * Whether this build has the part that reads phone backups.
   *
   * Asked here rather than on the screen that needs it so that two of the three
   * routes can be left off the first screen entirely when the answer is no. It is
   * one request to the program that served this page, and it is not made while that
   * program is busy: minutes of decrypting is no time to be rummaging through
   * somebody's backup folder for an answer nobody will see.
   */
  const backups = useBackups(stage === "empty" || stage === "failed");
  const importing = !isMissingImporter(backups.error);

  /**
   * The workspace the server suggested until somebody changes it, and then theirs.
   *
   * Holding "not changed" as nothing rather than copying the server's answer into
   * state does two things: a folder somebody typed is not overwritten by the next
   * poll, and an untouched one is left out of the request altogether, which is how
   * the server is told to keep what it already has rather than being handed back
   * its own suggestion as though it were a decision.
   */
  const into = edited ?? state?.workspace ?? "";

  const extras = (folder: boolean): Extras => ({
    ...(folder && edited !== undefined ? { into: edited.trim() } : {}),
    ...(contacts.trim() === "" ? {} : { contacts: contacts.trim() }),
  });

  const start = (work: () => Promise<Setup>) => {
    setRead(false);
    action.start(work);
  };

  /**
   * Leaving a failure behind when there is no form to correct.
   *
   * The server is asked to let go, and the failure is marked as read here as well.
   * Both, rather than either: if the server will not close from a failed stage, the
   * person is still moved on, because the one thing this screen may never do is
   * strand somebody whose phone has died.
   */
  const again = () => {
    setRead(true);
    setRoute("choose");
    letGo();
  };

  const back = () => {
    setRead(true);
    setRoute("choose");
  };

  const failure = stage === "failed" && !read ? state : undefined;

  return (
    <div className="flex min-h-dvh flex-col bg-[var(--color-bg)]">
      <p className="m-0 border-b border-[var(--color-line)] bg-[var(--color-surface)] px-5 py-2 text-center text-sm text-[var(--color-muted)]">
        {t("readOnly")}
      </p>

      {action.refused !== undefined && (
        <p
          role="alert"
          className="m-0 border-b border-[var(--color-accent)] px-5 py-2 text-center text-sm font-medium"
        >
          {t("refused", { detail: action.refused })}
        </p>
      )}

      <Screen
        state={state}
        unreachable={unreachable}
        failure={failure}
        route={route}
        importing={importing}
        into={into}
        contacts={contacts}
        typed={typed}
        busy={action.busy}
        language={language}
        onRetry={onRetry}
        onRoute={setRoute}
        onInto={setEdited}
        onContacts={setContacts}
        onTyped={type}
        onBack={back}
        onAgain={again}
        onStart={start}
        extras={extras}
      />

      <Notices />
    </div>
  );
}

/** Which of the wizard's screens the server's stage and the chosen route add up to. */
function Screen({
  state,
  unreachable,
  failure,
  route,
  importing,
  into,
  contacts,
  typed,
  busy,
  language,
  onRetry,
  onRoute,
  onInto,
  onContacts,
  onTyped,
  onBack,
  onAgain,
  onStart,
  extras,
}: {
  state: Setup | undefined;
  unreachable: string | undefined;
  failure: Setup | undefined;
  route: Route;
  importing: boolean;
  into: string;
  contacts: string;
  typed: Typed;
  busy: boolean;
  language: Language;
  onRetry: () => void;
  onRoute: (route: Route) => void;
  onInto: (into: string) => void;
  onContacts: (contacts: string) => void;
  onTyped: (change: Partial<Typed>) => void;
  onBack: () => void;
  onAgain: () => void;
  onStart: (work: () => Promise<Setup>) => void;
  extras: (folder: boolean) => Extras;
}) {
  if (unreachable !== undefined)
    return <Unreachable said={unreachable} onRetry={onRetry} />;
  if (state?.stage === "working")
    return <Working state={state} language={language} />;

  switch (route) {
    case "backups":
      return (
        <Backups
          into={into}
          contacts={contacts}
          busy={busy}
          failure={failure}
          language={language}
          onInto={onInto}
          onContacts={onContacts}
          onBack={onBack}
          onInstead={() => {
            onRoute("existing");
          }}
          onChoose={(path) => {
            onStart(() => extract(path, extras(true)));
          }}
        />
      );
    case "android":
      return (
        <Android
          into={into}
          contacts={contacts}
          typed={typed}
          busy={busy}
          failure={failure}
          onInto={onInto}
          onContacts={onContacts}
          onTyped={onTyped}
          onBack={onBack}
          language={language}
          onDecrypt={(file, key) => {
            onStart(() => decrypt({ file, key, ...extras(true) }));
          }}
          // Off the phone and unlocked in one go: a file somebody cannot open is
          // not what they came for.
          onFetch={(serial, path, key) => {
            onStart(() =>
              fetchFromPhone(serial, path, key, extras(true).into ?? ""),
            );
          }}
        />
      );
    case "existing":
      return (
        <Existing
          contacts={contacts}
          typed={typed}
          busy={busy}
          failure={failure}
          onContacts={onContacts}
          onTyped={onTyped}
          onBack={onBack}
          onOpen={(path) => {
            onStart(() => openFile(path, extras(false)));
          }}
        />
      );
    case "migrate":
      // Its own screens and its own state on the server: this one ends with a backup
      // to restore rather than with an archive to read.
      return <Migration language={language} onLeave={onBack} />;
    case "choose":
      // Nothing to correct behind a failure met here, so it gets a screen of its own.
      if (failure !== undefined)
        return <Failure state={failure} onBack={onAgain} />;
      return <Choose onChoose={onRoute} importing={importing} />;
  }
}

/**
 * The server did not answer at all.
 *
 * Almost always because it was closed: this page outlives the program that served it
 * whenever somebody presses control-C in the terminal it was started from, and the
 * tab is still sitting there. Saying so is better than a spinner that never stops.
 */
function Unreachable({ said, onRetry }: { said: string; onRetry: () => void }) {
  const t = useT();

  return (
    <Shell step={1} heading={t("couldNotAsk")}>
      <Say>{said}</Say>
      <div>
        <Button variant="primary" onClick={onRetry}>
          {t("retry")}
        </Button>
      </div>
    </Shell>
  );
}
