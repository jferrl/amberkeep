import { createContext, useContext } from "react";

/**
 * Where somebody can say thanks, if they want to.
 *
 * One address, carried from the state every screen already polls, so that the two
 * screens which offer it and the notice at the foot of the page all say the same
 * thing without any of them being handed it.
 *
 * Undefined means there is nowhere to point at, and everything that would show it
 * shows nothing: a program that sends somebody to a page which does not exist has
 * spent the only goodwill the offer was ever going to earn.
 */
const ThanksContext = createContext<string | undefined>(undefined);

export const ThanksProvider = ThanksContext.Provider;

/** useThanks is that address, when there is one. */
export function useThanks(): string | undefined {
  return useContext(ThanksContext);
}
