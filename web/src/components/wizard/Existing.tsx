import { useState } from "react";
import type { SyntheticEvent } from "react";

import type { Setup } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Field } from "@/components/wizard/Field";
import { Trouble } from "@/components/wizard/Failure";
import { Shell } from "@/components/wizard/Shell";
import type { Typed } from "@/components/wizard/route";
import { useT } from "@/i18n";

/**
 * Opening a file somebody already has.
 *
 * Nothing here asks which kind it is. The server recognises an Android database and
 * an iPhone one by what is inside them, so a file that was renamed on the way here,
 * or was never named properly in the first place, still opens.
 *
 * There is no workspace on this screen because the file is read where it lies, so a
 * folder to write into would be one more thing to worry about and one more promise
 * to keep. That is not the same as nothing being written: opening an archive builds
 * a search index beside it, and the field says so before anybody types a path,
 * because a program that writes a file somebody was not told about has spent the
 * only thing it has.
 */
export function Existing({
  contacts,
  typed,
  onContacts,
  onTyped,
  onOpen,
  onBack,
  busy,
  failure,
}: {
  contacts: string;
  typed: Typed;
  onContacts: (contacts: string) => void;
  onTyped: (change: Partial<Typed>) => void;
  onOpen: (path: string) => void;
  onBack: () => void;
  busy: boolean;
  failure: Setup | undefined;
}) {
  const t = useT();
  const [wrong, setWrong] = useState<string | undefined>(undefined);

  const send = (event: SyntheticEvent) => {
    event.preventDefault();
    const where = typed.path.trim();
    if (where === "") {
      setWrong(t("fileNeeded"));
      return;
    }
    setWrong(undefined);
    onOpen(where);
  };

  return (
    <Shell
      step={2}
      total={2}
      heading={t("fileTitle")}
      lead={t("fileHelp")}
      trouble={
        failure === undefined ? undefined : (
          <Trouble state={failure} correctable />
        )
      }
      onBack={onBack}
    >
      <form className="flex flex-col gap-5" onSubmit={send}>
        <Field
          label={t("fileLabel")}
          choosing={{ what: "database", named: t("chooseDatabase") }}
          hint={t("fileWrites")}
          value={typed.path}
          wrong={wrong}
          onChange={(next) => {
            setWrong(undefined);
            onTyped({ path: next });
          }}
        />
        <Field
          label={t("contactsLabel")}
          choosing={{ what: "contacts", named: t("chooseContacts") }}
          hint={t("contactsHint")}
          value={contacts}
          onChange={onContacts}
        />
        <div>
          <Button type="submit" disabled={busy}>
            {t("fileOpen")}
          </Button>
        </div>
      </form>
    </Shell>
  );
}
