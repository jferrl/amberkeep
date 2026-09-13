import js from "@eslint/js";
import jsxA11y from "eslint-plugin-jsx-a11y";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import globals from "globals";
import tseslint from "typescript-eslint";

// Written without the `i` flag: esquery's attribute matcher is not guaranteed to
// carry flags through, so the classes spell both cases out. Matches "https://x",
// "http://x" and the protocol-relative "//x", which is the one people forget.
const absoluteURL = String.raw`/^(?:[a-zA-Z][a-zA-Z0-9+.-]*:)?\/\//`;

// Rule 1 of this project is that nothing it produces makes a network call. In Go
// that is provable by reading the imports; in a browser it is not, because every
// one of these is a single line somebody adds without thinking about it. The
// messages exist so the person who hits one understands what it would have cost,
// rather than reaching for an eslint-disable.
const zeroNetwork = [
  {
    selector: `CallExpression[callee.name="fetch"] > Literal[value=${absoluteURL}]`,
    message:
      "This page may only talk to the loopback server that served it. An absolute URL leaves the machine, and tells whoever owns that host that somebody is reading a WhatsApp archive. Use a path beginning with a slash.",
  },
  {
    selector: `CallExpression[callee.name="fetch"] > TemplateLiteral > TemplateElement:first-child[value.raw=${absoluteURL}]`,
    message:
      "This page may only talk to the loopback server that served it. A template literal that begins with a host leaves the machine just as a string does. Use a path beginning with a slash.",
  },
  {
    selector: `NewExpression[callee.name="Request"] > Literal[value=${absoluteURL}]`,
    message:
      "A Request built against another host is a network call waiting for a fetch. The archive is served from this page's own origin.",
  },
  {
    selector: `ImportExpression > Literal[value=${absoluteURL}]`,
    message:
      "A module imported at run time is fetched from a host that can change what it returns tomorrow. Every dependency is bundled at build time so that the build is the whole program.",
  },
  {
    selector: "MemberExpression[property.name='sendBeacon']",
    message:
      "sendBeacon is telemetry under another name. This program reports nothing about anybody to anybody, and that promise is the reason people trust it with their conversations.",
  },
  {
    selector: `JSXAttribute[name.name=/^(src|srcSet|poster|action|formAction)$/] > Literal[value=${absoluteURL}]`,
    message:
      "An asset loaded from another host tells that host who is reading this archive, and lets it change what arrives. Every font, icon and image is vendored into the repository and served from this origin.",
  },
  {
    selector: `JSXOpeningElement[name.name="link"] > JSXAttribute[name.name="href"] > Literal[value=${absoluteURL}]`,
    message:
      "A stylesheet or preload from another host is a network call and a tracking pixel at once. There is no CDN and no web font here; the build carries everything it needs.",
  },
  {
    selector: "JSXAttribute[name.name='dangerouslySetInnerHTML']",
    message:
      "A message is text somebody else wrote, years ago, with no idea it would be rendered in a browser. dangerouslySetInnerHTML is how that text becomes script running in this page. React escapes text by default; there is no exception worth making here.",
  },
  {
    selector: "Property[key.name='dangerouslySetInnerHTML']",
    message:
      "A message is text somebody else wrote. Passing it as dangerouslySetInnerHTML through createElement is the same hole as the JSX attribute, and is banned for the same reason.",
  },
  {
    selector:
      "AssignmentExpression[left.property.name=/^(innerHTML|outerHTML)$/]",
    message:
      "Assigning HTML to an element parses whatever the archive contained as markup. Set textContent, or render the value through React.",
  },
  {
    selector: "MemberExpression[property.name='insertAdjacentHTML']",
    message:
      "insertAdjacentHTML parses archive content as markup. Set textContent, or render the value through React.",
  },
];

const zeroNetworkGlobals = [
  {
    name: "XMLHttpRequest",
    message:
      "XMLHttpRequest survives here only as a way around the rule on fetch. Everything this page reads comes from the loopback server that served it.",
  },
  {
    name: "WebSocket",
    message:
      "A WebSocket is a connection this page has no reason to hold open. The archive is read over the loopback HTTP API and nothing pushes to us.",
  },
  {
    name: "EventSource",
    message:
      "An EventSource is a long-lived connection to a server. The archive is read over the loopback HTTP API and nothing pushes to us.",
  },
  {
    name: "importScripts",
    message:
      "importScripts fetches and runs code at run time from wherever it is pointed. Everything this page runs is in the build.",
  },
];

export default tseslint.config(
  {
    ignores: [
      "test-results/",
      "playwright-report/","**/dist/**", "**/node_modules/**", "**/coverage/**"],
  },
  {
    files: ["**/*.{ts,tsx}"],
    extends: [
      js.configs.recommended,
      // Type-checked because the interesting mistakes in this codebase are about
      // values that might not be there: a chat with no messages, a message with
      // no text, a contact with no name. Untyped linting cannot see any of them.
      tseslint.configs.strictTypeChecked,
      tseslint.configs.stylisticTypeChecked,
      // v7 ships the React Compiler rules inside this preset, so memoisation
      // mistakes and impure renders fail the build rather than being reviewed for.
      reactHooks.configs.flat["recommended-latest"],
      reactRefresh.configs.vite,
      // Somebody reading their own history may well be doing it with a screen
      // reader, on a bad day, because a person died. Accessibility is a gate.
      jsxA11y.flatConfigs.recommended,
    ],
    languageOptions: {
      ecmaVersion: 2024,
      globals: globals.browser,
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      // Already an error under strictTypeChecked. Stated again because it is the
      // rule most often relaxed "just here", and this is where that argument ends.
      "@typescript-eslint/no-explicit-any": "error",
      // Upstream ships this as a warning. A dependency array that has gone
      // stale is a bug that shows up as a message list that will not refresh,
      // which is indistinguishable from data loss to the person reading it.
      "react-hooks/exhaustive-deps": "error",
      "no-restricted-syntax": ["error", ...zeroNetwork],
      "no-restricted-globals": ["error", ...zeroNetworkGlobals],
    },
  },
  {
    files: ["**/*.{js,mjs,cjs}"],
    extends: [js.configs.recommended, tseslint.configs.disableTypeChecked],
    languageOptions: { globals: globals.node },
  },
  {
    files: [
      "**/*.{test,spec}.{ts,tsx}",
      "**/__tests__/**/*.{ts,tsx}",
      "src/test/**/*.{ts,tsx}",
    ],
    // Only rules that exist to protect production code are relaxed. The network
    // and dangerouslySetInnerHTML bans live in no-restricted-syntax and
    // no-restricted-globals, which are deliberately not named here: naming either
    // one would replace the whole list rather than trim it, and a test is exactly
    // where somebody would first reach for a real URL.
    rules: {
      "@typescript-eslint/no-non-null-assertion": "off",
      "@typescript-eslint/no-unsafe-argument": "off",
      "@typescript-eslint/no-unsafe-assignment": "off",
      "@typescript-eslint/no-unsafe-call": "off",
      "@typescript-eslint/no-unsafe-member-access": "off",
      "@typescript-eslint/no-empty-function": "off",
      // Assertion helpers are passed about unbound on purpose.
      "@typescript-eslint/unbound-method": "off",
      // A test file exports nothing the dev server would ever hot-reload.
      "react-refresh/only-export-components": "off",
    },
  },
);
