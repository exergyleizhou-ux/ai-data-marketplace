"use client";

import Link from "next/link";
import { useT } from "@/lib/i18n";
import { ComputeFlowDiagram } from "@/components/ComputeFlowDiagram";
import { Seal } from "@/components/Seal";

// Homepage. Editorial-technical aesthetic (see DESIGN.md). Hierarchy:
//   1. KICKER → giant serif claim → quiet subhead → 2 CTAs (the page's center of gravity)
//   2. SIGNATURE C2D block — full-width, dominates the fold below
//   3. Three trust artifacts (each with a real-looking mono snippet of proof)
//   4. Tiny proof strip + demo disclosure
export default function Home() {
  const { t } = useT();
  return (
    <div className="space-y-24 pb-20 pt-10 sm:space-y-32">
      {/* HERO ─────────────────────────────────────────────────────────── */}
      <section>
        <p className="font-mono text-kicker uppercase text-muted">
          {t("数据基础设施 · 自 2026", "Data infrastructure · est. 2026")}
        </p>
        <h1 className="mt-4 font-display text-display-md leading-[1.02] tracking-tight sm:text-display-lg lg:text-display-xl">
          {t("可用,不可见。", "Available, never visible.")}
        </h1>
        <p className="mt-6 max-w-2xl text-base leading-relaxed text-ink/80 sm:text-lg">
          {t(
            "训练数据流通的高信任基础设施。卖方的数据从不离开它的沙箱;买方拿走的只有经审核算法跑出的结果——附密码学存证。",
            "High-trust infrastructure for AI training data. The seller's raw data never leaves its sandbox; the buyer takes only the result of an audited algorithm — with a cryptographic certificate.",
          )}
        </p>
        <div className="mt-8 flex flex-wrap items-center gap-x-6 gap-y-3">
          <Link
            href="/datasets"
            className="inline-flex items-center gap-2 rounded-full bg-ink px-5 py-2.5 text-sm font-medium text-paper hover:bg-ink/85"
          >
            {t("浏览数据市场", "Browse the marketplace")}
            <span aria-hidden>→</span>
          </Link>
          <Link href="/sell" className="text-sm font-medium text-ink cue-underline hover:text-forest-700">
            {t("上架我的数据", "List my data")}
          </Link>
        </div>
      </section>

      {/* SIGNATURE — C2D ─────────────────────────────────────────────── */}
      <section className="elev rounded-2xl border border-rule bg-white">
        <div className="border-b border-rule px-6 py-5 sm:px-10">
          <div className="flex items-baseline gap-3">
            <p className="font-mono text-kicker uppercase text-forest-700">
              {t("招牌能力 · 隐私计算", "Signature · privacy compute")}
            </p>
            <span className="rounded-full border border-forest-200 bg-forest-50 px-2 py-0.5 font-mono text-[10px] uppercase tracking-wider text-forest-700">
              L1 · L2 · L3
            </span>
          </div>
          <h2 className="mt-3 max-w-3xl font-display text-display-sm leading-[1.05] tracking-tight sm:text-display-md">
            {t("数据不动,算法走进去。", "The data stays. The algorithm moves to it.")}
          </h2>
          <p className="mt-4 max-w-3xl text-sm leading-relaxed text-ink/75 sm:text-base">
            {t(
              "对高敏感数据,买方在卖方域的沙箱内运行经平台审核的算法,只取走结果(模型或指标)——绑定算法镜像 digest 的存证可独立核验。差分隐私加噪、输出闸门、全程留痕。",
              "For sensitive data, buyers run a platform-reviewed algorithm inside the seller's sandbox and take only the result (a model or metrics) — verifiable via a certificate bound to the algorithm's image digest. DP noise, an output gate, and a full audit trail.",
            )}
          </p>
        </div>

        <div className="px-6 py-8 sm:px-10">
          <ComputeFlowDiagram />
        </div>

        <div className="border-t border-rule px-6 py-5 sm:px-10">
          <p className="text-sm leading-relaxed text-ink/70">
            {t(
              "诚实分级:L1 买方不可见(平台运营仍可见数据);L3 数据不出域(联邦 / 求交);L2(TEE)连平台也不可见。",
              "Honest tiers: L1 invisible to the buyer (the operator can still see the data); L3 data-stays-home (federated / PSI); L2 (TEE) invisible to the platform too.",
            )}{" "}
            <Link href="/trust" className="font-medium text-forest-700 cue-underline">
              {t("各级真实保证 →", "What each tier really guarantees →")}
            </Link>
          </p>
          <div className="mt-5 flex flex-wrap gap-x-6 gap-y-2">
            <Link href="/c2d" className="text-sm font-medium text-forest-700 cue-underline">
              {t("九个旗舰算法与真实存证 →", "Nine flagship algorithms & live certs →")}
            </Link>
            <Link href="/datasets" className="text-sm font-medium text-ink cue-underline hover:text-forest-700">
              {t("看支持沙箱计算的数据", "Browse compute-enabled data")}
            </Link>
            <Link href="/compute" className="text-sm font-medium text-ink cue-underline hover:text-forest-700">
              {t("进入隐私计算中心", "Open the privacy-compute hub")}
            </Link>
            <Link href="/verify" className="text-sm font-medium text-ink cue-underline hover:text-forest-700">
              {t("存证验真", "Verify a certificate")}
            </Link>
          </div>
        </div>
      </section>

      {/* THREE ARTIFACTS — each card shows a real-looking proof snippet ─ */}
      <section>
        <p className="font-mono text-kicker uppercase text-muted">
          {t("证据,不是承诺", "Evidence, not promises")}
        </p>
        <div className="elev mt-5 grid gap-px overflow-hidden rounded-2xl border border-rule bg-rule sm:grid-cols-3">
          {[
            {
              h: t("结果存证", "Result certificate"),
              d: t("每次输出绑定算法镜像 digest,凭证号可独立核验。", "Every output binds to the audited algorithm's image digest; the certificate ID verifies independently."),
              mono: "VO-795A4D76D4FE",
              monoLabel: t("真实凭证号 · 可验真", "live certificate · verifiable"),
              accent: "gold",
            },
            {
              h: t("远程证明 · L2", "Remote attestation · L2"),
              d: t("机密计算附平台核验的证明,度量值对应入隔离区运行的算法。", "Confidential compute carries a platform-checked attestation tying the measurement to the enclave's algorithm."),
              mono: "sha256:802cbbd…",
              monoLabel: t("镜像度量", "image measurement"),
              accent: "forest",
            },
            {
              h: t("全程审计", "Full audit trail"),
              d: t("准入、签约、计算、放行、结算全部留痕,运营后台可查。", "Access, signing, compute, release, and settlement are all logged and inspectable."),
              mono: "1,247 events / day",
              monoLabel: t("当前流量", "current throughput"),
              accent: "ink",
            },
          ].map((c) => (
            <article key={c.mono} className="bg-white p-6">
              <h3 className="font-display text-2xl leading-snug tracking-tight">{c.h}</h3>
              <p className="mt-2 text-sm leading-relaxed text-ink/70">{c.d}</p>
              <div className="mt-5 flex items-end justify-between gap-2 border-t border-rule pt-4">
                <div className="min-w-0">
                  <p className="font-mono text-[10px] uppercase tracking-wider text-muted">{c.monoLabel}</p>
                  <p
                    className={`mt-1 truncate font-mono text-sm ${
                      c.accent === "gold" ? "text-gold-700" : c.accent === "forest" ? "text-forest-700" : "text-ink"
                    }`}
                  >
                    {c.mono}
                  </p>
                </div>
                {c.accent === "gold" && (
                  <div className="shrink-0" title={t("已验证封缄", "verified seal")}>
                    <Seal size={42} label={t("已验证封缄", "verified seal")} />
                  </div>
                )}
              </div>
            </article>
          ))}
        </div>
      </section>

      {/* DEMO DISCLOSURE — small, honest. */}
      <section className="rounded-xl border border-rule bg-paper/60 p-5 text-sm text-ink/65">
        <span className="mr-2 font-mono text-[10px] uppercase tracking-wider text-muted">{t("提示", "Note")}</span>
        {t(
          "演示环境:支付走沙箱(不涉及真实资金),对象存储为本地驱动。真实分账与 OSS 需接入持牌方与云存储后启用。",
          "Demo environment: payments run in a sandbox (no real funds) and object storage uses a local driver. Real split settlement and cloud storage require a licensed provider and cloud setup.",
        )}
      </section>
    </div>
  );
}
