import type { Message } from "@/api/types";

/**
 * describes is what a message was when it was not words: a file that is not in the
 * archive, a call, a place, a message that was withdrawn.
 *
 * The server sends both the words and a rendered line describing the message, and
 * for an ordinary message they are the same thing. Showing the description only
 * when it says something the text does not is what keeps a conversation from
 * repeating itself.
 */
export function describes(message: Message): string | undefined {
  if (message.rendered === "" || message.rendered === message.text)
    return undefined;
  return message.rendered;
}
