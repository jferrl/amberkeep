import type { Phrase } from "@/i18n";
import { Field } from "@/components/wizard/Field";
import { useT } from "@/i18n";

/**
 * The two things every route asks for besides the file itself.
 *
 * They are one component rather than two pairs of fields copied into three screens,
 * because the sentence explaining what will be written where is the promise this
 * program is trusted on, and three copies of it is three chances for one of them to
 * stop being true.
 */
export function Where({
  into,
  onInto,
  wrong,
  writes,
  contacts,
  onContacts,
}: {
  into: string;
  onInto: (into: string) => void;
  wrong?: string | undefined;
  /** What will appear in that folder, said plainly and before anything is written. */
  writes: Phrase;
  contacts: string;
  onContacts: (contacts: string) => void;
}) {
  const t = useT();

  return (
    <>
      <Field
        label={t("workspaceLabel")}
        choosing={{ what: "folder", named: t("chooseWorkspace") }}
        hint={t(writes)}
        value={into}
        wrong={wrong}
        onChange={onInto}
      />
      <Field
        label={t("contactsLabel")}
        choosing={{ what: "contacts", named: t("chooseContacts") }}
        hint={t("contactsHint")}
        value={contacts}
        onChange={onContacts}
      />
    </>
  );
}
