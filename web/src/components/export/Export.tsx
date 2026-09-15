import type { SyntheticEvent } from "react";
import { useId, useState } from "react";

import { forgetExport, writeArchive } from "@/api/client";
import { useExport, useExportStep } from "@/api/queries";
import type { Export as State, Format } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Thanks } from "@/components/Thanks";
import { Field } from "@/components/wizard/Field";
import { Trouble } from "@/components/wizard/Failure";
import { Aside, Say, Shell } from "@/components/wizard/Shell";
import { Working } from "@/components/wizard/Working";
import { useT } from "@/i18n";
import { carried } from "@/lib/said";
import type { Language, Phrase } from "@/i18n";
import { count } from "@/lib/format";

/**
 * Writing the archive out to keep.
 *
 * The command has done this from the beginning and the page could not, which put the
 * most ordinary thing anybody wants — their own history, in a form they can keep,
 * print, or hand to a solicitor — behind a terminal.
 *
 * Nothing here is dangerous. It reads an archive that is already open and produces
 * files in a folder; no original is touched and no device is involved. So unlike the
 * migration it needs no typed word, and it may be asked for as often as somebody
 * likes.
 */
export function Export({
  language,
  onLeave,
}: {
  language: Language;
  onLeave: () => void;
}) {
  const now = useExport();
  const step = useExportStep();

  const [into, setInto] = useState("");
  const [formats, setFormats] = useState<Format[]>(["html"]);
  const [notices, setNotices] = useState(false);

  const state: State = now.data ?? { stage: "idle" };

  if (now.isPending)
    return (
      <Working
        state={{ stage: "working", workspace: "" }}
        language={language}
      />
    );

  switch (state.stage) {
    case "writing":
      return (
        <Working
          state={{
            stage: "working",
            workspace: "",
            ...(state.step === undefined ? {} : { step: state.step }),
            ...carried(state),
          }}
          language={language}
        />
      );

    case "done":
      return (
        <Written
          state={state}
          language={language}
          onAgain={() => {
            step.start(() => forgetExport());
          }}
          onLeave={onLeave}
        />
      );

    default:
      return (
        <Choices
          into={into}
          language={language}
          onInto={setInto}
          formats={formats}
          onFormats={setFormats}
          notices={notices}
          onNotices={setNotices}
          busy={step.busy}
          failure={state.stage === "failed" ? state : undefined}
          refused={step.refused}
          onBack={onLeave}
          onWrite={() => {
            step.start(() =>
              writeArchive({ into: into.trim(), formats, notices }),
            );
          }}
        />
      );
  }
}

/** The formats, each with what it is actually for. */
const offered: { format: Format; title: Phrase; help: Phrase }[] = [
  { format: "html", title: "exportHTML", help: "exportHTMLHelp" },
  { format: "text", title: "exportText", help: "exportTextHelp" },
  { format: "json", title: "exportJSON", help: "exportJSONHelp" },
];

