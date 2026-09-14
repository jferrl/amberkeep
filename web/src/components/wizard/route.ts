/**
 * The three ways somebody can have a WhatsApp history, and nothing else.
 *
 * They are written down here rather than beside the chooser so that the screen that
 * offers them and the screen that acts on one can both name the same thing without
 * importing each other.
 */
export type Route = "choose" | "backups" | "android" | "existing";

/**
 * What somebody has typed, and how far along they are.
 *
 * It lives here, and in the machine rather than in the screens, because the working
 * screen replaces whichever form started the work. A form holding its own answers
 * loses them the moment work begins — which is to say it loses them exactly when
 * something goes wrong, and that is when they are needed: a failure here is not the
 * end of the road, and the next attempt should be a correction rather than a fresh
 * start.
 *
 * `at` is how far along the Android walkthrough somebody has got. Being thrown back
 * to the first of five steps for mistyping a key is how people give up.
 *
 * The key is deliberately not here. It is sent and forgotten in the same breath, and
 * typing it again is a small price for it existing in one fewer place.
 */
export interface Typed {
  at: number;
  file: string;
  path: string;
}
