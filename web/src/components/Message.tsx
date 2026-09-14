import { memo } from "react";

import type { Message as ArchivedMessage } from "@/api/types";
import {
  Attribution,
  LinkCard,
  PollBody,
  Quoted,
  Reactions,
  RecoveredPicture,
  Tags,
} from "@/components/MessageParts";
import type { Language } from "@/i18n";
import { describes } from "@/lib/message";
import { cn } from "@/lib/utils";

/**
 * One message.
 *
 * It reads as a list of what a message might carry, because that is what a message
 * is: words, or a picture that outlived its file, or a poll, or the fact that
 * somebody withdrew what they said. Each piece decides for itself whether it has
 * anything to show; see MessageParts.
 *
 * Everything rendered is a text node. A message is words somebody else wrote and
 * this page shows it in a browser, so nothing it contains may ever become markup.
 *
 * It is memoised because a conversation renders thousands of these and only the few
 * around the scroll position change.
 */
export const Message = memo(function Message({
  message,
  language,
}: {
  message: ArchivedMessage;
  language: Language;
}) {
  const notice = message.kind === "system";
  const mine = message.from_me;
  const description = describes(message);

  return (
    <article
      className={cn(
        "message-row my-1.5 flex",
        mine && "justify-end",
        notice && "justify-center",
      )}
      data-sent-at={message.sent_at}
    >
      <div
        className={cn(
          "min-w-0 max-w-[85%] rounded-2xl border px-3 py-1.5",
          "border-[var(--color-line)] break-words",
          // The rule, not the fill, is what makes a bubble a shape. The fill says
          // which side said it; the rule says where it starts and stops.
          "ring-1 ring-[var(--color-line)] ring-inset",
          mine ? "bg-[var(--color-mine)]" : "bg-[var(--color-theirs)]",
          notice &&
            "max-w-[90%] border-dashed bg-transparent text-center text-[0.82rem] text-[var(--color-muted)]",
        )}
      >
        {!notice && <Attribution message={message} language={language} />}
        {message.reply_to !== undefined && <Quoted quote={message.reply_to} />}
        <RecoveredPicture message={message} />

        {description !== undefined && (
          <div className="italic text-[var(--color-muted)]">{description}</div>
        )}
        {message.text !== undefined && message.poll === undefined && (
          <div className="mt-0.5 whitespace-pre-wrap">{message.text}</div>
        )}

        <PollBody message={message} />
        <LinkCard message={message} />
        <Reactions message={message} />
        <Tags message={message} />
      </div>
    </article>
  );
});
