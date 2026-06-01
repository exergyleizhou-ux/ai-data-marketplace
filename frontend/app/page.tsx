import Link from "next/link";
import { BRAND } from "@/lib/brand";

export default function Home() {
  return (
    <div className="space-y-12">
      <section className="space-y-5 py-8">
        <h1 className="text-4xl font-semibold tracking-tight">{BRAND.name}</h1>
        <p className="text-lg font-medium text-emerald-700">{BRAND.sloganEn}</p>
        <p className="text-lg text-emerald-700">{BRAND.sloganZh}</p>
        <p className="max-w-2xl leading-relaxed text-neutral-600">
          {BRAND.tagline}：高信任、可追溯、合规的训练数据流通基础设施。让优质数据被公平定价、安全交易、全程留痕。
        </p>
        <div className="flex gap-3">
          <Link href="/datasets" className="rounded-md bg-neutral-900 px-5 py-2.5 text-sm font-medium text-white hover:bg-neutral-700">
            浏览数据市场
          </Link>
          <Link href="/sell" className="rounded-md border border-neutral-300 bg-white px-5 py-2.5 text-sm font-medium hover:bg-neutral-50">
            上架我的数据
          </Link>
        </div>
      </section>

      <section className="grid gap-5 md:grid-cols-3">
        {[
          { t: "质量可信", d: "上传即过质检：格式、统计、去重、PII 扫描。买家敢为干净数据付溢价。" },
          { t: "来源合规", d: "强制来源声明 + 电子签约 + 形式审查留痕。仅境内、实名准入。" },
          { t: "资金安全", d: "分账模式，资金由持牌方存管，确认收货后自动结算，纠纷可裁决。" },
        ].map((c) => (
          <div key={c.t} className="rounded-xl border border-neutral-200 bg-white p-6">
            <h3 className="text-lg font-semibold">{c.t}</h3>
            <p className="mt-2 text-sm leading-relaxed text-neutral-600">{c.d}</p>
          </div>
        ))}
      </section>

      <section className="rounded-xl border border-amber-200 bg-amber-50 p-5 text-sm text-amber-800">
        演示环境：支付走<strong>沙箱</strong>（不涉及真实资金），对象存储为本地驱动。真实微信/支付宝分账与 OSS
        需接入持牌方与云存储后启用。
      </section>
    </div>
  );
}
