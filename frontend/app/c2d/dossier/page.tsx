"use client";

import Link from "next/link";
import { useT } from "@/lib/i18n";
import { Card, PageHeader } from "@/components/ui";
import { Reveal } from "@/components/Reveal";
import { VerifyChip } from "@/components/VerifyChip";
import { Seal } from "@/components/Seal";

// A real dossier: five analyses run on ONE BSF bioconversion study dataset
// (feed_rate → larval_density → protein_yield), each yielding a signed certificate.
const DATASET = "53780a7e-10f6-4c30-82e1-09abd2b950f9";
const STEPS = [
  { cert: "VO-99931BE5D343", n: 1, zh: { name: "数据完整性筛查", what: "8 个统计异常检测器(Benford / 终端位 / 算术一致性…),确认数据本身无明显异常" }, en: { name: "Data-integrity screen", what: "8 statistical anomaly detectors confirm the data itself shows no obvious irregularities" } },
  { cert: "VO-0FC85EC1917A", n: 2, zh: { name: "处理效应 (ATE)", what: "feed_rate 对 protein_yield 的平均因果效应(OLS + 交叉拟合 DML)" }, en: { name: "Average treatment effect", what: "the average causal effect of feed_rate on protein_yield (OLS + cross-fitted DML)" } },
  { cert: "VO-135793939260", n: 3, zh: { name: "因果中介 (NDE/NIE)", what: "效应有多少是直接的、多少经 larval_density 传导(Pearl 分解)" }, en: { name: "Causal mediation (NDE/NIE)", what: "how much of the effect is direct vs mediated through larval_density (Pearl decomposition)" } },
  { cert: "VO-C0F0FA7D4147", n: 4, zh: { name: "敏感性", what: "效应对未观测混杂的稳健性(Cinelli-Hazlett 稳健性值)" }, en: { name: "Sensitivity", what: "robustness of the effect to unobserved confounding (Cinelli-Hazlett robustness value)" } },
  { cert: "VO-F912F3BE1073", n: 5, zh: { name: "证伪", what: "安慰剂处理 / 随机共因 / 数据子集 三重有效性校验" }, en: { name: "Refutation", what: "placebo-treatment / random-common-cause / data-subset validity checks" } },
];

export default function DossierPage() {
  const { t, lang } = useT();
  const L = <T,>(o: { zh: T; en: T }) => (lang === "en" ? o.en : o.zh);
  return (
    <div className="max-w-3xl space-y-10 pb-20">
      <PageHeader
        kicker={t("绿洲 · 可信计算 · 研究档案", "Verdant Oasis · compute-to-data · research dossier")}
        title={t("一份可验证的研究档案", "A verifiable research dossier")}
        subtitle={t(
          "对同一个数据集做的全部分析,每一步都在沙箱内「可用不可见」地完成,并各自签发一张可独立核验的存证——五张证书串成这项研究的完整证据链。",
          "Every analysis run on one dataset, each done compute-to-data inside the sandbox and each issuing an independently verifiable certificate — five certificates forming the study's complete evidence chain.",
        )}
      />

      <Card className="bg-paper">
        <p className="font-mono text-kicker uppercase tracking-widest text-forest-700">{t("研究数据集", "Study dataset")}</p>
        <p className="mt-1 font-medium text-ink">{t("黑水虻固废生物转化:投喂率 → 幼虫密度 → 蛋白产率", "BSF solid-waste bioconversion: feed-rate → larval-density → protein-yield")}</p>
        <p className="mt-1 font-mono text-xs text-muted">dataset {DATASET.slice(0, 8)}… · 600 rows · model-grounded</p>
      </Card>

      <ol className="space-y-3">
        {STEPS.map((s, i) => (
          <Reveal as="li" key={s.cert} delay={i * 70}>
            <Card className="lift relative overflow-hidden">
              <div className="absolute inset-x-0 top-0 h-0.5 bg-gradient-to-r from-gold-700 via-gold to-gold-700" />
              <div className="flex items-start justify-between gap-3">
                <div className="flex min-w-0 items-start gap-3">
                  <span className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-forest-50 font-mono text-xs text-forest-700">{s.n}</span>
                  <div className="min-w-0">
                    <span className="font-display text-lg leading-snug text-ink">{L(s).name}</span>
                    <p className="mt-1 text-xs leading-relaxed text-ink/70">{L(s).what}</p>
                    <Link href={`/verify?cert=${s.cert}`} className="mt-1.5 inline-block rounded font-mono text-xs text-forest-700 hover:underline focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ink">{s.cert}</Link>
                  </div>
                </div>
                <div className="flex shrink-0 flex-col items-end gap-2">
                  <Seal size={32} label={t(`已验证 · ${s.cert}`, `verified · ${s.cert}`)} />
                  <VerifyChip certId={s.cert} />
                </div>
              </div>
            </Card>
          </Reveal>
        ))}
      </ol>

      <Card className="bg-paper text-sm leading-relaxed text-ink/75">
        {t(
          "每张证书都把结果指纹绑定到产出它的算法镜像 digest 与本数据集,可独立重算核验;五张合起来,就是这项研究「数据从未暴露、每一步都可追溯可复现」的完整证明。",
          "Each certificate binds its result fingerprint to the producing algorithm's image digest and this dataset, independently re-hashable; together the five are the study's complete proof that the data was never exposed and every step is traceable and reproducible.",
        )}
        <div className="mt-3">
          <Link href="/c2d" className="font-medium text-forest-700 hover:underline">{t("← 返回可信计算总览", "← Back to the compute-to-data overview")}</Link>
        </div>
      </Card>
    </div>
  );
}
