import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useBackoffPoll } from "./usePoll";

describe("useBackoffPoll", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("polls at increasing intervals capped at max while active", () => {
    const fn = vi.fn();
    renderHook(() => useBackoffPoll(true, fn, { minMs: 1000, maxMs: 4000, factor: 2 }));
    expect(fn).toHaveBeenCalledTimes(0); // nothing fires immediately
    vi.advanceTimersByTime(1000);
    expect(fn).toHaveBeenCalledTimes(1); // first poll after minMs
    vi.advanceTimersByTime(2000);
    expect(fn).toHaveBeenCalledTimes(2); // next after minMs*factor
    vi.advanceTimersByTime(4000);
    expect(fn).toHaveBeenCalledTimes(3); // grows, capped at maxMs
    vi.advanceTimersByTime(4000);
    expect(fn).toHaveBeenCalledTimes(4); // stays at the cap, not faster
  });

  it("never polls when inactive", () => {
    const fn = vi.fn();
    renderHook(() => useBackoffPoll(false, fn, { minMs: 1000 }));
    vi.advanceTimersByTime(10_000);
    expect(fn).toHaveBeenCalledTimes(0);
  });

  it("stops polling once active flips to false", () => {
    const fn = vi.fn();
    const { rerender } = renderHook(({ a }) => useBackoffPoll(a, fn, { minMs: 1000, maxMs: 4000, factor: 2 }), {
      initialProps: { a: true },
    });
    vi.advanceTimersByTime(1000);
    expect(fn).toHaveBeenCalledTimes(1);
    rerender({ a: false });
    vi.advanceTimersByTime(10_000);
    expect(fn).toHaveBeenCalledTimes(1); // no further polls after going inactive
  });
});
