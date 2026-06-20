"use client";

import Link from "next/link";
import { useT } from "@/lib/i18n";
import { PageHeader } from "@/components/ui";
import { Reveal } from "@/components/Reveal";

type Status = "verified" | "partial" | "gated" | "none";
const ITEMS: { status: Status; zh: { c: string; d: string }; en: { c: string; d: string } }[] = [
  { status: "verified", zh: { c: "C2D 沙箱执行", d: "--network=none、只读、非特权;只产出聚合,原始数据不出沙箱" }, en: { c: "C2D sandbox execution", d: "--network=none, read-only, unprivileged; only aggregates leave, raw data never does" } },
  { status: "verified", zh: { c: "结果存证 (可重算核验)", d: "输出 SHA-256 绑定算法镜像 digest + 源数据集;买方可重算比对" }, en: { c: "Result certificates (re-hashable)", d: "output SHA-256 bound to the algorithm image digest + source dataset; the buyer re-hashes to verify" } },
  { status: "verified", zh: { c: "反恶意算法输出闸", d: "结构 + 信息量校验(JSON / zip-of-json,字符串/数值/熵上限),挡住把原始行隐写进「聚合」的外泄" }, en: { c: "Anti-malicious-algorithm output gate", d: "structural + information-content checks (JSON / zip-of-json; string/numeric/entropy caps) block exfil of raw rows hidden in an 'aggregate'" } },
  { status: "verified", zh: { c: "9 个算法,各一张真证书", d: "完整性筛查 + 因果五件套(估计/中介/敏感性/证伪)+ 过程(动力学/物料/经济/碳足迹),全部在 live 平台跑通" }, en: { c: "9 algorithms, each with a real cert", d: "integrity + the causal quintet + process (kinetics/mass-balance/economics/LCA), all run live" } },
  { status: "verified", zh: { c: "差分隐私(中心化)", d: "Laplace 机制 + 原子预算账本;由平台注入 epsilon,买方无法关闭噪声" }, en: { c: "Differential privacy (central)", d: "Laplace mechanism + atomic budget ledger; platform-injected epsilon the buyer cannot disable" } },
  { status: "partial", zh: { c: "联邦学习 / 私有求交 (PSI)", d: "单进程已验证(FedAvg / DDH-PSI);真·多节点(Secretflow)仍受限" }, en: { c: "Federated learning / PSI", d: "single-process verified (FedAvg / DDH-PSI); true multi-node (Secretflow) is gated" } },
  { status: "partial", zh: { c: "L2 真 TEE 远程证明", d: "真 DCAP quote 验证已实现并离线测过(对 Intel 生产样本 quote 验签 + PCK 证书链到 Intel 根 + 度量值白名单);仍受限:硬件出 live quote + 机密性本身" }, en: { c: "L2 real TEE attestation", d: "real DCAP quote verification implemented + offline-tested (ECDSA sig + PCK chain to Intel's root + measurement allowlist, on Intel's production sample); still gated: hardware emitting live quotes + confidentiality itself" } },
  { status: "gated", zh: { c: "真实实验数据", d: "所有演示数据均为 model-grounded 合成(如实标注),非真实实验数据" }, en: { c: "Real experimental data", d: "all demo data is model-grounded synthetic (labelled as such), not real experimental data" } },
  { status: "gated", zh: { c: "真实分账 / 支付", d: "支付走沙箱,不涉及真实资金;需接入持牌支付方" }, en: { c: "Real settlement / payment", d: "payments run in a sandbox with no real funds; needs a licensed payment provider" } },
  { status: "none", zh: { c: "同态加密 / 恶意安全", d: "未实现 —— 我们不 over-claim" }, en: { c: "Homomorphic encryption / malicious security", d: "not implemented — we don't over-claim" } },
];

const BADGE: Record<Status, { cls: string; zh: string; en: string }> = {
  verified: { cls: "bg-forest-50 text-forest-700", zh: "已验证", en: "verified" },
  partial: { cls: "bg-gold-50 text-gold-700", zh: "部分", en: "partial" },
  gated: { cls: "bg-neutral-100 text-neutral-600", zh: "受限", en: "gated" },
  none: { cls: "bg-neutral-100 text-neutral-500", zh: "未做", en: "not built" },
};

