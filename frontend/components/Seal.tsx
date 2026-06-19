/**
 * A wax-seal verification mark — the signature "verified" cue. Gold (the brand's
 * seal accent), milled edge, an embossed/pressed-into-the-page feel (cast shadow +
 * highlight), and a checkmark that draws in as it stamps.
 */
export function Seal({ size = 76, animate = true, label }: { size?: number; animate?: boolean; label?: string }) {
  const ticks = Array.from({ length: 40 }, (_, i) => {
    const a = (i / 40) * Math.PI * 2;
    return {
      x1: 50 + 33 * Math.cos(a), y1: 50 + 33 * Math.sin(a),
      x2: 50 + 38 * Math.cos(a), y2: 50 + 38 * Math.sin(a),
    };
  });
  return (
    <svg width={size} height={size} viewBox="0 0 100 100" className={animate ? "seal-anim" : ""} role="img" aria-label={label || "verified seal"}>
      <defs>
        <radialGradient id="vo-wax" cx="38%" cy="34%" r="78%">
          <stop offset="0%" stopColor="#e0982f" />
          <stop offset="52%" stopColor="#b45309" />
          <stop offset="100%" stopColor="#7c3a06" />
        </radialGradient>
        <radialGradient id="vo-wax-hi" cx="36%" cy="30%" r="45%">
          <stop offset="0%" stopColor="#ffffff" stopOpacity="0.30" />
          <stop offset="100%" stopColor="#ffffff" stopOpacity="0" />
        </radialGradient>
        <filter id="vo-seal-shadow" x="-30%" y="-30%" width="160%" height="160%">
          <feDropShadow dx="0" dy="1.4" stdDeviation="2" floodColor="#7c3a06" floodOpacity="0.4" />
        </filter>
      </defs>
      <g filter="url(#vo-seal-shadow)">
        {ticks.map((t, i) => (
          <line key={i} x1={t.x1} y1={t.y1} x2={t.x2} y2={t.y2} stroke="#b45309" strokeOpacity="0.55" strokeWidth="1.4" />
        ))}
        <circle cx="50" cy="50" r="34" fill="url(#vo-wax)" />
        {/* pressed lip: a dark inner-shadow ring + a soft top-left highlight */}
        <circle cx="50" cy="50" r="33" fill="none" stroke="#5c2a04" strokeOpacity="0.45" strokeWidth="1.6" />
        <circle cx="50" cy="50" r="34" fill="url(#vo-wax-hi)" />
        <circle cx="50" cy="50" r="27" fill="none" stroke="#ffffff" strokeOpacity="0.42" strokeWidth="1.3" />
        <path
          className={animate ? "seal-check" : ""}
          d="M38 51 l8.5 8.5 L64 41"
          fill="none"
          stroke="#fff8ef"
          strokeWidth="4.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </g>
    </svg>
  );
}
