"use client";

/** MiniChart — zero-dependency SVG sparkline for timeseries data.
 *  Props:
 *    points: {date:string; value:number}[]
 *    color?:  stroke colour (default "#3b82f6")
 *    height?: SVG height in px (default 60)
 *    label?:  accessible label
 *    className?: wrapper class */

export function MiniChart({
  points,
  color = "#3b82f6",
  height = 60,
  label,
  className = "",
}: {
  points: { date: string; value: number }[];
  color?: string;
  height?: number;
  label?: string;
  className?: string;
}) {
  if (!points || points.length === 0) {
    return <div className={`flex items-center justify-center text-xs text-neutral-300 ${className}`} style={{ height }}>—</div>;
  }

  const w = 300;
  const h = height;
  const pad = 2;
  const vals = points.map((p) => p.value);
  const min = Math.min(0, ...vals);
  const max = Math.max(...vals, 1);
  const range = max - min || 1;

  const xStep = points.length > 1 ? (w - 2 * pad) / (points.length - 1) : 0;
  const y = (v: number) => h - pad - ((v - min) / range) * (h - 2 * pad);

  const d = points
    .map((p, i) => `${i === 0 ? "M" : "L"} ${pad + i * xStep} ${y(p.value)}`)
    .join(" ");

  return (
    <div className={className} aria-label={label}>
      <svg
        viewBox={`0 0 ${w} ${h}`}
        width="100%"
        height={h}
        role="img"
        className="overflow-visible"
      >
        <path d={d} fill="none" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
        {/* tiny dots for each point */}
        {points.map((p, i) => (
          <circle
            key={i}
            cx={pad + i * xStep}
            cy={y(p.value)}
            r={1.5}
            fill={color}
          />
        ))}
      </svg>
    </div>
  );
}