/** What to write, and where to put it. */
function Choices({
  into,
  language,
  onInto,
  formats,
  onFormats,
  notices,
  onNotices,
  busy,
  failure,
  refused,
  onBack,
  onWrite,
}: {
  into: string;
  language: Language;
  onInto: (into: string) => void;
  formats: Format[];
  onFormats: (formats: Format[]) => void;
  notices: boolean;
  onNotices: (on: boolean) => void;
  busy: boolean;
  failure: State | undefined;
  refused: string | undefined;
  onBack: () => void;
  onWrite: () => void;
}) {
  const t = useT();
  const [wrong, setWrong] = useState<string | undefined>(undefined);

  const send = (event: SyntheticEvent) => {
    event.preventDefault();
    if (formats.length === 0) {
      setWrong(t("exportNoFormat"));
      return;
    }
    onWrite();
  };

  const toggle = (format: Format, on: boolean) => {
    setWrong(undefined);
    onFormats(on ? [...formats, format] : formats.filter((f) => f !== format));
  };

  return (
    <Shell
      step={1}
      heading={t("exportTitle")}
      lead={t("exportHelp")}
      trouble={
        failure === undefined ? undefined : (
          <Trouble state={failure} language={language} correctable />
        )
      }
      onBack={onBack}
    >
      {refused !== undefined && (
        <Aside heading={t("exportRefused")} tone="blocker">
          <Say>{refused}</Say>
        </Aside>
      )}

      <form className="flex flex-col gap-5" onSubmit={send}>
        <fieldset className="m-0 flex flex-col gap-3 border-0 p-0">
          <legend className="mb-1 p-0 text-sm font-medium">
            {t("exportFormats")}
          </legend>
          {offered.map(({ format, title, help }) => (
            <Pick
              key={format}
              label={t(title)}
              hint={t(help)}
              on={formats.includes(format)}
              onChange={(chosen) => {
                toggle(format, chosen);
              }}
            />
          ))}
          {wrong !== undefined && (
            <p
              role="alert"
              className="m-0 text-[0.8125rem] font-medium text-[var(--color-alarm)]"
            >
              {wrong}
            </p>
          )}
        </fieldset>

        <Field
          label={t("exportIntoLabel")}
          hint={t("exportIntoHint")}
          choosing={{ what: "folder", named: t("chooseExportInto") }}
          value={into}
          onChange={onInto}
        />

        <Pick
          label={t("exportNotices")}
          hint={t("exportNoticesHelp")}
          on={notices}
          onChange={onNotices}
        />

        <div>
          <Button variant="primary" type="submit" disabled={busy}>
            {t("exportWrite")}
          </Button>
        </div>
      </form>
    </Shell>
  );
}

/** One thing that is off unless somebody turns it on, with the reason beside it. */
function Pick({
  label,
  hint,
  on,
  onChange,
}: {
  label: string;
  hint: string;
  on: boolean;
  onChange: (on: boolean) => void;
}) {
  const box = useId();
  const note = useId();

  return (
    <div className="flex items-start gap-3 py-1">
      <Checkbox
        id={box}
        checked={on}
        aria-describedby={note}
        className="mt-0.5"
        onCheckedChange={(state) => {
          onChange(state === true);
        }}
      />
      <div>
        <label
          htmlFor={box}
          className="block cursor-pointer text-sm font-medium"
        >
          {label}
        </label>
        <p
          id={note}
          className="m-0 mt-0.5 text-[0.8125rem] text-[var(--color-muted)]"
        >
          {hint}
        </p>
      </div>
    </div>
  );
}

/** A folder of files, and where to find it. */
function Written({
  state,
  language,
  onAgain,
  onLeave,
}: {
  state: State;
  language: Language;
  onAgain: () => void;
  onLeave: () => void;
}) {
  const t = useT();
  const result = state.result;

  return (
    <Shell step={2} heading={t("exportDoneTitle")} lead={t("exportDoneHelp")}>
      {result !== undefined && (
        <div className="flex flex-col gap-3">
          <dl className="m-0 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5">
            {(
              [
                [result.conversations, t("exportDoneConversations")],
                [result.messages, t("exportDoneMessages")],
              ] as [number, string][]
            ).map(([n, said]) => (
              <div key={said} className="contents">
                <dt className="text-right text-sm font-medium tabular-nums">
                  {count(n, language)}
                </dt>
                <dd className="m-0 text-sm text-[var(--color-muted)]">
                  {said}
                </dd>
              </div>
            ))}
          </dl>

          <p className="m-0 text-sm">
            <span className="font-medium">{t("exportDoneWhere")} </span>
            <span className="break-all text-[var(--color-muted)]">
              {result.into}
            </span>
          </p>
        </div>
      )}

      <Say>{t("exportDoneKeep")}</Say>

      <Thanks full />

      <div className="flex flex-wrap gap-2">
        <Button variant="primary" onClick={onLeave}>
          {t("exportDoneBack")}
        </Button>
        <Button variant="quiet" onClick={onAgain}>
          {t("exportAgain")}
        </Button>
      </div>
    </Shell>
  );
}
