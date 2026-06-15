"use client";

import Link from "next/link";
import { BRAND } from "@/lib/brand";
import { useT } from "@/lib/i18n";
import { ComputeFlowDiagram } from "@/components/ComputeFlowDiagram";

export default function Home() {
  const { t } = useT();
  const cards = [
    {
      title: t("质量可信", "Trustworthy quality"),
      desc: t(
        "上传即过质检：格式、统计、去重、PII 扫描。买家敢为干净数据付溢价。",
        "Every upload is screened — format, statistics, dedup, PII. Buyers pay a premium for clean data with confidence.",
      ),
    },
    {
      title: t("来源合规", "Compliant provenance"),
      desc: t(
        "强制来源声明 + 电子签约 + 形式审查留痕。仅境内、实名准入。",
        "Mandatory provenance declaration, e-signing, and an audited review trail. Real-name access.",
      ),
    },
    {
      title: t("资金安全", "Safe settlement"),
      desc: t(
        "分账模式，资金由持牌方存管，确认收货后自动结算，纠纷可裁决。",
        "Split settlement with licensed custody; auto-settled on confirmation, with adjudicable disputes.",
      ),
    },
  ];

  return (
    <div className="space-y-12">
      <section className="space-y-5 py-8">
        <h1 className="text-4xl font-semibold tracking-tight">{BRAND.name}</h1>
        <p className="text-lg font-medium text-emerald-700">{BRAND.sloganEn}</p>
        <p className="text-lg text-emerald-700">{BRAND.sloganZh}</p>
        <p className="max-w-2xl leading-relaxed text-neutral-600">
          {t(
            `${BRAND.tagline}：高信任、可追溯、合规的训练数据流通基础设施。让优质数据被公平定价、安全交易、全程留痕。`,
            "A high-trust, traceable, compliant marketplace for AI training data — fair pricing, safe trading, and a full audit trail.",
          )}
        </p>
        <div className="flex gap-3">
          <Link href="/datasets" className="rounded-md bg-neutral-900 px-5 py-2.5 text-sm font-medium text-white hover:bg-neutral-700">
            {t("浏览数据市场", "Browse the marketplace")}
          </Link>
          <Link href="/sell" className="rounded-md border border-neutral-300 bg-white px-5 py-2.5 text-sm font-medium hover:bg-neutral-50">
            {t("上架我的数据", "List my data")}
          </Link>
        </div>
      </section>

      <section className="grid gap-5 md:grid-cols-3">
        {cards.map((c) => (
          <div key={c.title} className="rounded-xl border border-neutral-200 bg-white p-6">
            <h3 className="text-lg font-semibold">{c.title}</h3>
            <p className="mt-2 text-sm leading-relaxed text-neutral-600">{c.desc}</p>
          </div>
        ))}
      </section>

      <section className="rounded-xl border border-emerald-200 bg-emerald-50 p-6">
        <div className="flex items-center gap-2">
          <h3 className="text-lg font-semibold text-emerald-900">
            {t("可用不可见 · 沙箱计算（招牌能力）", "Available-but-Invisible · Sandbox Compute (our signature)")}
          </h3>
        </div>
        <p className="mt-2 max-w-3xl text-sm leading-relaxed text-emerald-800">
          {t(
            "对高敏感数据，买方可在平台沙箱内运行经审核的算法，只取走计算结果（模型 / 统计），不获得原始数据——差分隐私加噪、输出闸门、全程留痕。",
            "For sensitive data, buyers run platform-reviewed algorithms inside a sandbox and take only the result (model / statistics) — never the raw data. Differential-privacy noise, an output gate, and a full audit trail.",
          )}
        </p>

        {/* How it works — the flow is counterintuitive, so show it visually. */}
        <div className="mt-5 rounded-lg border border-emerald-200 bg-white/70 p-4">
          <ComputeFlowDiagram />
        </div>

        <p className="mt-4 text-xs leading-relaxed text-emerald-700">
          {t(
            "诚实分级：L1 买方不可见(平台运营方仍可见数据)；L3 数据不出域(联邦 / 求交)；L2（TEE）连平台也不可见。",
            "Honest tiers: L1 invisible to the buyer (the platform operator can still see the data); L3 data-stays-home (federated / PSI); L2 (TEE) invisible to the platform too.",
          )}{" "}
          <Link href="/trust" className="font-medium underline underline-offset-2">
            {t("各级真实保证 →", "What each tier really guarantees →")}
          </Link>
        </p>

        <div className="mt-4 flex flex-wrap gap-3">
          <Link href="/datasets" className="rounded-md bg-emerald-700 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-800">
            {t("看支持沙箱计算的数据", "Browse compute-enabled data")}
          </Link>
          <Link href="/compute" className="rounded-md border border-emerald-300 bg-white px-4 py-2 text-sm font-medium text-emerald-800 hover:bg-emerald-50">
            {t("进入隐私计算中心", "Open the privacy-compute hub")}
          </Link>
          <Link href="/verify" className="rounded-md border border-emerald-300 bg-white px-4 py-2 text-sm font-medium text-emerald-800 hover:bg-emerald-50">
            {t("存证验真", "Verify a certificate")}
          </Link>
        </div>
      </section>

      {/* Trust strip — real features, not fabricated metrics. */}
      <section className="grid gap-3 text-sm sm:grid-cols-3">
        {[
          { h: t("结果存证可验真", "Verifiable result certificates"), d: t("每次计算输出绑定已审核算法镜像 digest，凭证号可独立核验。", "Every output binds to the audited algorithm image digest; the certificate ID verifies independently.") },
          { h: t("远程证明(L2)", "Remote attestation (L2)"), d: t("机密计算作业附平台核验的证明，度量值对应入隔离区运行的算法。", "Confidential jobs carry a platform-checked attestation tying the measurement to the enclave's algorithm.") },
          { h: t("全程审计留痕", "Full audit trail"), d: t("准入、签约、计算、放行、结算全部留痕，运营后台可查。", "Access, signing, compute, release, and settlement are all logged and inspectable in the ops console.") },
        ].map((c) => (
          <div key={c.h} className="rounded-lg border border-neutral-200 bg-white p-4">
            <div className="font-semibold text-neutral-800">{c.h}</div>
            <p className="mt-1 text-xs leading-relaxed text-neutral-500">{c.d}</p>
          </div>
        ))}
      </section>

      <section className="rounded-xl border border-amber-200 bg-amber-50 p-5 text-sm text-amber-800">
        {t(
          "演示环境：支付走沙箱（不涉及真实资金），对象存储为本地驱动。真实微信/支付宝分账与 OSS 需接入持牌方与云存储后启用。",
          "Demo environment: payments run in a sandbox (no real funds) and object storage uses a local driver. Real WeChat/Alipay split settlement and cloud storage require a licensed provider and cloud setup.",
        )}
      </section>
    </div>
  );
}
