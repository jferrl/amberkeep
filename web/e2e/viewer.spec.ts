import { expect, test } from "./server";

/**
 * The viewer, in a real browser, against a real server.
 *
 * Everything else this project tests runs in jsdom, which has no layout: it cannot
 * tell whether a conversation of two hundred messages scrolls, whether only the
 * visible rows exist in the document, or whether a page that says it fetches nothing
 * from elsewhere in fact fetches nothing from elsewhere. These are the tests that
 * can.
 */

test.beforeEach(async ({ page, viewer }) => {
  await page.goto(viewer.opening);
});

test.describe("opening an archive", () => {
  test("says what it holds and lists the conversations", async ({ page }) => {
    await expect(page.getByText(/2 conversations/)).toBeVisible();
    await expect(page.getByRole("button", { name: /Ana Lopez/ })).toBeVisible();
    await expect(page.getByRole("button", { name: /Vermut del sabado/ })).toBeVisible();
  });

  test("drops the secret from the address once it is in", ({ page }) => {
    // The secret arrives once and is kept in a cookie afterwards, so it is not left
    // sitting in the address bar or in whatever the browser remembers of it.
    expect(page.url()).not.toContain("?t=");
  });

  /**
   * The claim this whole program rests on. A page holding somebody's entire message
   * history must not talk to anybody, and a browser is the only place that can be
   * checked properly.
   */
  test("talks to nothing but the program that served it", async ({ page, viewer }) => {
    const elsewhere: string[] = [];
    page.on("request", (request) => {
      const to = new URL(request.url());
      const home = new URL(viewer.opening);
      if (to.host !== home.host && to.protocol !== "data:") elsewhere.push(request.url());
    });

    await page.getByRole("button", { name: /Ana Lopez/ }).click();
    await expect(page.getByText("message number 200")).toBeVisible();
    await page.getByRole("searchbox").fill("needle");
    await page.waitForTimeout(500);

    expect(elsewhere).toEqual([]);
  });
});

test.describe("reading a conversation", () => {
  test("opens at the end, where the last thing said is", async ({ page }) => {
    await page.getByRole("button", { name: /Ana Lopez/ }).click();

    await expect(page.getByText("message number 200")).toBeVisible();
    // The first message is two hundred rows up, so it is not on screen.
    await expect(page.getByText("message number 1", { exact: true })).not.toBeVisible();
  });

  /**
   * The reason the list is virtualised at all. A real conversation is ninety
   * thousand messages; if every one of them were in the document the browser would
   * stop responding. Two hundred is enough to prove only a window of them exists.
   */
  test("keeps only the messages near the reader in the document", async ({ page }) => {
    await page.getByRole("button", { name: /Ana Lopez/ }).click();
    await expect(page.getByText("message number 200")).toBeVisible();

    const rendered = await page.locator("article").count();
    expect(rendered).toBeGreaterThan(0);
    expect(rendered).toBeLessThan(200);
  });

  /**
   * Reaching the top asks the server for the messages before these, sixty at a time.
   * Getting from the last of two hundred to the first therefore exercises the whole
   * of it: the virtualiser, the paging, and the scroll position surviving rows being
   * added above the reader.
   */
  test("scrolls back through every page to the first thing said", async ({ page }) => {
    await page.getByRole("button", { name: /Ana Lopez/ }).click();
    const last = page.getByText("message number 200");
    await expect(last).toBeVisible();

    // The wheel turns wherever the pointer is, and the pointer starts in the corner,
    // which is the conversation list. It has to be over the conversation.
    await last.hover();

    const first = page.getByText("message number 1", { exact: true });
    for (let attempt = 0; attempt < 60; attempt++) {
      if (await first.isVisible()) break;
      await page.mouse.wheel(0, -1200);
      await page.waitForTimeout(120);
    }

    await expect(first).toBeVisible();
  });

  test("shows a picture that outlived its file", async ({ page }) => {
    await page.getByRole("button", { name: /Vermut del sabado/ }).click();

    const picture = page.getByRole("img", { name: /Picture recovered/ });
    await expect(picture).toBeVisible();
    // It came with the message rather than from anywhere.
    await expect(picture).toHaveAttribute("src", /^data:image\//);
    await expect(page.getByText(/recovered preview/i)).toBeVisible();
  });

  test("enlarges a picture and closes it again with the keyboard", async ({ page }) => {
    await page.getByRole("button", { name: /Vermut del sabado/ }).click();
    await page.getByRole("button", { name: /Show this picture larger/ }).click();

    const enlarged = page.getByRole("button", { name: /Close/ });
    await expect(enlarged).toBeVisible();

    await page.keyboard.press("Escape");
    await expect(enlarged).toBeHidden();
  });
});

test.describe("searching", () => {
  test("finds a message by a word in it, without an index", async ({ page }) => {
    // This archive is served without a search index, so the box searches names.
    await expect(page.getByRole("searchbox", { name: /Search conversation names/ })).toBeVisible();

    await page.getByRole("searchbox").fill("Vermut");
    await expect(page.getByRole("button", { name: /Vermut del sabado/ })).toBeVisible();
    await expect(page.getByRole("button", { name: /Ana Lopez/ })).toBeHidden();
  });

  test("says plainly when a name matches nothing", async ({ page }) => {
    await page.getByRole("searchbox").fill("tortuga");
    await expect(page.getByText(/No conversation by that name/)).toBeVisible();
  });
});

test.describe("reaching it by keyboard", () => {
  test("the search box can be reached and typed into", async ({ page }) => {
    await page.getByRole("searchbox").focus();
    await page.keyboard.type("Ana");
    await expect(page.getByRole("button", { name: /Ana Lopez/ })).toBeVisible();
  });

  test("a conversation can be opened without a mouse", async ({ page }) => {
    const conversation = page.getByRole("button", { name: /Ana Lopez/ });
    await conversation.focus();
    await page.keyboard.press("Enter");

    await expect(page.getByRole("heading", { name: "Ana Lopez" })).toBeVisible();
  });
});
