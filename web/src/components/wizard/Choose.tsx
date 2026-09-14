import { ChevronRight } from "lucide-react";

import { Aside, Say, Shell } from "@/components/wizard/Shell";
import type { Route } from "@/components/wizard/route";
import { useT } from "@/i18n";
import type { Phrase } from "@/i18n";

/**
 * The routes, in the order somebody is most likely to need them.
 *
 * Two of them need the part of the engine that reads phone backups, which can be
 * left out of a build. When it has been, they are not offered: a choice that answers
 * with an error is worse than a choice that was never there.
 */
const routes: {
  route: Route;
  title: Phrase;
  help: Phrase;
  needsImporter: boolean;
}[] = [
  {
    route: "backups",
    title: "routeBackup",
    help: "routeBackupHelp",
    needsImporter: true,
  },
  {
    route: "android",
    title: "routeAndroid",
    help: "routeAndroidHelp",
    needsImporter: true,
  },
  {
    route: "existing",
    title: "routeFile",
    help: "routeFileHelp",
    needsImporter: false,
  },
  // Gated on the same part of the engine as the two above. Reading phone backups and
  // writing one are the same half of the program, so a build without one has neither.
  {
    route: "migrate",
    title: "routeMigrate",
    help: "routeMigrateHelp",
    needsImporter: true,
  },
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

      {/*
        A list, not a grid of boxes. Four identically sized cards is what a page
        reaches for when it has four things and no opinion about them; these are four
        answers to one question, and a list is what a question's answers look like.
        The hairlines between them do the separating a border-box was doing, at a
        quarter of the weight.
      */}
      <ul className="m-0 flex list-none flex-col p-0">
        {offered.map(({ route, title, help }, at) => (
          <li
            key={route}
            className={at === 0 ? "" : "border-t border-[var(--color-line)]"}
          >
            <button
              type="button"
              className="group flex w-full items-center gap-4 rounded-md px-3 py-4 text-left transition-colors hover:bg-[var(--color-surface)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[var(--color-accent)]"
              onClick={() => {
                onChoose(route);
              }}
            >
              <span className="min-w-0 flex-1">
                <span className="block text-[0.9375rem] font-medium">
                  {t(title)}
                </span>
                <span className="mt-1 block text-[0.8125rem] text-[var(--color-muted)]">
                  {t(help)}
                </span>
              </span>
              <ChevronRight
                aria-hidden
                className="size-4 shrink-0 text-[var(--color-muted)] transition-transform group-hover:translate-x-0.5 group-hover:text-[var(--color-accent)]"
              />
            </button>
          </li>
        ))}
      </ul>
    </Shell>
  );
}
