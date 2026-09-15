import { createContext, useContext } from "react";

/**
 * The way back to the first screen, from wherever somebody is.
 *
 * Carried in a context rather than passed down because every screen in the wizard
 * would otherwise have to accept it and hand it on — five of them, plus the five
 * inside the migration — and a prop that every component takes and none of them uses
 * is how a wizard turns into a switchboard.
 *
 * Undefined means there is nowhere to go: the first screen itself, and any screen
 * where work is running, where offering to leave would be offering something the
 * program would not honour.
 */
const HomeContext = createContext<(() => void) | undefined>(undefined);

export const HomeProvider = HomeContext.Provider;

/** useHome is the way back, when there is one. */
export function useHome(): (() => void) | undefined {
  return useContext(HomeContext);
}
