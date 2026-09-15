import type { ReactNode } from "react";

import { Notices } from "@/components/Notices";
import { Thanks } from "@/components/Thanks";
import { useT } from "@/i18n";

/**
 * The window around a screen that takes the whole of it.
 *
 * Three things belong to every such screen and to none of them in particular: the
 * promise that nothing leaves this computer, whatever the server last refused, and
 * the two notices this program has to make about itself. They were the wizard's,
 * written into it, so the screen that writes an archive out — which also takes the
 * whole window — had none of them: no promise, no notices, and in a window the
 * close, minimise and zoom buttons drawn over its first line of text.
 *
 * The band at the top of the frame is the window's own. --frame is how tall it is,
 * and it is zero in a browser; giving the strip that height puts the sentence on the
 * same line as those buttons instead of eight pixels above them, and the side
 * padding keeps it clear of them when the window is narrow.
 */
export function Framed({
  children,
  refused,
}: {
  children: ReactNode;
  /** What the server would not do, when it has just said so. */
  refused?: string | undefined;
}) {
  const t = useT();

  return (
    <div className="flex min-h-dvh flex-col bg-[var(--color-bg)]">
      <p className="m-0 flex min-h-[max(2.25rem,var(--frame))] items-center justify-center border-b border-[var(--color-line)] bg-[var(--color-surface)] px-[calc(var(--frame)+1.25rem)] text-center text-sm text-[var(--color-muted)]">
        {t("readOnly")}
      </p>

      {refused !== undefined && (
        <p
          role="alert"
          className="m-0 border-b border-[var(--color-accent)] px-5 py-2 text-center text-sm font-medium"
        >
          {t("refused", { detail: refused })}
        </p>
      )}

      {children}

      {/*
        At the foot of every screen, above the notices: somebody looking for a way to
        say thanks should find one without having to finish something first.
      */}
      <div className="mx-auto mt-auto w-full max-w-2xl px-5 pb-4">
        <Thanks />
      </div>

      <Notices />
    </div>
  );
}
