import { Smartphone } from "lucide-react";

import { usePhoneBackups, usePhones } from "@/api/queries";
import type { Phone, PhoneBackup } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Aside, Say } from "@/components/wizard/Shell";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";
import { size } from "@/lib/format";

/**
 * The phone, if it is plugged in.
 *
 * The screen beside this one asks somebody to open
 * Android/media/com.whatsapp/WhatsApp/Databases on the phone's own storage and copy
 * a file out of it. That is a sentence a developer writes: the folder is hidden on
 * most launchers, and the file next to the one they want is a fragment that looks
 * almost identical and is useless alone — so getting it wrong costs them the twenty
 * minutes the backup took, and they find out much later.
 *
 * The phone is usually plugged into the same computer. So this asks it, and the
 * typing stays underneath for everybody it cannot help: no Android tools installed,
 * no cable, a phone that will not be trusted, or a file already copied across.
 */
export function Plugged({
  language,
  busy,
  chosen,
  onChoose,
}: {
  language: Language;
  busy: boolean;
  /** The backup already picked, so the one in the list can say it is the one. */
  chosen: string;
  onChoose: (phone: Phone, backup: PhoneBackup) => void;
}) {
  const t = useT();
  const phones = usePhones();

  const found = phones.data?.phones ?? [];
  const trouble = phones.data?.trouble;

  // A computer that has never had the Android tools says nothing: this genuinely is
  // not on offer there, and explaining a thing that cannot happen is noise.
  if (trouble !== undefined) return null;

  // A computer that can look and sees nothing plugged in is a different case, and
  // getting it wrong was the whole bug: somebody holding the phone this exists for
  // could not discover the option, because it only appeared once they had already
  // guessed to plug the phone in. So it says so, and waits.
  const waiting = found.length === 0 && !phones.isPending;

  return (
    <section className="flex flex-col gap-3">
      <h2 className="m-0 flex items-center gap-2 text-sm font-semibold">
        <Smartphone aria-hidden className="size-4 text-[var(--color-accent)]" />
        {waiting ? t("phoneWaiting") : t("phoneFound")}
      </h2>

      {waiting && <Say>{t("phoneWaitingHelp")}</Say>}

      {found.map((phone) => (
        <One
          key={phone.serial}
          phone={phone}
          language={language}
          busy={busy}
          chosen={chosen}
          onChoose={onChoose}
        />
      ))}
    </section>
  );
}

/** One phone, and what is on it. */
function One({
  phone,
  language,
  busy,
  chosen,
  onChoose,
}: {
  phone: Phone;
  language: Language;
  busy: boolean;
  chosen: string;
  onChoose: (phone: Phone, backup: PhoneBackup) => void;
}) {
  const t = useT();
  // Only asked once the phone will answer. An unauthorised phone would refuse and
  // the refusal would read as the program failing.
  const backups = usePhoneBackups(phone.ready ? phone.serial : "");

  if (!phone.ready) {
    return (
      <Aside heading={t("phoneNotReady", { name: phone.name })} tone="warning">
        <Say>
          {phone.trouble === "offline"
            ? t("phoneOffline")
            : t("phoneUnauthorized")}
        </Say>
      </Aside>
    );
  }

  const found = backups.data?.backups ?? [];
  // Fragments are shown but never offered: a person who sees only one file and is
  // told it is the wrong one has no way to tell whether the right one exists.
  const whole = found.filter((b) => !b.partial);

  if (backups.isPending) {
    return <Say>{t("phoneLooking", { name: phone.name })}</Say>;
  }
  if (whole.length === 0) {
    return (
      <Aside heading={t("phoneNoBackup", { name: phone.name })} tone="note">
        <Say>{t("phoneNoBackupHelp")}</Say>
      </Aside>
    );
  }

  return (
    <ul className="m-0 flex list-none flex-col gap-2 p-0">
      {whole.map((backup) => (
        <li
          key={backup.path}
          className="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-[var(--color-surface)] p-3 ring-1 ring-[var(--color-line)] ring-inset"
        >
          <span className="min-w-0">
            <span className="block text-sm font-medium">{phone.name}</span>
            <span className="mt-0.5 block text-[0.8125rem] text-[var(--color-muted)]">
              {backup.name} · {size(backup.size, language)}
            </span>
          </span>
          <Button
            disabled={busy || chosen === backup.path}
            onClick={() => {
              onChoose(phone, backup);
            }}
          >
            {chosen === backup.path ? t("phoneChosen") : t("phoneUse")}
          </Button>
        </li>
      ))}
      {found.length > whole.length && (
        <li className="text-[0.8125rem] text-[var(--color-muted)]">
          {t("phoneFragments")}
        </li>
      )}
    </ul>
  );
}
