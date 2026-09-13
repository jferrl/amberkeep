import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as reactRender, type RenderResult } from "@testing-library/react";
import type { ReactElement, ReactNode } from "react";

import { LanguageProvider, translator, type Language } from "@/i18n";

/**
 * render puts a component inside what it needs to exist: a language, and a query
 * client that never retries so a failing test fails now rather than in a second.
 */
export function render(ui: ReactElement, language: Language = "en"): RenderResult {
  const queries = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queries}>
        <LanguageProvider value={translator(language)}>{children}</LanguageProvider>
      </QueryClientProvider>
    );
  }

  return reactRender(ui, { wrapper: Wrapper });
}
