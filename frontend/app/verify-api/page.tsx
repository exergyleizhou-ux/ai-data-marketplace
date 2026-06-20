"use client";

import Link from "next/link";
import { useState } from "react";
import { api } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { PageHeader, Card } from "@/components/ui";
import { Reveal } from "@/components/Reveal";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080/api/v1";
const PRO_PRICE_ID = process.env.NEXT_PUBLIC_STRIPE_PRO_PRICE_ID ?? "";

const STEPS = [
  { n: "01", zh: { t: "拿一个 API key", d: "登录后在控制台自助签发(免费档即开即用)。" }, en: { t: "Get an API key", d: "Self-issue one in the console after sign-in (free tier, instant)." } },
  { n: "02", zh: { t: "POST 你的数据集", d: "把 CSV 发到 /screen 端点;数据在断网沙箱里筛查,只出聚合,原始行不外泄。" }, en: { t: "POST your dataset", d: "Send a CSV to /screen; it's screened in a network-severed sandbox — aggregates only, raw rows never leave." } },
  { n: "03", zh: { t: "拿报告 + 可验证证书", d: "返回完整性报告 + 一张可重算核验、可公开 /verify 的证书。" }, en: { t: "Get a report + a verifiable cert", d: "Back comes an integrity report + a re-hashable, publicly /verify-able certificate." } },
];

const TIERS = [
  { name: "Free", price: "$0", zh: { q: "5 次/月,≤5MB", who: "试用、个人项目" }, en: { q: "5 scans/mo · ≤5MB", who: "trial, personal projects" }, cta: false },
  { name: "Pro", price: "$29", zh: { q: "500 次/月,≤100MB", who: "ML 团队、数据工程" }, en: { q: "500 scans/mo · ≤100MB", who: "ML teams, data engineering" }, cta: true },
  { name: "Scale", price: "Custom", zh: { q: "高量 / 私有部署", who: "数据供应商、企业" }, en: { q: "high volume / private", who: "data vendors, enterprise" }, cta: false },
];

