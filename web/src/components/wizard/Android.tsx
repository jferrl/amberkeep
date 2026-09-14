import { useState } from "react";
import type { SyntheticEvent } from "react";

import type { Setup } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Field } from "@/components/wizard/Field";
import { Trouble } from "@/components/wizard/Failure";
import { Aside, Expect, Say, Shell } from "@/components/wizard/Shell";
import { Where } from "@/components/wizard/Where";
import type { Typed } from "@/components/wizard/route";
import { useT } from "@/i18n";
import type { Phrase } from "@/i18n";

/**
 * The four screens that happen on the phone, one instruction each.
 *
 * They are a table rather than four almost identical components because the thing
 * that must never drift is the shape: every one of them says what to do and then
 * what will appear afterwards, so somebody halfway through a menu they have never
 * opened can tell whether they are still on course.
 */
const onThePhone: { title: Phrase; says: readonly Phrase[]; expect: Phrase }[] =
  [
    {
      title: "androidStep1Title",
      says: ["androidStep1a", "androidStep1b"],
      expect: "androidNext1",
    },
    {
      title: "androidStep2Title",
      says: ["androidStep2a", "androidStep2b"],
      expect: "androidNext2",
    },
    {
      title: "androidStep3Title",
      says: ["androidStep3a"],
      expect: "androidNext3",
    },
    {
      title: "androidStep4Title",
      says: ["androidStep4a", "androidStep4b"],
      expect: "androidNext4",
    },
  ];

/** Every screen of this route, including the choice that led to it. */
const screens = onThePhone.length + 2;

/**
 * A key as WhatsApp shows it: 64 hexadecimal characters, in eight groups of eight.
 *
 * Checked here as well as on the server because the mistake it catches is the one
 * everybody makes — WhatsApp offers a password beside the key, and a password cannot
 * be used at all — and catching it before a request is sent is what keeps a mistyped
 * secret from being carried anywhere.
 */
const looksLikeAKey = /^[0-9a-f]{64}$/i;

/**
 * Something that is a path rather than an attempt at a key.
 *
 * The server accepts a file holding the key as well as the key itself, and somebody
 * who saved it into a file has done exactly what they were told to. A separator is
 * enough to tell the two apart: a key has none, and every path that is not a bare
 * file name in the current directory has one.
 */
const looksLikeAPath = /[/\\]|^~/;

/** What is wrong with what has been filled in, if anything is. */
interface Wrong {
  file?: string;
  key?: string;
  into?: string;
}

/**
 * Getting a history off an Android phone, which this program cannot do for anybody.
 *
 * Nothing here is automation. The backup has to be made on the phone, by the person
 * holding it, and the key WhatsApp shows them is the only copy that will ever exist.
 * So this route is instructions, one screen at a time, and the program's part does
 * not begin until there is a file and a key on this computer.
 */
export function Android({
  into,
  onInto,
  contacts,
  onContacts,
  typed,
  onTyped,
  onDecrypt,
  onBack,
  busy,
  failure,
}: {
  into: string;
  onInto: (into: string) => void;
  contacts: string;
  onContacts: (contacts: string) => void;
  typed: Typed;
  onTyped: (change: Partial<Typed>) => void;
  onDecrypt: (file: string, key: string) => void;
  onBack: () => void;
  busy: boolean;
  failure: Setup | undefined;
}) {
  const t = useT();
  const { at, file } = typed;
  // The key alone is not kept anywhere but here, and not for long; see Typed.
  const [key, setKey] = useState("");
  const [wrong, setWrong] = useState<Wrong>({});

  const back = () => {
    setWrong({});
    if (at === 0) onBack();
    else onTyped({ at: at - 1 });
  };

  const showing = onThePhone[at];
  if (showing !== undefined) {
    return (
      <Shell
        step={at + 2}
        total={screens}
        heading={t(showing.title)}
        onBack={back}
      >
        <div className="flex flex-col gap-3">
          {showing.says.map((phrase) => (
            <Say key={phrase}>{t(phrase)}</Say>
          ))}
          {at === 0 && (
            <Aside>
              <Say>{t("androidStep1c")}</Say>
            </Aside>
          )}
          <Expect>{t(showing.expect)}</Expect>
        </div>

        <div>
          <Button
            variant="primary"
            onClick={() => {
              onTyped({ at: at + 1 });
            }}
          >
            {t("next")}
          </Button>
        </div>
      </Shell>
    );
  }

  /**
   * The key is checked, sent and forgotten in the same breath.
   *
   * It is dropped from this component the moment the request has been handed over,
   * so that the only place it exists afterwards is the body of a request in flight
   * to a program on this same machine. Nothing keeps a copy: not this component,
   * not the query cache, and never the address of the request.
   */
  const send = (event: SyntheticEvent) => {
    event.preventDefault();

    const where = file.trim();

    // The groups WhatsApp shows the key in are for reading it aloud, so whatever
    // somebody typed or pasted between them is thrown away rather than refused.
    const digits = key.replace(/\s+/g, "");
    const secret = looksLikeAKey.test(digits)
      ? digits
      : looksLikeAPath.test(key.trim())
        ? key.trim()
        : "";
    const folder = into.trim();

    // Every problem at once. Reporting the first and stopping meant somebody with
    // three blanks made three attempts to discover they had three blanks — and this
    // screen's own key is sixty-four characters read off a phone, so the attempt
    // they are being sent back to is the expensive one.
    const problems: { file?: string; key?: string; into?: string } = {};
    if (where === "") problems.file = t("fileNeeded");
    if (secret === "")
      problems.key = digits === "" ? t("keyNeeded") : t("keyNotAKey");
    if (folder === "") problems.into = t("workspaceNeeded");

    if (Object.keys(problems).length > 0) {
      setWrong(problems);
      return;
    }

    setWrong({});
    onDecrypt(where, secret);
    setKey("");
  };

  return (
    <Shell
      step={screens}
      total={screens}
      heading={t("androidStep5Title")}
      trouble={
        failure === undefined ? undefined : (
          <Trouble state={failure} correctable />
        )
      }
      onBack={back}
    >
      {/* No action and no method: this form is handled here and submitted nowhere.
          A form that fell back to the browser would put the key in the address bar,
          which is the one place it must never reach. */}
      <form className="flex flex-col gap-5" onSubmit={send}>
        <Field
          label={t("androidFileLabel")}
          hint={t("androidFileHint")}
          choosing={{ what: "encrypted", named: t("chooseEncrypted") }}
          value={file}
          wrong={wrong.file}
          onChange={(next) => {
            setWrong({});
            onTyped({ file: next });
          }}
        />

        <Field
          label={t("androidKeyLabel")}
          hint={`${t("androidKeyStays")} ${t("androidKeyOrFile")}`}
          value={key}
          wrong={wrong.key}
          secret
          autoComplete="off"
          onChange={(next) => {
            setWrong({});
            setKey(next);
          }}
        />

        <Where
          into={into}
          onInto={(next) => {
            setWrong({});
            onInto(next);
          }}
          wrong={wrong.into}
          writes="workspaceDecrypt"
          contacts={contacts}
          onContacts={onContacts}
        />

        <div>
          <Button variant="primary" type="submit" disabled={busy}>
            {t("androidDecrypt")}
          </Button>
        </div>
      </form>
    </Shell>
  );
}
