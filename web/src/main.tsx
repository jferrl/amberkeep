import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "@/App";
import { LanguageProvider, languageOf, translator } from "@/i18n";
import "@/styles.css";

/**
 * An archive does not change while it is being read.
 *
 * The server holds one file open and nothing writes to it, so anything fetched
 * stays true until the program is closed. Refetching on a window focus or a
 * reconnect would be pure waste, and a reconnect is not even a thing here: there is
 * no network beyond this computer.
 */
const queries = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: Number.POSITIVE_INFINITY,
      gcTime: Number.POSITIVE_INFINITY,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
      retry: 1,
    },
  },
});

const language = languageOf(navigator.languages);
document.documentElement.lang = language;

const root = document.getElementById("root");
if (root === null) throw new Error("the page has no root to render into");

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queries}>
      <LanguageProvider value={translator(language)}>
        <App language={language} />
      </LanguageProvider>
    </QueryClientProvider>
  </StrictMode>,
);