// Threat model: four adversaries. Each one names where the system defends and
// where that defense fails — the no-over-claim posture, item by item.
type Defense = "defended" | "partial" | "gap";
const DEFENSE: Record<Defense, { cls: string; zh: string; en: string }> = {
  defended: { cls: "bg-forest-50 text-forest-700", zh: "已防御", en: "defended" },
  partial: { cls: "bg-gold-50 text-gold-700", zh: "部分", en: "partial" },
  gap: { cls: "bg-neutral-100 text-neutral-600", zh: "L1 缺口", en: "L1 gap" },
};
const ADVERSARIES: {
  defense: Defense;
  zh: { who: string; defend: string; fail: string };
  en: { who: string; defend: string; fail: string };
}[] = [
  {
    defense: "defended",
    zh: { who: "好奇的买家", defend: "沙箱只回传闸控后的聚合,从不回传原始行;DP 注入买方无法关闭的噪声。", fail: "DP 只覆盖启用它的算法;塞太多信息的聚合由输出闸兜底。" },
    en: { who: "Curious buyer", defend: "the sandbox returns only the gated aggregate, never raw rows; DP injects noise the buyer can't disable.", fail: "DP only covers algorithms that enable it; an over-informative aggregate is caught by the output gate." },
  },
  {
    defense: "defended",
    zh: { who: "恶意的买家", defend: "自定义算法需卖方显式开启;L1 上的模型输出必须用可信算法;DP 预算账本原子防超额。", fail: "若卖方开了自定义又不 review,买方算法只受输出闸约束。" },
    en: { who: "Malicious buyer", defend: "custom algorithms need explicit seller opt-in; model output on L1 requires a trusted algorithm; the atomic DP ledger prevents overshoot.", fail: "if the seller allows custom without review, the buyer's algorithm is bounded only by the output gate." },
  },
  {
    defense: "partial",
    zh: { who: "恶意的算法作者", defend: "输出闸(本次新增):结构形状 + 字符串/数值/深度/熵上限,fail-closed,绝不改写输出。挡死「把数据集 dump 成 output.bin」。", fail: "部分缓解,非归零:有决心的作者仍能在边界内外泄少量信息(几 KB / 约 1 万数)。" },
    en: { who: "Malicious algorithm author", defend: "the output gate (new): structural shape + string/numeric/depth/entropy caps, fail-closed, never mutates. Kills 'dump the dataset as output.bin'.", fail: "partial, not zero: a determined author can still exfil a little WITHIN the bounds (a few KB / ~10k numbers)." },
  },
  {
    defense: "gap",
    zh: { who: "被攻陷的运营方", defend: "L2(TEE)把计算放进飞地,连运营方也看不见明文 + 远程证明;L3 让数据不出域。", fail: "当前默认 L1 下运营方能看到暂存数据——如实披露。真 TEE 受限于硬件。" },
    en: { who: "Compromised operator", defend: "L2 (TEE) puts compute in an enclave invisible even to the operator + remote attestation; L3 keeps data home.", fail: "under the current default L1 the operator CAN see staged data — disclosed honestly. Real TEE is gated on hardware." },
  },
];

const GUARANTEES = {
  zh: ["溯源:这个 output 由这个 digest 钉死的算法 + 这份数据集产生,可重算核验。", "完整性:输出未被事后篡改;算法镜像未被掉包。", "可复现:方法 + 数据 + 结果 是一条可重跑、防篡改的记录。"],
  en: ["Provenance: this output came from this digest-pinned algorithm + this dataset, re-hash verifiable.", "Integrity: the output wasn't altered after the fact; the image wasn't swapped.", "Reproducibility: method + data + result is a re-runnable, tamper-evident record."],
};
const NON_GUARANTEES = {
  zh: ["不保证正确性:有 bug/有偏的算法照样出有效证书。", "不保证安全/良性:闸内仍可能有有界外泄。", "不保证运营方看不到:L1 下运营方能看到数据(机密性是 L2/TEE 的承诺)。"],
  en: ["NOT correctness: a buggy/biased algorithm still produces a valid cert.", "NOT safety: bounded exfil within the gate is still possible.", "NOT operator-invisibility: under L1 the operator can see the data (confidentiality is L2/TEE's promise)."],
};

const WHITEPAPER_URL = encodeURI(
  "https://github.com/exergyleizhou-ux/ai-data-marketplace/blob/main/docs/威胁模型与保证-C2D可验证证据层.md",
);

