import { existsSync, mkdtempSync, readFileSync, readdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { expect, test } from "./server";

/**
 * Keeping a copy, in a real browser, against the real binary.
 *
 * This is the one flow whose whole point is a file on disk, so a test that only
 * looked at the screen would prove nothing. What is checked here is the folder
 * afterwards: that it exists, that it holds pages, and that the pages hold the
 * conversation somebody came for.
 */

test("writes the archive out, and the files are really there", async ({ page, viewer }) => {
  const into = join(mkdtempSync(join(tmpdir(), "amberkeep-export-")), "archive");

  try {
    await page.goto(viewer.opening);
    await expect(page.getByRole("button", { name: /Ana Lopez/ })).toBeVisible({ timeout: 30_000 });

    await page.getByRole("button", { name: "Keep a copy" }).click();
    await expect(page.getByRole("heading", { name: /Keep a copy of your history/ })).toBeVisible();

    // Web pages are already chosen; plain text is not, and both are wanted here so
    // that the formats a person ticks are the formats that appear on disk.
    await page.getByLabel("Plain text").check();
    await page.getByLabel(/Where to put it/).fill(into);
    await page.getByRole("button", { name: "Write it out" }).click();

    await expect(page.getByRole("heading", { name: /Your history is on this computer/ })).toBeVisible({
      timeout: 60_000,
    });
    await expect(page.getByText(into)).toBeVisible();

    // The screen says it wrote files. This is the part that checks it did.
    expect(existsSync(into)).toBe(true);
    const written = readdirSync(into);
    expect(written.some((f) => f.endsWith(".html"))).toBe(true);
    expect(written.some((f) => f.endsWith(".txt"))).toBe(true);
    expect(written).toContain("index.html");

    // And a page holds the conversation, not an empty shell of one.
    const pages = written.filter((f) => f.endsWith(".html") && f !== "index.html");
    expect(pages.length).toBeGreaterThan(0);
    const contents = readFileSync(join(into, pages[0] ?? ""), "utf8");
    expect(contents).toContain("<!doctype html>");
    expect(contents.length).toBeGreaterThan(500);
  } finally {
    rmSync(into, { recursive: true, force: true });
  }
});

/**
 * The archive is what this program is for; a copy of it is a thing somebody takes
 * away. Asking for one must not put the archive itself out of reach.
 */
test("leaves the archive open behind it", async ({ page, viewer }) => {
  await page.goto(viewer.opening);
  await expect(page.getByRole("button", { name: /Ana Lopez/ })).toBeVisible({ timeout: 30_000 });

  await page.getByRole("button", { name: "Keep a copy" }).click();
  await expect(page.getByRole("heading", { name: /Keep a copy/ })).toBeVisible();

  await page.getByRole("button", { name: /Back/ }).click();
  await expect(page.getByRole("button", { name: /Ana Lopez/ })).toBeVisible();
});

test("will not write nothing", async ({ page, viewer }) => {
  await page.goto(viewer.opening);
  await expect(page.getByRole("button", { name: /Ana Lopez/ })).toBeVisible({ timeout: 30_000 });

  await page.getByRole("button", { name: "Keep a copy" }).click();
  await page.getByLabel("Web pages").uncheck();
  await page.getByRole("button", { name: "Write it out" }).click();

  await expect(page.getByText("Choose at least one thing to write.")).toBeVisible();
  await expect(page.getByRole("heading", { name: /Keep a copy/ })).toBeVisible();
});
