import { useEffect, useRef } from "react";

/**
 * useBackoffPoll calls `fn` on a self-slowing schedule while `active` is true.
 *
 * Compute panels previously polled on a fixed 1.5–1.8s setInterval for as long as
 * a job was in flight — needless load when a job runs for minutes. This polls
 * fast at first (minMs), then backs off by `factor` up to `maxMs`, and stops
 * entirely when `active` goes false. A fresh active period (e.g. a new job) starts
 * the backoff over from minMs.
 */
export function useBackoffPoll(
  active: boolean,
  fn: () => void,
  opts: { minMs?: number; maxMs?: number; factor?: number } = {},
) {
  const { minMs = 1500, maxMs = 6000, factor = 1.5 } = opts;
  const fnRef = useRef(fn);
  useEffect(() => {
    fnRef.current = fn;
  }, [fn]);

  useEffect(() => {
    if (!active) return;
    let delay = minMs;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const tick = () => {
      fnRef.current();
      delay = Math.min(delay * factor, maxMs);
      if (!cancelled) timer = setTimeout(tick, delay);
    };
    timer = setTimeout(tick, delay);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [active, minMs, maxMs, factor]);
}
