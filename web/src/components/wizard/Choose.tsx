import { Aside, Say, Shell } from "@/components/wizard/Shell";
import type { Route } from "@/components/wizard/route";
import { useT } from "@/i18n";
import type { Phrase } from "@/i18n";

/**
 * The three routes, in the order somebody is most likely to need them.
 *
 * Two of them need the part of the engine that reads phone backups, which can be
 * left out of a build. When it has been, they are not offered: a choice that answers
 * with an error is worse than a choice that was never there.
 */
const routes: { route: Route; title: Phrase; help: Phrase; needsImporter: boolean }[] = [
  { route: "backups", title: "routeBackup", help: "routeBackupHelp", needsImporter: true },
  { route: "android", title: "routeAndroid", help: "routeAndroidHelp", needsImporter: true },
  { route: "existing", title: "routeFile", help: "routeFileHelp", needsImporter: false },
];

/**
 * Where somebody says what they have.
 *
 * Each choice is written as the sentence a person would say, not as the name of a
 * file format. Somebody whose phone died does not know what a crypt15 is and should
 * not have to find out to get past this screen.
 */
export function Choose({
  onChoose,
  importing,
}: {
  onChoose: (route: Route) => void;
  /** Whether this build has the part that reads phone backups at all. */
  importing: boolean;
}) {
  const t = useT();
  const offered = routes.filter((one) => importing || !one.needsImporter);

  return (
    <Shell step={1} heading={t("chooseTitle")} lead={t("chooseHelp")}>
      {!importing && (
        <Aside heading={t("noImporter")}>
          <Say>{t("noImporterHelp")}</Say>
        </Aside>
      )}

      <ul className="m-0 flex list-none flex-col gap-3 p-0">
        {offered.map(({ route, title, help }) => (
          <li key={route}>
            <button
              type="button"
              className="w-full rounded-lg border border-[var(--color-line)] bg-[var(--color-panel)] p-4 text-left transition-colors hover:border-[var(--color-accent)] focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-[var(--color-accent)]"
              onClick={() => {
                onChoose(route);
              }}
            >
              <span className="block font-semibold">{t(title)}</span>
              <span className="mt-1 block text-sm text-[var(--color-muted)]">{t(help)}</span>
            </button>
          </li>
        ))}
      </ul>
    </Shell>
  );
}
