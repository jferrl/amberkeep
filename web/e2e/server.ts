import { test as base } from "@playwright/test";
import { spawn, type ChildProcessByStdio } from "node:child_process";
import type { Readable } from "node:stream";

import { build, type Archive } from "./archive";

/**
 * A real server, over a real archive, for the browser to talk to.
 *
 * These tests exist because everything else runs in jsdom, which has no layout and
 * therefore cannot tell whether a conversation of two hundred messages actually
 * scrolls. A browser can. So there is no mocking here at all: the Go binary is
 * built, started over an archive written to a temporary directory, and driven
 * through the same address it prints for a person.
 *
 * The address carries the launch secret, which is made fresh each time the server
 * starts and is the only way in. Reading it out of the server's own output is
 * exactly what a person does.
 */
const opening = /http:\/\/127\.0\.0\.1:\d+\/\?t=\S+/;

/** startedWithin is how long the server has to say it is ready. */
const startedWithin = 30_000;

export interface Viewer {
  /** opening is the address the server printed, secret and all. */
  opening: string;
  /** archive is where the archive was written, for a test that has to type it. */
  archive?: string;
  /** contacts is the address book beside it, which that same test has to type too. */
  contacts?: string;
}

/**
 * The server is started once per worker rather than once per test.
 *
 * Building an archive and starting a process for every assertion would take longer
 * than everything else here put together, and nothing any of these tests does
 * changes the archive: it is opened read-only and there is no way in through the
 * page to write to it.
 */
export const test = base.extend<object, { viewer: Viewer; wizard: Viewer }>({
  viewer: [
    // Playwright calls the second argument "use". It is named otherwise here
    // because a function called use, in a repository full of React, is read by
    // both people and linters as the hook it is not.
    // Playwright reads this destructuring pattern to work out which fixtures a
    // test needs, so it has to be written this way even when nothing is taken
    // from it. Naming a parameter instead makes the whole suite fail to load.
    // eslint-disable-next-line no-empty-pattern -- required by Playwright's API
    async ({}, run: (viewer: Viewer) => Promise<void>) => {
      const archive = build();
      const { server, address } = await start(archive);

      try {
        await run({ opening: address });
      } finally {
        server.kill();
        archive.remove();
      }
    },
    { scope: "worker" },
  ],

  /**
   * The same binary started the way somebody with a dead phone starts it: pointed at
   * nothing at all.
   *
   * An archive is built anyway, because the one thing worth proving in a browser is
   * that the whole wizard leads somewhere — that a path typed into a field ends with
   * a conversation on the screen.
   */
  wizard: [
    // eslint-disable-next-line no-empty-pattern -- required by Playwright's API
    async ({}, run: (viewer: Viewer) => Promise<void>) => {
      const archive = build();
      const { server, address } = await start(archive, { open: false });

      try {
        await run({ opening: address, archive: archive.path, contacts: archive.contacts });
      } finally {
        server.kill();
        archive.remove();
      }
    },
    { scope: "worker" },
  ],
});

export { expect } from "@playwright/test";

/**
 * start runs the binary and waits for it to print where it is listening.
 *
 * Without `open` it is started the way somebody who has nothing starts it, and the
 * page it serves is the wizard rather than an archive.
 */
async function start(
  archive: Archive,
  { open = true }: { open?: boolean } = {},
): Promise<{ server: ChildProcessByStdio<null, Readable, Readable>; address: string }> {
  const server: ChildProcessByStdio<null, Readable, Readable> = spawn(
    process.env.AMBERKEEP_BINARY ?? "../amberkeep",
    [
      "serve",
      ...(open ? ["--db", archive.path, "--contacts", archive.contacts, "--country", "34"] : []),
      "--no-open",
      "--no-search",
      "--port",
      "0",
    ],
    { stdio: ["ignore", "pipe", "pipe"] },
  );

  const address = await new Promise<string>((resolve, reject) => {
    const giveUp = setTimeout(() => {
      reject(new Error(`the server printed no address within ${String(startedWithin)}ms`));
    }, startedWithin);

    let said = "";
    const watch = (chunk: Buffer) => {
      said += chunk.toString();
      const found = opening.exec(said);
      if (found !== null) {
        clearTimeout(giveUp);
        resolve(found[0]);
      }
    };

    // Both streams are pipes, because that is how the process was started.
    server.stdout.on("data", watch);
    server.stderr.on("data", watch);
    server.on("error", (cause) => {
      clearTimeout(giveUp);
      reject(new Error(`the server would not start: ${cause.message}`));
    });
    server.on("exit", (code) => {
      clearTimeout(giveUp);
      reject(new Error(`the server stopped with ${String(code)} before saying anything:\n${said}`));
    });
  });

  return { server, address };
}
