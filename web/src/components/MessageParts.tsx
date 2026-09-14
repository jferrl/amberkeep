import type { Message as ArchivedMessage, Quote } from "@/api/types";
import { Preview } from "@/components/Preview";
import { useT } from "@/i18n";
import type { Language } from "@/i18n";
import { timeOfDay } from "@/lib/format";

/**
 * The pieces a message is made of.
 *
 * They live apart from Message because a message can carry a dozen different things
 * and a single component that decides between all of them is one nobody can read.
 * Each piece here answers one question and returns nothing when it has no answer,
 * so Message becomes a list of what a message might contain rather than a thicket
 * of conditions.
 *
 * Everything is a text node. A message is words somebody else wrote and this page
 * renders it in a browser, so nothing it contains may ever become markup.
 */

/** Who wrote a message, and when. */
export function Attribution({
  message,
  language,
}: {
  message: ArchivedMessage;
  language: Language;
}) {
  const t = useT();
  const who = message.from_me
    ? t("you")
    : (message.sender_name ?? message.sender ?? t("unknownSender"));

  return (
    <div>
      <span className="text-[0.8rem] font-semibold text-[var(--color-accent)]">
        {who}
      </span>
      <time
        className="ml-1.5 text-[0.72rem] text-[var(--color-muted)]"
        dateTime={message.sent_at}
      >
        {timeOfDay(message.sent_at, language)}
      </time>
    </div>
  );
}

/** What a reply was answering, as it travelled with the reply. */
export function Quoted({ quote }: { quote: Quote }) {
  const t = useT();

  return (
    <blockquote className="my-1 rounded border-l-[3px] border-[var(--color-accent)] bg-[color-mix(in_srgb,var(--color-accent)_8%,transparent)] px-2 py-1 text-[0.85rem] text-[var(--color-muted)]">
      <span className="text-[0.78rem] font-semibold text-[var(--color-accent)]">
        {quote.from_me ? t("you") : (quote.sender_name ?? t("unknownSender"))}
      </span>
      <div className="whitespace-pre-wrap">
        {quote.text ?? `<${quote.kind}>`}
      </div>
    </blockquote>
  );
}

/** The picture that survived its own file, when one did. */
export function RecoveredPicture({ message }: { message: ArchivedMessage }) {
  const preview = message.attachment?.preview_base64;
  if (preview === undefined) return null;

  return (
    <Preview base64={preview} mediaType={message.attachment?.media_type} />
  );
}

/** A poll's question and what people could choose. */
export function PollBody({ message }: { message: ArchivedMessage }) {
  const t = useT();
  if (message.poll === undefined) return null;

  return (
    <div className="mt-1 text-[0.88rem]">
      <div className="whitespace-pre-wrap">{message.poll.question}</div>
      {(message.poll.options ?? []).map((option) => (
        <div key={option.name} className="flex justify-between gap-2 py-px">
          <span>{option.name}</span>
          <span className="whitespace-nowrap text-[var(--color-muted)]">
            {option.votes === 1
              ? t("vote")
              : t("votes", { count: option.votes })}
          </span>
        </div>
      ))}
    </div>
  );
}

/**
 * What a page said at the moment it was shared.
 *
 * This is often the only surviving record of it: the page itself may have changed
 * or gone years ago.
 */
export function LinkCard({ message }: { message: ArchivedMessage }) {
  if (message.link === undefined) return null;

  return (
    <div className="mt-1 rounded border border-[var(--color-line)] px-2 py-1 text-[0.85rem]">
      {message.link.title !== undefined && (
        <div className="font-semibold">{message.link.title}</div>
      )}
      {message.link.description !== undefined && (
        <div>{message.link.description}</div>
      )}
      {message.link.url !== undefined && (
        <div className="text-[0.78rem] text-[var(--color-accent)]">
          {message.link.url}
        </div>
      )}
    </div>
  );
}

/** Who reacted, and with what. */
export function Reactions({ message }: { message: ArchivedMessage }) {
  const t = useT();
  const reactions = message.reactions ?? [];
  if (reactions.length === 0) return null;

  return (
    <div className="mt-1 flex flex-wrap gap-1">
      {reactions.map((reaction, at) => (
        <span
          key={`${reaction.emoji}-${String(at)}`}
          className="rounded-full border border-[var(--color-line)] bg-[var(--color-bg)] px-1.5 text-[0.78rem]"
        >
          {reaction.emoji}{" "}
          {reaction.from_me ? t("you") : (reaction.sender_name ?? "")}
        </span>
      ))}
    </div>
  );
}

/** The small facts under a message: edited, forwarded, starred. */
export function Tags({ message }: { message: ArchivedMessage }) {
  const t = useT();

  const tags: string[] = [];
  if (message.edited_at !== undefined) tags.push(t("edited"));
  if (message.forwarded === true) tags.push(t("forwarded"));
  if (message.starred === true) tags.push(t("starred"));
  if (tags.length === 0) return null;

  return (
    <div className="mt-0.5 text-[0.72rem] text-[var(--color-muted)]">
      {tags.join(" · ")}
    </div>
  );
}
