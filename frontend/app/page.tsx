"use client";

import Link from "next/link";
import { BRAND } from "@/lib/brand";
import { useT } from "@/lib/i18n";

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
        <h3 className="text-lg font-semibold text-emerald-900">
          {t("可用不可见 · 沙箱计算（新）", "Available-but-Invisible · Sandbox Compute (new)")}
        </h3>
        <p className="mt-2 max-w-3xl text-sm leading-relaxed text-emerald-800">
          {t(
            "对高敏感数据，买方可在平台沙箱内运行经审核的算法，只取走计算结果（模型 / 统计），不获得原始数据——差分隐私加噪、输出闸门、全程留痕。诚实分级：L1 买方不可见、L2（TEE）连平台也不可见。",
            "For sensitive data, buyers run platform-reviewed algorithms inside a sandbox and take only the result (model / statistics) — never the raw data. Differential-privacy noise, an output gate, and a full audit trail. Honest tiers: L1 invisible to the buyer; L2 (TEE) invisible to the platform too.",
          )}
        </p>
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
