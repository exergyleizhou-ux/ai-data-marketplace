"use client";

import { useT } from "@/lib/i18n";
import { Seal } from "@/components/Seal";

// Signature C2D flow, redrawn as a schematic plate, not a flowchart:
//   - Stage numbers in mono (technical authority, like a paper figure)
//   - Display-serif stage titles (editorial gravitas)
//   - The platform-sandbox boundary is a long thin dashed rule, not a rounded box
//   - The certificate carries a gold wax-seal mark (the only gold on the homepage)
//   - Bilingual via useT; intrinsic 720x260 + overflow-x-auto wrapper for mobile.
export function ComputeFlowDiagram() {
  const { t } = useT();

  // Stage layout in viewBox coords. Spread out for breathing room.
  const STAGES = [
    {
      n: "01",
      title: t("数据留下", "Data stays"),
      role: t("卖方域", "seller domain"),
      caption: t("从不下载,不外传", "never downloaded or shipped"),
      x: 110,
      titleColor: "#18181b",
    },
    {
      n: "02",
      title: t("算法走入", "Algorithm enters"),
      role: t("平台沙箱", "platform sandbox"),
      caption: t("经审核的镜像 + DP 噪声 + 输出闸门", "audited image · DP noise · output gate"),
      x: 360,
      titleColor: "#047857",
    },
    {
      n: "03",
      title: t("结果出域", "Result emerges"),
      role: t("买方收件", "buyer's receipt"),
      caption: t("绑定镜像 digest 的凭证", "certificate bound to image digest"),
      x: 610,
      titleColor: "#18181b",
    },
  ];

  return (
    <div>
      <div className="hidden overflow-x-auto sm:block">
        <svg
          viewBox="0 0 720 260"
          width="720"
          height="260"
          role="img"
          aria-label={t(
            "C2D 三阶段流程图:数据留下、算法走入沙箱、结果出域并带凭证",
            "C2D three-stage flow: data stays, the algorithm enters a sandbox, the result emerges with a certificate",
          )}
          className="mx-auto block"
        >
          {/* Long thin connecting rule across all three stages */}
          <line x1="50" y1="120" x2="670" y2="120" stroke="#e7e5e0" strokeWidth="1" />

          {/* Platform sandbox boundary — a dashed bracket around stage 2,
              communicating "this is where the action happens, inside walls" */}
          <line x1="260" y1="44" x2="460" y2="44" stroke="#047857" strokeWidth="1" strokeDasharray="3 3" />
          <line x1="260" y1="44" x2="260" y2="180" stroke="#047857" strokeWidth="1" strokeDasharray="3 3" />
          <line x1="460" y1="44" x2="460" y2="180" stroke="#047857" strokeWidth="1" strokeDasharray="3 3" />
          <line x1="260" y1="180" x2="460" y2="180" stroke="#047857" strokeWidth="1" strokeDasharray="3 3" />
          <text
            x="360"
            y="36"
            textAnchor="middle"
            fontSize="9"
            fontFamily="var(--font-mono)"
            letterSpacing="0.12em"
            fill="#047857"
          >
            {t("沙箱边界 · DATA STAYS HOME", "SANDBOX BOUNDARY · DATA STAYS HOME")}
          </text>

          {/* Three stage nodes */}
          {STAGES.map((s, i) => (
            <g key={s.n}>
              {/* Stage anchor dot on the connecting rule */}
              <circle cx={s.x} cy="120" r="5" fill="#fafaf7" stroke="#18181b" strokeWidth="1.5" />
              {i === 1 && <circle cx={s.x} cy="120" r="2.5" fill="#047857" />}
              {i === 2 && <circle cx={s.x} cy="120" r="2.5" fill="#b45309" />}

              {/* Stage number kicker (mono) */}
              <text
                x={s.x}
                y="80"
                textAnchor="middle"
                fontSize="10"
                fontFamily="var(--font-mono)"
                letterSpacing="0.16em"
                fill="#71717a"
              >
                STAGE {s.n}
              </text>
              {/* Title (serif display) */}
              <text
                x={s.x}
                y="106"
                textAnchor="middle"
                fontSize="20"
                fontFamily="var(--font-display)"
                fill={s.titleColor}
              >
                {s.title}
              </text>
              {/* Role (small mono uppercase) */}
              <text
                x={s.x}
                y="142"
                textAnchor="middle"
                fontSize="10"
                fontFamily="var(--font-mono)"
                letterSpacing="0.14em"
                fill="#71717a"
              >
                {s.role.toUpperCase()}
              </text>
              {/* Caption */}
              <text x={s.x} y="162" textAnchor="middle" fontSize="11.5" fill="#52525b">
                {s.caption}
              </text>
            </g>
          ))}

          {/* Gold seal mark on Stage 3 — the only gold on the page,
              tied to the verifiable certificate */}
          <g transform="translate(665 100)">
            <circle r="9" fill="none" stroke="#b45309" strokeWidth="1" />
            <text
              y="3"
              textAnchor="middle"
              fontSize="9"
              fontFamily="var(--font-mono)"
              fontWeight="500"
              fill="#b45309"
            >
              ✓
            </text>
          </g>

          {/* Direction arrows — minimal, ink-colored */}
          <g fill="#a1a1aa">
            <polygon points="245,118 235,113 235,123" />
            <polygon points="485,118 475,113 475,123" />
          </g>

          {/* Bottom footnote — the L-tier honesty in mono */}
          <text
            x="360"
            y="220"
            textAnchor="middle"
            fontSize="11"
            fontFamily="var(--font-mono)"
            letterSpacing="0.06em"
            fill="#71717a"
          >
            {t(
              "L1 沙箱  ·  L2 TEE 连平台也不可见  ·  L3 数据不出域(联邦 / PSI)",
              "L1 SANDBOX  ·  L2 TEE INVISIBLE TO PLATFORM  ·  L3 DATA-STAYS-HOME (FED / PSI)",
            )}
          </text>
        </svg>
      </div>

      {/* Mobile: the same schematic plate reflowed vertically — fully legible,
          no horizontal scroll (this is the most-shared viewport). */}
      <div className="relative sm:hidden">
        <span className="absolute bottom-3 left-[9px] top-3 w-px bg-rule" aria-hidden />
        <ol className="space-y-5 pl-7">
          {STAGES.map((s, i) => (
            <li key={s.n} className="relative">
              <span
                className="absolute -left-7 top-1.5 flex h-4 w-4 items-center justify-center rounded-full border-2 border-ink bg-paper"
                aria-hidden
              >
                {i === 1 && <span className="h-1.5 w-1.5 rounded-full bg-forest" />}
                {i === 2 && <span className="h-1.5 w-1.5 rounded-full bg-gold" />}
              </span>
              <div className={i === 1 ? "-ml-3 border-l-2 border-dashed border-forest pl-3" : ""}>
                <p className="font-mono text-[10px] uppercase tracking-widest text-muted">
                  STAGE {s.n}
                  {i === 1 ? ` · ${t("沙箱", "SANDBOX")}` : ""}
                </p>
                <div className="flex items-center gap-2">
                  <span className="font-display text-xl leading-tight" style={{ color: s.titleColor }}>
                    {s.title}
                  </span>
                  {i === 2 && <Seal size={26} animate={false} label={t("可验证封缄", "verified seal")} />}
                </div>
                <p className="font-mono text-[10px] uppercase tracking-wider text-muted">{s.role.toUpperCase()}</p>
                <p className="mt-0.5 text-xs leading-relaxed text-ink/70">{s.caption}</p>
              </div>
            </li>
          ))}
        </ol>
        <p className="mt-5 font-mono text-[10px] leading-relaxed tracking-wide text-muted">
          {t(
            "L1 沙箱 · L2 TEE 连平台也不可见 · L3 数据不出域(联邦 / PSI)",
            "L1 SANDBOX · L2 TEE INVISIBLE TO PLATFORM · L3 DATA-STAYS-HOME (FED / PSI)",
          )}
        </p>
      </div>
    </div>
  );
}
