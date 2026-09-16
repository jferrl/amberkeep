import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { expect, test } from "./server";

/**
 * The wizard, in a real browser, against a real binary started with nothing open.
 *
 * This is the situation the program exists for: a phone that died, a file somebody
 * was told to copy, and nobody to sit next to them. jsdom covers what each screen
 * says; what it cannot cover is that the whole thing leads somewhere — that a path
 * typed into a field ends with a conversation on the screen, through a server that
 * really opened a database.
 */

test.beforeEach(async ({ page, wizard }) => {
  await page.goto(wizard.opening);
});

test("starts by asking what somebody has, rather than demanding a database", async ({ page }) => {
  await expect(page.getByRole("heading", { name: /Where is your WhatsApp history/ })).toBeVisible();

  // All three ways in are offered. Somebody who does not recognise any of them has
  // been failed before they have typed anything.
  await expect(page.getByRole("button", { name: /My iPhone is backed up/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /I have an Android phone/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /I already have a file/ })).toBeVisible();
});

/**
 * The same claim the viewer makes, on the screens where somebody is about to hand
 * over the key to their entire message history. A browser is the only place it can
 * be checked properly.
 */
test("talks to nothing but the program that served it", async ({ page, wizard }) => {
  const elsewhere: string[] = [];
  page.on("request", (request) => {
    const to = new URL(request.url());
    const home = new URL(wizard.opening);
    if (to.host !== home.host && to.protocol !== "data:") elsewhere.push(request.url());
  });

  await page.getByRole("button", { name: /I have an Android phone/ }).click();
  await expect(page.getByRole("heading", { name: /Turn on the encrypted backup/ })).toBeVisible();
  await page.waitForTimeout(500);

  expect(elsewhere).toEqual([]);
});

test("opens a database somebody already has, and shows the conversations in it", async ({
  page,
  wizard,
}) => {
  await page.getByRole("button", { name: /I already have a file/ }).click();

  const path = page.getByLabel(/The full path to the file/);
  await expect(path).toBeVisible();
  await path.fill(wizard.archive ?? "");
  // The address book is optional and is the difference between a conversation with
  // somebody's name on it and one labelled by phone number, which the README calls
  // the single most noticeable way an archive can disappoint.
  await page.getByLabel(/An address book/).fill(wizard.contacts ?? "");
  await page.getByRole("button", { name: "Open it" }).click();

  // Straight through the working screen and out the other side into the archive,
  // with the names applied.
  await expect(page.getByRole("button", { name: /Ana Lopez/ })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole("button", { name: /Close this archive/ })).toBeVisible();
});

/**
 * Failure is not the end of the road. Somebody who typed the wrong path has to be
 * able to see what went wrong and correct it, from the same screen, without
 * restarting the program.
 */
test("says what went wrong, and lets somebody correct it and go on", async ({ page }) => {
  const directory = mkdtempSync(join(tmpdir(), "amberkeep-wizard-"));
  const notAnArchive = join(directory, "holiday.jpg");
  writeFileSync(notAnArchive, Buffer.from([0xff, 0xd8, 0xff, 0xe0, 0xff, 0xd9]));

  try {
    await page.getByRole("button", { name: /I already have a file/ }).click();
    await page.getByLabel(/The full path to the file/).fill(notAnArchive);
    await page.getByRole("button", { name: "Open it" }).click();

    // The server's own sentence, above the form that caused it, with the several
    // lines of what to try underneath.
    await expect(page.getByRole("alert")).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText(/What to try/)).toBeVisible();
    await expect(page.getByText(/Correct whatever was wrong below/)).toBeVisible();

    // And the field still holds what was typed, so correcting it is a correction
    // rather than starting again.
    await expect(page.getByLabel(/The full path to the file/)).toHaveValue(notAnArchive);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

/**
 * Naming the phone's folder, and being told when it is the wrong one.
 *
 * An archive finds the folder beside the database on its own, which covers somebody
 * who copied the two across together. This is for everybody else: the database in
 * one place and the photographs in another. Typing a folder with nothing in it is
 * the mistake that actually happens — one level too deep, or too shallow — and being
 * told nothing would read as "there are no photographs", which is a different and
 * much worse thing to believe about your own history.
 */
test("refuses a folder with no WhatsApp files in it, and says which folder to name", async ({
  page,
  wizard,
}) => {
  const empty = mkdtempSync(join(tmpdir(), "amberkeep-nothing-"));

  try {
    await page.getByRole("button", { name: /I already have a file/ }).click();
    await page.getByLabel(/The full path to the file/).fill(wizard.archive ?? "");
    await page.getByLabel(/The phone's WhatsApp folder/).fill(empty);
    await page.getByRole("button", { name: "Open it" }).click();

    await expect(page.getByText(/no WhatsApp files in that folder/i)).toBeVisible({
      timeout: 30_000,
    });
    // And the advice says what to do about it rather than only that it failed. Named
    // by its own sentence: the field's hint says where the folder is on the phone
    // too, and a loose match would find both and prove neither.
    await expect(page.getByText(/name the one above instead/i)).toBeVisible();

    // The form is still there, with what was typed still in it, so it can be
    // corrected rather than started again.
    await expect(page.getByLabel(/The full path to the file/)).toHaveValue(wizard.archive ?? "");
  } finally {
    rmSync(empty, { recursive: true, force: true });
  }
});

test("opens with a folder somebody named, and shows the photographs", async ({ page, wizard }) => {
  // The archive's own directory: it holds the Media folder, which is what makes it
  // the phone's folder as far as this program is concerned.
  const folder = (wizard.archive ?? "").replace(/\/[^/]+$/, "");

  await page.getByRole("button", { name: /I already have a file/ }).click();
  await page.getByLabel(/The full path to the file/).fill(wizard.archive ?? "");
  await page.getByLabel(/An address book/).fill(wizard.contacts ?? "");
  await page.getByLabel(/The phone's WhatsApp folder/).fill(folder);
  await page.getByRole("button", { name: "Open it" }).click();

  await expect(page.getByRole("button", { name: /Vermut del sabado/ })).toBeVisible({
    timeout: 30_000,
  });
  await page.getByRole("button", { name: /Vermut del sabado/ }).click();
  await expect(page.getByRole("img", { name: /Photograph sent/ })).toBeVisible();
});
