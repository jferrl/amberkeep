import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { expect, test } from "./server";

/**
 * Moving a history onto a phone, in a real browser, against the real binary.
 *
 * jsdom covers what each screen does with an answer. What it cannot cover is that
 * the answers are the ones the program actually gives: the checks come out of
 * `internal/migrate`, and the sentences telling somebody what to do about a failed
 * check come out of `internal/guide`, which is Go source with no copy on this side.
 * If either drifts, this is the test that notices.
 *
 * Nothing here writes anything. The furthest it goes is asking the server to look at
 * a folder, which it opens read-only.
 */

/**
 * Every address the page asked for, from before it was opened.
 *
 * A listener attached in the body of a test has already missed whatever the first
 * screen asked for on the way in. This was a fixture, on the belief that fixtures are
 * set up before the hooks — they are not: one the hooks do not themselves use is
 * created lazily, just before the test body, which is after the navigation below. It
 * passed here and failed on a CI runner, where the request it watches for lands
 * during the hook rather than after it.
 *
 * Hooks run in the order they are declared, so this one is attached first and sees
 * everything. Module state is safe: tests in one file share a worker and run one at a
 * time.
 */
const watched: string[] = [];

test.beforeEach(({ page }) => {
  watched.length = 0;
  page.on("request", (request) => {
    watched.push(request.url());
  });
});

test.beforeEach(async ({ page, wizard }) => {
  await page.goto(wizard.opening);

  // Every test here starts with the server holding nothing. How far along a
  // migration is belongs to the server and survives a reload — which is the right
  // behaviour, and it means one test that stops halfway decides what the next one
  // opens on. Said once, here, rather than left to the order the tests happen to run
  // in. The address carries the launch secret in a cookie by now, from the line above.
  const letGo = await page.request.post(
    new URL("/api/migration/forget", wizard.opening).toString(),
    { data: {} },
  );
  expect(letGo.ok()).toBe(true);

  await page.getByRole("button", { name: /move an Android history onto my iPhone/ }).click();
});

test("is offered as a way in, and opens on what it cannot promise", async ({ page }) => {
  await expect(page.getByRole("heading", { name: /What to move, and where to/ })).toBeVisible();

  // Said before anything is typed, not buried at the end. Somebody about to do this
  // is entitled to know it has never been done end to end.
  await expect(
    page.getByText(/No backup Amberkeep has produced has yet been restored to a phone/),
  ).toBeVisible();
  // The page makes that promise once, on the strip above every screen, rather than
  // three times in one viewport.
  await expect(page.getByText(/Nothing leaves this computer/)).toBeVisible();
});

/**
 * The same claim the rest of the program makes, on the screens that handle a whole
 * message history and end with a phone being written to.
 */
test("talks to nothing but the program that served it", async ({ page, wizard }) => {
  await expect(page.getByLabel(/The iPhone backup folder/)).toBeVisible();
  await page.waitForTimeout(500);

  const home = new URL(wizard.opening);
  const elsewhere = watched.filter((address) => {
    const to = new URL(address);
    return to.host !== home.host && to.protocol !== "data:";
  });
  expect(elsewhere).toEqual([]);
});

/**
 * The check, all the way through the real engine.
 *
 * A plain directory is not a backup, and the program says so as a finding rather
 * than as a crash: something to go and put right. What is being proved is the whole
 * length of it — the request, `migrate.Preflight`, the finding, the stage, the
 * screen — and then that the button which would do the work is shut.
 */
test("refuses to plan past a folder that is not a backup, and says what to do instead", async ({
  page,
}) => {
  const directory = mkdtempSync(join(tmpdir(), "amberkeep-migration-"));

  try {
    await page.getByLabel(/The iPhone backup folder/).fill(directory);
    await page.getByLabel(/The decrypted Android database/).fill(join(directory, "msgstore.db"));
    await page.getByRole("button", { name: "Look at the backup" }).click();

    await expect(page.getByRole("heading", { name: /What could be checked/ })).toBeVisible({
      timeout: 30_000,
    });

    // The engine's own words about what it found.
    await expect(
      page.getByText(/none of the four files every Finder, iTunes and Apple Devices backup has/),
    ).toBeVisible();

    // And `internal/guide`'s own words about what to do, which exist nowhere on this
    // side of the wire. A guide served from Go and rendered here is the only reason
    // there is one copy of these sentences rather than two.
    await expect(
      page.getByText(/Back up to this Mac once more/, { exact: false }),
    ).toBeVisible();

    // Nothing can be planned until that is put right.
    await expect(page.getByRole("button", { name: "Work out what would move" })).toBeDisabled();
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

/**
 * A migration that was started and abandoned is not a migration somebody is stuck
 * in. Going back has to leave the server with nothing held.
 */
test("lets somebody go back and change what they named", async ({ page }) => {
  const directory = mkdtempSync(join(tmpdir(), "amberkeep-migration-"));

  try {
    await page.getByLabel(/The iPhone backup folder/).fill(directory);
    await page.getByLabel(/The decrypted Android database/).fill(join(directory, "msgstore.db"));
    await page.getByRole("button", { name: "Look at the backup" }).click();

    await expect(page.getByRole("heading", { name: /What could be checked/ })).toBeVisible({
      timeout: 30_000,
    });
    await page.getByRole("button", { name: "Change something" }).click();

    // Back at the form, and the server has let go of the attempt: a reload lands
    // here too rather than back on the checks.
    await expect(page.getByLabel(/The iPhone backup folder/)).toBeVisible();
    await page.reload();
    await expect(page.getByRole("heading", { name: /Where is your WhatsApp history/ })).toBeVisible();
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

/**
 * The screen asks the program what backups it already has, rather than asking the
 * person to go and find one.
 *
 * What comes back depends on the machine — a developer's Mac has real backups, a CI
 * runner has none and may not even be allowed to look — so what is asserted is that
 * it asked, and that typing a path stays possible whatever the answer was.
 */
test("asks the program for the backups it has already made", async ({ page }) => {
  await expect(page.getByLabel(/The iPhone backup folder/)).toBeVisible();
  await expect
    .poll(() => watched.map((address) => new URL(address).pathname))
    .toContain("/api/backups");

  // Whatever it found, the path can still be typed: an external disk or a copy
  // somebody made is not in that list and is perfectly good.
  await expect(page.getByLabel(/The iPhone backup folder/)).toBeEditable();
});

test("will not look at a backup nobody has named", async ({ page }) => {
  await page.getByRole("button", { name: "Look at the backup" }).click();

  await expect(page.getByText("Say where the file is before going on.").first()).toBeVisible();
  await expect(page.getByRole("heading", { name: /What to move, and where to/ })).toBeVisible();
});
