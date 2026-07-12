import { RefObject, useEffect, useState } from "react";

// useScrollFade reports whether a scrollable viewport still has content hidden
// below the fold — i.e. it actually overflows AND isn't scrolled to the bottom.
// Drives the bottom-fade overlays so they only appear when they signal
// something real: a list that fits without scrolling, or one scrolled to its
// end, shows no fade. Re-measures on scroll, on element resize, and whenever
// `deps` change (content swaps that don't resize the box, e.g. pagination).
export function useScrollFade(
  ref: RefObject<HTMLElement | null>,
  deps: readonly unknown[] = []
): boolean {
  const [faded, setFaded] = useState(false);

  useEffect(() => {
    const el = ref.current;
    const measure = () => {
      const node = ref.current;
      // 1px slack: scrollTop can land on fractional values at non-1x zoom.
      setFaded(
        node ? node.scrollHeight - node.scrollTop - node.clientHeight > 1 : false
      );
    };
    measure();
    if (!el) return;
    el.addEventListener("scroll", measure, { passive: true });
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => {
      el.removeEventListener("scroll", measure);
      ro.disconnect();
    };
    // `deps` re-runs the effect when the caller's content changes shape; the
    // spread is intentional.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ref, ...deps]);

  return faded;
}
