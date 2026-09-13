import { defineConfig, devices } from "@playwright/test";

/**
 * The end-to-end tests run against the real binary, not a development server.
 *
 * That is the point of them: everything else this project tests runs in jsdom,
 * which has no layout and cannot tell whether a long conversation actually scrolls
 * or whether the page fetches anything from elsewhere. What runs here is exactly
 * what somebody would run, including the viewer as it was built into the binary.
 */
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: process.env.CI !== undefined,
  retries: process.env.CI !== undefined ? 1 : 0,
  reporter: process.env.CI !== undefined ? "list" : [["list"]],
  use: {
    trace: "retain-on-failure",
    // There is no base address: each worker starts its own server on a port the
    // system picks, and the address it prints carries the secret to get in.
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
