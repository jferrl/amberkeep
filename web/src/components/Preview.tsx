import { useCallback, useEffect, useRef, useState } from "react";

import { useT } from "@/i18n";

/**
 * A picture that survived its own file.
 *
 * WhatsApp keeps a small copy of a photograph inside the message database, and for
 * an old archive that is usually the only image of it left anywhere. Showing it is
 * the single most valuable thing this viewer does, which is why it gets its own
 * component and a way to see it larger rather than being a thumbnail nobody can
 * read.
 *
 * The bytes arrive as base64 in the message itself, so there is no second request
 * and nothing to fetch. That is also what lets an exported page work with the
 * network switched off.
 */
export function Preview({
  base64,
  mediaType,
}: {
  base64: string;
  mediaType?: string | undefined;
}) {
  const t = useT();
  const [enlarged, setEnlarged] = useState(false);
  const dialog = useRef<HTMLDialogElement>(null);

  // The type is told to us when it is known. Guessing wrong shows a broken picture
  // where a recovered one should be, and JPEG is what WhatsApp writes almost
  // without exception.
  const source = `data:${mediaType ?? "image/jpeg"};base64,${base64}`;

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
          from here, so it says where it came from instead.
        */}
        <img
          src={source}
          alt={t("pictureAlt")}
          loading="lazy"
          className="block h-auto max-w-full rounded-lg"
        />
      </button>
      <div className="text-[0.72rem] text-[var(--color-muted)]">
        {t("recoveredPreview")}
      </div>

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
              alt={t("pictureAlt")}
              className="max-h-[96vh] max-w-[96vw] rounded"
            />
          </button>
        )}
      </dialog>
    </>
  );
}
