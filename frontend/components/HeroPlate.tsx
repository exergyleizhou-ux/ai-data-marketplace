"use client";

import { useT } from "@/lib/i18n";
import { Seal } from "@/components/Seal";

/**
 * The hero's signature visual: a compact schematic of the core value prop —
 * the dataset stays put, an audited algorithm travels into the sandbox, and a
 * sealed, aggregate-only result emerges. The seal is the only gold; the
 * algorithm token animates in once (reduced-motion → static).
 */
export function HeroPlate() {
  const { t } = useT();
  return (
    <div className="elev relative overflow-hidden rounded-2xl border border-rule bg-white p-6">
      <p className="font-mono text-kicker uppercase tracking-widest text-forest-700">
        {t("可信计算 · 示意", "compute-to-data · schematic")}
      </p>

      <div className="mt-5 space-y-3">
        {/* DATASET — stays */}
        <div className="flex items-center gap-3 rounded-lg border border-rule bg-paper px-3 py-2.5">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="#18181b" strokeWidth="2" aria-hidden>
            <rect x="3" y="11" width="18" height="11" rx="2" />
            <path d="M7 11V7a5 5 0 0 1 10 0v4" />
          </svg>
          <div className="min-w-0">
            <p className="truncate font-mono text-xs text-ink">DATASET · 08a8b1…</p>
            <p className="font-mono text-[10px] text-muted">{t("从不离开沙箱", "never leaves the sandbox")}</p>
          </div>
        </div>

        {/* SANDBOX — algorithm travels in */}
        <div className="rounded-lg border-2 border-dashed border-forest bg-forest-50/40 px-3 py-3">
          <p className="font-mono text-[10px] uppercase tracking-widest text-forest-700">
            {t("沙箱 · 数据不出域", "SANDBOX · --network=none")}
          </p>
          <div className="hero-travel mt-1.5 inline-flex items-center gap-1.5 rounded-full border border-ink/15 bg-white px-2.5 py-1">
            <span className="font-mono text-[11px] text-ink">{t("已审核算法", "AUDITED ALGORITHM")}</span>
            <span className="text-forest" aria-hidden>→</span>
          </div>
        </div>

        {/* CERTIFICATE — sealed result emerges */}
        <div className="flex items-center justify-between gap-3 rounded-lg border border-rule bg-white px-3 py-2.5">
          <div className="min-w-0">
            <p className="font-mono text-xs text-gold-700">VO-6CB8181EBD2C</p>
            <p className="font-mono text-[10px] text-muted">{t("只出聚合结果 + 封缄", "aggregate result + seal only")}</p>
          </div>
          <div className="shrink-0">
            <Seal size={40} label={t("可验证封缄", "verified seal")} />
          </div>
        </div>
      </div>

      <p className="mt-4 font-mono text-[10px] leading-relaxed text-muted">
        {t("算法走向数据,而非数据被搬走。", "the algorithm moves to the data — not the reverse.")}
      </p>
    </div>
  );
}
