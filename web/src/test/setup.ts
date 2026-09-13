import "@testing-library/jest-dom/vitest";
import { beforeAll } from "vitest";

/**
 * jsdom has no layout: every element is nought by nought and every scroll position
 * is zero. That is fine for most components and fatal for the two that are
 * virtualised, which ask how tall their viewport is, get nothing, and conclude that
 * no rows are visible.
 *
 * Giving the environment plausible dimensions is what lets those components be
 * tested at all. The numbers are a viewport and a row, chosen so that a handful of
 * rows fit: what is being tested is which rows appear and what they say, not
 * arithmetic about pixels.
 */
beforeAll(() => {
  const viewportHeight = 800;
  const rowHeight = 64;

  Object.defineProperty(HTMLElement.prototype, "clientHeight", {
    configurable: true,
    get(this: HTMLElement) {
      return this.hasAttribute("data-index") ? rowHeight : viewportHeight;
    },
  });
  Object.defineProperty(HTMLElement.prototype, "clientWidth", {
    configurable: true,
    get: () => 600,
  });
  Object.defineProperty(HTMLElement.prototype, "scrollHeight", {
    configurable: true,
    get: () => viewportHeight * 4,
  });

  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect(
    this: HTMLElement,
  ): DOMRect {
    const height = this.hasAttribute("data-index") ? rowHeight : viewportHeight;
    return {
      x: 0,
      y: 0,
      top: 0,
      left: 0,
      right: 600,
      bottom: height,
      width: 600,
      height,
      toJSON: () => ({}),
    };
  };

  /**
   * The virtualiser learns how tall its viewport is by observing it, and jsdom has
   * no such observer. A stub that does nothing is not enough: the virtualiser then
   * believes the viewport is nought tall and decides that no rows are visible,
   * which is exactly the failure this replaces. So it reports once, immediately,
   * with the dimensions faked above.
   */
  globalThis.ResizeObserver = class {
    private readonly notify: ResizeObserverCallback;

    constructor(notify: ResizeObserverCallback) {
      this.notify = notify;
    }

    observe(target: Element) {
      const box = target.getBoundingClientRect();
      const size = [{ inlineSize: box.width, blockSize: box.height }];
      this.notify(
        [
          {
            target,
            contentRect: box,
            borderBoxSize: size,
            contentBoxSize: size,
            devicePixelContentBoxSize: size,
          },
        ],
        this,
      );
    }

    unobserve() {
      /* nothing changes size without layout */
    }

    disconnect() {
      /* nothing changes size without layout */
    }
  };

  // A modal dialog is not implemented in jsdom, and Preview opens one.
  HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
    this.open = true;
  };
  HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
    this.open = false;
    this.dispatchEvent(new Event("close"));
  };
});
