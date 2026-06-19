"use client";

import Link from "next/link";
import { useT } from "@/lib/i18n";
import { PageHeader } from "@/components/ui";

type Status = "verified" | "partial" | "gated" | "none";
const ITEMS: { status: Status; zh: { c: string; d: string }; en: { c: string; d: string } }[] = [
  { status: "verified", zh: { c: "C2D 沙箱执行", d: "--network=none、只读、非特权;只产出聚合,原始数据不出沙箱" }, en: { c: "C2D sandbox execution", d: "--network=none, read-only, unprivileged; only aggregates leave, raw data never does" } },
  { status: "verified", zh: { c: "结果存证 (可重算核验)", d: "输出 SHA-256 绑定算法镜像 digest + 源数据集;买方可重算比对" }, en: { c: "Result certificates (re-hashable)", d: "output SHA-256 bound to the algorithm image digest + source dataset; the buyer re-hashes to verify" } },
  { status: "verified", zh: { c: "9 个算法,各一张真证书", d: "完整性筛查 + 因果五件套(估计/中介/敏感性/证伪)+ 过程(动力学/物料/经济/碳足迹),全部在 live 平台跑通" }, en: { c: "9 algorithms, each with a real cert", d: "integrity + the causal quintet + process (kinetics/mass-balance/economics/LCA), all run live" } },
  { status: "verified", zh: { c: "差分隐私(中心化)", d: "Laplace 机制 + 原子预算账本;由平台注入 epsilon,买方无法关闭噪声" }, en: { c: "Differential privacy (central)", d: "Laplace mechanism + atomic budget ledger; platform-injected epsilon the buyer cannot disable" } },
  { status: "partial", zh: { c: "联邦学习 / 私有求交 (PSI)", d: "单进程已验证(FedAvg / DDH-PSI);真·多节点(Secretflow)仍受限" }, en: { c: "Federated learning / PSI", d: "single-process verified (FedAvg / DDH-PSI); true multi-node (Secretflow) is gated" } },
  { status: "gated", zh: { c: "L2 真 TEE(硬件)", d: "代码 scaffold + mock attester;需 TEE 云/硬件才能验证机密性" }, en: { c: "L2 real TEE (hardware)", d: "code scaffold + mock attester; needs TEE cloud/hardware to verify confidentiality" } },
  { status: "gated", zh: { c: "真实实验数据", d: "所有演示数据均为 model-grounded 合成(如实标注),非真实实验数据" }, en: { c: "Real experimental data", d: "all demo data is model-grounded synthetic (labelled as such), not real experimental data" } },
  { status: "gated", zh: { c: "真实分账 / 支付", d: "支付走沙箱,不涉及真实资金;需接入持牌支付方" }, en: { c: "Real settlement / payment", d: "payments run in a sandbox with no real funds; needs a licensed payment provider" } },
  { status: "none", zh: { c: "同态加密 / 恶意安全", d: "未实现 —— 我们不 over-claim" }, en: { c: "Homomorphic encryption / malicious security", d: "not implemented — we don't over-claim" } },
];

const BADGE: Record<Status, { cls: string; zh: string; en: string }> = {
  verified: { cls: "bg-emerald-50 text-emerald-700", zh: "已验证", en: "verified" },
  partial: { cls: "bg-gold-50 text-gold-700", zh: "部分", en: "partial" },
  gated: { cls: "bg-neutral-100 text-neutral-600", zh: "受限", en: "gated" },
  none: { cls: "bg-neutral-100 text-neutral-500", zh: "未做", en: "not built" },
};

export default function HonestyPage() {
  const { t, lang } = useT();
  const L = <T,>(o: { zh: T; en: T }) => (lang === "en" ? o.en : o.zh);
  return (
    <div className="max-w-2xl space-y-8 pb-20">
      <PageHeader
        kicker={t("绿洲 · 诚实分级", "Verdant Oasis · honest status")}
        title={t("什么已验证,什么仍受限", "What's verified, what's gated")}
        subtitle={t(
          "可信平台先得对自己诚实。下面逐条标明每项能力的真实状态——已验证的、部分的、受外部条件限制的、以及我们没做的。",
          "A trust platform has to be honest about itself first. Below is the real status of each capability — verified, partial, gated on external conditions, or simply not built.",
        )}
      />
      <ul className="space-y-2.5">
        {ITEMS.map((it) => {
          const b = BADGE[it.status];
          return (
            <li key={it.zh.c} className="rounded-xl border border-rule bg-white p-4">
              <div className="flex items-center justify-between gap-3">
                <span className="font-medium text-ink">{L(it).c}</span>
                <span className={`inline-block whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-medium ${b.cls}`}>{L(b)}</span>
              </div>
              <p className="mt-1 text-xs leading-relaxed text-ink/70">{L(it).d}</p>
            </li>
          );
        })}
      </ul>
      <p className="text-sm leading-relaxed text-ink/70">
        {t(
          "突破「受限」项需要外部输入(真实数据方 / TEE 云 / 多节点 / 合规支付),而非更多代码。在那之前,这套东西是一个真实可跑、可验证、零造假的可信科研计算 demo —— 它诚实地知道自己是什么。",
          "Lifting the gated items needs external inputs (real data owners / TEE cloud / multi-node / compliant payment), not more code. Until then this is a real, verifiable, zero-fabrication trustworthy-research-compute demo that honestly knows what it is.",
        )}
      </p>
      <Link href="/c2d" className="font-medium text-forest-700 hover:underline">{t("← 返回可信计算总览", "← Back to the compute-to-data overview")}</Link>
    </div>
  );
}
