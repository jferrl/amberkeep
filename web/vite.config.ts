import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

// The build has one unusual requirement: everything the page needs must end up
// inside the files this produces. No CDN, no web font, no request to anywhere but
// the program that served the page. That is why assets are inlined rather than
// linked wherever they are small enough, and why nothing here reaches for a remote
// resource at build time either.
export default defineConfig({
  plugins: [
    // The React Compiler memoises what would otherwise be memoised by hand. It
    // earns its place here rather than being fashionable: a conversation renders
    // thousands of message components and re-renders them whenever anything above
    // changes, and the alternative is scattering memo and useCallback by hand until
    // somebody forgets one.
    react({ compiler: true }),
    tailwindcss(),
  ],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  build: {
    // Built straight into the Go package that embeds it. Building beside the source
    // instead would put the output inside a directory the Go tool is deliberately
    // kept out of, and embed cannot reach across that boundary anyway.
    outDir: "../internal/viewer/dist",
    emptyOutDir: true,
    // Anything under this size becomes a data URI rather than a second request.
    assetsInlineLimit: 8192,
    sourcemap: false,
    rollupOptions: {
      output: {
        entryFileNames: "assets/[name]-[hash].js",
        chunkFileNames: "assets/[name]-[hash].js",
        assetFileNames: "assets/[name]-[hash][extname]",
      },
    },
  },
  server: {
    // In development the Go server runs separately and holds the archive. Requests
    // are proxied to it so that the page is same-origin in development exactly as it
    // is in production, and the secret cookie therefore behaves the same way.
    proxy: {
      "/api": {
        target: process.env.AMBERKEEP_SERVER ?? "http://127.0.0.1:8080",
        changeOrigin: false,
      },
    },
  },
  test: {
    // The end-to-end tests belong to Playwright, which drives a real browser
    // against the real binary. Vitest would try to run them and fail.
    exclude: ["e2e/**", "node_modules/**", "dist/**"],
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    coverage: {
      provider: "v8",
      reporter: ["text", "lcov"],
      include: ["src/**/*.{ts,tsx}"],
      exclude: ["src/**/*.test.{ts,tsx}", "src/test/**", "src/main.tsx"],
    },
  },
});