export default function VerifyApiLanding() {
  const { t, lang } = useT();
  const L = <T,>(o: { zh: T; en: T }) => (lang === "en" ? o.en : o.zh);
  const [busy, setBusy] = useState(false);

  // Upgrade to Pro: when a Stripe price is configured, start a checkout and
  // redirect; otherwise send the visitor to get a (free) key first.
  async function upgradePro() {
    if (!PRO_PRICE_ID) {
      window.location.href = "/account";
      return;
    }
    setBusy(true);
    try {
      const { checkout_url } = await api.verifyCheckout(PRO_PRICE_ID);
      window.location.href = checkout_url;
    } catch {
      window.location.href = "/account"; // not logged in / billing off → get a key
    } finally {
      setBusy(false);
    }
  }

  const curl = `curl -X POST ${API_BASE}/screen \\
  -H "X-API-Key: sk_live_…" \\
  -F "file=@your-dataset.csv"`;

  return (
    <div className="max-w-3xl space-y-12 pb-24">
      <PageHeader
        kicker={t("Oasis Verify · 验证 API", "Oasis Verify · verification API")}
        title={t("证明你数据集的质量与来源", "Prove your dataset's quality & provenance")}
        subtitle={t(
          "上传一个数据集,拿回一份完整性体检报告 + 一张任何人都能重算核验的证书——把结果指纹钉死到审计算法与源数据。给训练前/采购前的数据把关,API 优先,按量付费。",
          "Upload a dataset, get back an integrity screen + a certificate anyone can re-hash to verify — binding the result to an audited algorithm and the source data. Vet data before you train or buy. API-first, pay-as-you-go.",
        )}
      />

      {/* Why */}
      <Reveal className="rounded-xl border border-forest-200 bg-forest-50/40 p-5">
        <p className="text-sm leading-relaxed text-ink/80">
          {t(
            "坏数据很贵:训练废掉、论文被撤、采购踩雷。本地能跑检查,但拿不到一张可分享、第三方可验证的凭证——那正是 Oasis Verify 提供的。",
            "Bad data is expensive — wasted training runs, retracted papers, procurement risk. You can run checks locally, but you can't produce a shareable, third-party-verifiable certificate. That's what Oasis Verify gives you.",
          )}
        </p>
      </Reveal>

      {/* How */}
      <section className="space-y-4">
        <h2 className="font-display text-xl tracking-tight text-ink">{t("三步", "Three steps")}</h2>
        <div className="grid gap-3 sm:grid-cols-3">
          {STEPS.map((s, i) => (
            <Reveal key={s.n} delay={i * 60}>
              <Card className="lift h-full">
                <div className="font-mono text-kicker uppercase tracking-widest text-forest-700">{s.n}</div>
                <div className="mt-1 font-medium text-ink">{L(s).t}</div>
                <p className="mt-1 text-xs leading-relaxed text-ink/70">{L(s).d}</p>
              </Card>
            </Reveal>
          ))}
        </div>
        <pre className="overflow-x-auto rounded-xl bg-ink/95 p-4 text-[12px] leading-relaxed text-paper">
          <code>{curl}</code>
        </pre>
      </section>

      {/* Pricing */}
      <section className="space-y-4">
        <h2 className="font-display text-xl tracking-tight text-ink">{t("定价", "Pricing")}</h2>
        <div className="grid gap-3 sm:grid-cols-3">
          {TIERS.map((tier, i) => (
            <Reveal key={tier.name} delay={i * 60}>
              <Card className={`lift flex h-full flex-col ${tier.cta ? "border-forest-300 ring-1 ring-forest-200" : ""}`}>
                <div className="font-display text-lg text-ink">{tier.name}</div>
                <div className="mt-1 font-mono text-2xl font-semibold text-ink">{tier.price}<span className="text-sm font-normal text-muted">{tier.price.startsWith("$") && tier.price !== "$0" ? "/mo" : ""}</span></div>
                <p className="mt-2 text-xs text-ink/70">{L(tier).q}</p>
                <p className="mt-1 text-xs text-muted">{L(tier).who}</p>
                {tier.cta && (
                  <button
                    type="button"
                    onClick={upgradePro}
                    disabled={busy}
                    className="mt-3 inline-flex items-center justify-center rounded-full bg-forest-700 px-4 py-1.5 text-xs font-medium text-paper transition hover:bg-forest-700/85 disabled:opacity-60"
                  >
                    {busy ? t("跳转中…", "Redirecting…") : t("升级 Pro →", "Upgrade to Pro →")}
                  </button>
                )}
              </Card>
            </Reveal>
          ))}
        </div>
        <p className="text-xs text-muted">{t("免费档现在就能用;付费档接入 Stripe 订阅。", "Free tier works today; paid tiers via Stripe subscription.")}</p>
      </section>

      {/* Honest */}
      <Reveal className="rounded-xl border border-rule bg-white p-5">
        <h3 className="text-sm font-semibold text-ink">{t("证书保证什么,不保证什么", "What the certificate does & doesn't prove")}</h3>
        <p className="mt-2 text-xs leading-relaxed text-ink/70">
          {t(
            "保证溯源与完整性(这个结果由这个审计算法在这份数据上产生、未被篡改、可重算);完整性筛查标的是「异常」而非「造假」;不保证数据「正确」或「无版权问题」。我们不 over-claim。",
            "It proves provenance + integrity (this result came from this audited algorithm on this data, untampered, re-hashable); the integrity screen flags anomalies, not fraud; it does not certify that the data is 'correct' or rights-cleared. We don't over-claim.",
          )}
        </p>
      </Reveal>

      <div className="flex flex-wrap items-center gap-4">
        <Link href="/account" className="inline-flex items-center justify-center rounded-full bg-ink px-6 py-2.5 text-sm font-medium text-paper transition hover:bg-ink/85">
          {t("拿一个免费 API key →", "Get a free API key →")}
        </Link>
        <Link href="/c2d" className="text-sm font-medium text-forest-700 hover:underline">{t("背后的技术 →", "The technology behind it →")}</Link>
        <a href={`${API_BASE.replace(/\/api\/v1$/, "")}/docs`} target="_blank" rel="noreferrer" className="text-sm font-medium text-forest-700 hover:underline">{t("API 文档 →", "API docs →")}</a>
      </div>
    </div>
  );
}