export default function HonestyPage() {
  const { t, lang } = useT();
  const L = <T,>(o: { zh: T; en: T }) => (lang === "en" ? o.en : o.zh);
  return (
    <div className="max-w-2xl space-y-10 pb-20">
      <PageHeader
        kicker={t("绿洲 · 诚实分级", "Verdant Oasis · honest status")}
        title={t("什么已验证,什么仍受限", "What's verified, what's gated")}
        subtitle={t(
          "可信平台先得对自己诚实。下面逐条标明每项能力的真实状态——已验证的、部分的、受外部条件限制的、以及我们没做的。",
          "A trust platform has to be honest about itself first. Below is the real status of each capability — verified, partial, gated on external conditions, or simply not built.",
        )}
      />

      {/* The core honest statement: what the certificate does and does not guarantee. */}
      <section className="grid gap-3 sm:grid-cols-2">
        <Reveal className="rounded-xl border border-forest-200 bg-forest-50/40 p-4">
          <h3 className="text-sm font-semibold text-forest-800">{t("证书保证", "The certificate guarantees")}</h3>
          <ul className="mt-2 space-y-1.5">
            {L(GUARANTEES).map((g) => (
              <li key={g} className="flex gap-2 text-xs leading-relaxed text-ink/80">
                <span aria-hidden className="mt-0.5 select-none text-forest-600">✓</span>
                <span>{g}</span>
              </li>
            ))}
          </ul>
        </Reveal>
        <Reveal delay={60} className="rounded-xl border border-rule bg-white p-4">
          <h3 className="text-sm font-semibold text-ink">{t("证书不保证", "The certificate does NOT guarantee")}</h3>
          <ul className="mt-2 space-y-1.5">
            {L(NON_GUARANTEES).map((g) => (
              <li key={g} className="flex gap-2 text-xs leading-relaxed text-ink/70">
                <span aria-hidden className="mt-0.5 select-none text-neutral-400">✕</span>
                <span>{g}</span>
              </li>
            ))}
          </ul>
        </Reveal>
      </section>

      {/* Threat model: four adversaries, defend vs fail. */}
      <section className="space-y-3">
        <div className="flex items-baseline justify-between gap-3">
          <h2 className="font-display text-lg text-ink">{t("威胁模型:四类对手", "Threat model: four adversaries")}</h2>
          <a href={WHITEPAPER_URL} target="_blank" rel="noreferrer" className="whitespace-nowrap text-xs font-medium text-forest-700 hover:underline">
            {t("白皮书 →", "Whitepaper →")}
          </a>
        </div>
        <ul className="space-y-2.5">
          {ADVERSARIES.map((a, i) => {
            const d = DEFENSE[a.defense];
            return (
              <Reveal as="li" key={a.zh.who} delay={i * 45} className="lift rounded-xl border border-rule bg-white p-4">
                <div className="flex items-center justify-between gap-3">
                  <span className="font-medium text-ink">{L(a).who}</span>
                  <span className={`inline-block whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-medium ${d.cls}`}>{L(d)}</span>
                </div>
                <p className="mt-2 text-xs leading-relaxed text-ink/75">
                  <span className="font-medium text-forest-700">{t("防御", "Defends")}: </span>{L(a).defend}
                </p>
                <p className="mt-1 text-xs leading-relaxed text-ink/60">
                  <span className="font-medium text-neutral-500">{t("失效", "Fails")}: </span>{L(a).fail}
                </p>
              </Reveal>
            );
          })}
        </ul>
      </section>

      {/* Per-capability honest status list. */}
      <section className="space-y-3">
        <h2 className="font-display text-lg text-ink">{t("逐项状态", "Capability status")}</h2>
        <ul className="space-y-2.5">
          {ITEMS.map((it, i) => {
            const b = BADGE[it.status];
            return (
              <Reveal as="li" key={it.zh.c} delay={i * 40} className="lift rounded-xl border border-rule bg-white p-4">
                <div className="flex items-center justify-between gap-3">
                  <span className="font-medium text-ink">{L(it).c}</span>
                  <span className={`inline-block whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-medium ${b.cls}`}>{L(b)}</span>
                </div>
                <p className="mt-1 text-xs leading-relaxed text-ink/70">{L(it).d}</p>
              </Reveal>
            );
          })}
        </ul>
      </section>

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
