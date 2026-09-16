import { usePhoneFiles } from "@/api/queries";
import { Pick } from "@/components/wizard/Pick";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";
import { size } from "@/lib/format";

/**
 * The offer to bring the photographs across as well.
 *
 * Off unless somebody asks for it, and the label says why: this is the longest wait
 * in the whole program, it happens before any messages appear, and the archive is
 * perfectly readable without it. What it costs is said in gigabytes rather than in
 * adjectives, because "this may take a while" is what every program says before
 * taking an hour.
 *
 * A phone with no such folder is offered nothing at all rather than a tick box that
 * would do nothing.
 */
export function Photographs({
  serial,
  language,
  on,
  onChange,
}: {
  serial: string;
  language: Language;
  on: boolean;
  onChange: (on: boolean) => void;
}) {
  const t = useT();
  const files = usePhoneFiles(serial);

  const kinds = files.data?.kinds ?? [];
  if (kinds.length === 0) return null;

  const bytes = files.data?.bytes ?? 0;

  return (
    <Pick
      label={
        bytes > 0
          ? t("phonePhotographsSized", { size: size(bytes, language) })
          : t("phonePhotographs")
      }
      hint={t("phonePhotographsHelp")}
      on={on}
      onChange={onChange}
    />
  );
}
