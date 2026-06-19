"use client";

import { useEffect, useRef, useState } from "react";

/**
 * Tweens an integer up from 0 the first time it scrolls into view, so a proof
 * point (a count) reads as a value that just resolved rather than static decor.
 * Reduced-motion → renders the final value immediately.
 */
export function CountUp({ to, durationMs = 1100 }: { to: number; durationMs?: number }) {
  const ref = useRef<HTMLSpanElement>(null);
  const [val, setVal] = useState(0);
  const started = useRef(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setVal(to);
      return;
    }
    const obs = new IntersectionObserver(
      (entries) => entries.forEach((e) => {
        if (e.isIntersecting && !started.current) {
          started.current = true;
          const start = performance.now();
          const tick = (now: number) => {
            const p = Math.min(1, (now - start) / durationMs);
            const eased = 1 - Math.pow(1 - p, 3);
            setVal(p < 1 ? to * eased : to);
            if (p < 1) requestAnimationFrame(tick);
          };
          requestAnimationFrame(tick);
          obs.disconnect();
        }
      }),
      { threshold: 0.5 },
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, [to, durationMs]);

  return <span ref={ref}>{Math.round(val).toLocaleString()}</span>;
}
