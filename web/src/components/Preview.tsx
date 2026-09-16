import { useCallback, useEffect, useRef, useState } from "react";

import { useT } from "@/i18n";

/**
 * A picture in a conversation, and a way to see it larger.
 *
 * It shows one of two things, and says which. WhatsApp keeps a small copy of a
 * photograph inside the message database, and for an old archive that is usually the
 * only image of it left anywhere — that one arrives as base64 in the message itself,
 * so there is no second request and nothing to fetch, which is also what lets an
 * exported page work with the network switched off. When somebody brings the phone's
 * folder as well, the photograph itself is here, and this fetches it from the
 * program rather than carrying it in the message.
 *
 * Showing either is the single most valuable thing this viewer does, which is why it
 * gets its own component rather than being a thumbnail nobody can read.
 */
export function Preview({
  base64,
  source: given,
  mediaType,
  recovered = true,
}: {
  base64?: string | undefined;
  /** Where the picture is, for one this program can fetch whole. */
  source?: string | undefined;
  mediaType?: string | undefined;
  /**
   * Whether this is the small copy that survived inside the database rather than
   * the file itself. It is said out loud under the picture, because the difference
   * matters to somebody looking at their own history: one is what was sent, the
   * other is all that is left of it.
   */
  recovered?: boolean;
}) {
  const t = useT();
  const [enlarged, setEnlarged] = useState(false);
  const dialog = useRef<HTMLDialogElement>(null);

  // The type is told to us when it is known. Guessing wrong shows a broken picture
  // where a recovered one should be, and JPEG is what WhatsApp writes almost
  // without exception.
  const source =
    given ?? `data:${mediaType ?? "image/jpeg"};base64,${base64 ?? ""}`;
  const alt = recovered ? t("pictureAlt") : t("photographAlt");

  const close = useCallback(() => {
    setEnlarged(false);
  }, []);

  useEffect(() => {
    const element = dialog.current;
    if (element === null) return;
    if (enlarged && !element.open) element.showModal();
    if (!enlarged && element.open) element.close();
  }, [enlarged]);

  return (
    <>
      <button
        type="button"
        onClick={() => {
          setEnlarged(true);
        }}
        className="my-1 block cursor-zoom-in rounded-lg bg-[var(--color-line)]"
        aria-label={t("enlargePreview")}
      >
        {/*
          The alt says what the picture is rather than being empty, because this is
          content and not decoration: for most of these the original file is long
          gone and this is the only image of it left. What it depicts is unknowable
          from here, so it says where it came from instead — and it says which of the
          two this is, because "recovered from this message" would be a lie about a
          photograph whose own file is sitting on the disk.
        */}
        <img
          src={source}
          alt={alt}
          loading="lazy"
          className="block h-auto max-w-full rounded-lg"
        />
      </button>
      {recovered && (
        <div className="text-[0.72rem] text-[var(--color-muted)]">
          {t("recoveredPreview")}
        </div>
      )}

      {/*
        Escape closes this without any help, because a modal dialog does that
        itself; onClose is what keeps this component's own state in step with it.
        The way out by mouse is the button below rather than a click handler on the
        dialog, so that it is reachable by keyboard and announced, which a clickable
        backdrop is not.
      */}
      <dialog
        ref={dialog}
        onClose={close}
        aria-label={t("enlargePreview")}
        className="max-h-[96vh] max-w-[96vw] border-none bg-transparent p-0 backdrop:bg-black/80"
      >
        {enlarged && (
          <button
            type="button"
            onClick={close}
            className="block cursor-zoom-out"
            aria-label={t("closePreview")}
          >
            <img
              src={source}
              alt={alt}
              className="max-h-[96vh] max-w-[96vw] rounded"
            />
          </button>
        )}
      </dialog>
    </>
  );
}
