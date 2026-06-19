"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";

/**
 * Settles its children in like ink when they scroll into view. Self-contained:
 * content above the fold (or with JS/motion disabled) stays in the default
 * "idle" state — rendered immediately, never hidden — so there is no flash and no
 * no-JS content loss. Only a below-the-fold element with motion enabled is hidden
 * then animated in when observed.
 */
export function Reveal({
  children,
  delay = 0,
  as: Tag = "div",
  className = "",
}: {
  children: ReactNode;
  delay?: number;
  as?: "div" | "section" | "li";
  className?: string;
}) {
  const ref = useRef<HTMLElement>(null);
  const [state, setState] = useState<"idle" | "hidden" | "in">("idle");

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    // Reduced motion or already in/above view → stay "idle" (visible, no animation).
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    if (el.getBoundingClientRect().top < window.innerHeight * 0.92) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setState("hidden");
    const obs = new IntersectionObserver(
      (entries) => entries.forEach((e) => {
        if (e.isIntersecting) {
          setState("in");
          obs.disconnect();
        }
      }),
      { threshold: 0.12 },
    );
    obs.observe(el);
    // Safety net: never leave a node stuck at opacity:0 if the observer misfires.
    const t = setTimeout(() => setState((s) => (s === "hidden" ? "in" : s)), 1600);
    return () => {
      obs.disconnect();
      clearTimeout(t);
    };
  }, []);

  const cls = `${state === "hidden" ? "reveal-hidden" : ""} ${state === "in" ? "reveal-in" : ""} ${className}`.trim();
  const style = delay ? { animationDelay: `${delay}ms` } : undefined;
  return (
    <Tag
      ref={(node: HTMLElement | null) => {
        ref.current = node;
      }}
      className={cls}
      style={style}
    >
      {children}
    </Tag>
  );
}
