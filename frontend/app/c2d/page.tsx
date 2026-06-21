"use client";

import Link from "next/link";
import { useT } from "@/lib/i18n";
import { Card, PageHeader } from "@/components/ui";
import { ComputeCertificateCard } from "@/components/ComputeCertificate";
import { Reveal } from "@/components/Reveal";
import { VerifyChip } from "@/components/VerifyChip";

// The four flagship compute-to-data algorithms, each with a REAL certificate
// issued on this platform (verified live below).
const ALGOS = [
  {
    cert: "VO-6CB8181EBD2C",
    origin: "PaperGuard",
    zh: { name: "数据完整性筛查", desc: "在沙箱内对表格数据跑 8 个统计异常检测器(Benford、终端位、算术一致性…),只回传按检测器分级的完整性判定,绝不回传原始行。" },
    en: { name: "Data-integrity screen", desc: "Runs 8 statistical anomaly detectors (Benford, terminal-digit, arithmetic consistency…) over tabular data in the sandbox; returns a per-detector integrity verdict, never raw rows." },
  },
  {
    cert: "VO-1DFD9CBEFFAB",
    origin: "bos-platform",
    zh: { name: "因果中介 (Pearl NDE/NIE)", desc: "把一个处理效应分解为直接效应(NDE)与经中介的间接效应(NIE),含自助置信区间;只回传效应量。" },
    en: { name: "Causal mediation (Pearl NDE/NIE)", desc: "Decomposes a treatment effect into natural direct (NDE) and indirect (NIE) effects with bootstrap CIs; returns effect sizes only." },
  },
  {
    cert: "VO-639F1C2A367C",
    origin: "bos-platform",
    zh: { name: "敏感性分析 (Cinelli-Hazlett)", desc: "量化一个效应对未观测混杂的稳健性(稳健性值 + 偏 R²);只回传敏感性统计量。" },
    en: { name: "Sensitivity (Cinelli-Hazlett)", desc: "Quantifies how robust an effect is to unobserved confounding (robustness value + partial R²); returns sensitivity stats only." },
  },
  {
    cert: "VO-6B9E6ACC8A5F",
    origin: "bos-platform",
    zh: { name: "生长动力学 (Logistic/Gompertz)", desc: "对生物量时间序列拟合生长曲线,回传承载力、生长率、滞后期等参数与拟合优度;不回传原始测量。" },
    en: { name: "Growth kinetics (Logistic/Gompertz)", desc: "Fits growth curves to a biomass time-series; returns carrying capacity, growth rate, lag and goodness-of-fit — not the raw measurements." },
  },
  {
    cert: "VO-817B868978BB", origin: "bos-platform",
    zh: { name: "处理效应估计 (ATE)", desc: "估计一个处理对结果的平均因果效应(OLS + 交叉拟合 DML);只回传效应量与置信区间。" },
    en: { name: "Effect estimation (ATE)", desc: "The average causal effect of a treatment on an outcome (OLS + cross-fitted DML); returns effect sizes + CIs only." },
  },
  {
    cert: "VO-D9342583F9B4", origin: "bos-platform",
    zh: { name: "因果证伪", desc: "用安慰剂处理 / 随机共因 / 数据子集三重 refuter 校验效应是否稳健;只回传校验结论。" },
    en: { name: "Causal refutation", desc: "Stress-tests an effect with placebo / random-common-cause / data-subset refuters; returns the validity verdict only." },
  },
  {
    cert: "VO-885746636F33", origin: "bos-platform",
    zh: { name: "物料平衡", desc: "对一批转化运行检查物料闭合率与残差 ε;只回传聚合闭合统计。" },
    en: { name: "Mass balance", desc: "Checks mass closure + residual ε across a process run log; returns aggregate closure statistics only." },
  },
  {
    cert: "VO-1410D25DB7E2", origin: "bos-platform",
    zh: { name: "过程经济", desc: "对一批生产批次聚合收入/成本/毛利/单位成本;不回传单批数据。" },
    en: { name: "Process economics", desc: "Aggregates revenue/cost/margin/unit-cost across production batches; never the per-batch figures." },
  },
  {
    cert: "VO-6FC497AD7987", origin: "bos-platform",
    zh: { name: "碳足迹 (LCA)", desc: "对一批运行聚合温室气体足迹(GWP);不回传单次运行的能耗/物流数据。" },
    en: { name: "GHG footprint (LCA)", desc: "Aggregates the greenhouse-gas footprint (GWP) across a run log; never the per-run energy/logistics data." },
  },
];

// The nine algorithms group into three families, so the grid tells a story
// (integrity → causal inference → bioprocess) instead of being a uniform wall.
const FAMILIES = [
  { key: "integrity", zh: "完整性", en: "Integrity", certs: ["VO-6CB8181EBD2C"] },
  { key: "causal", zh: "因果推断", en: "Causal inference", certs: ["VO-817B868978BB", "VO-1DFD9CBEFFAB", "VO-639F1C2A367C", "VO-D9342583F9B4"] },
  { key: "process", zh: "生物过程", en: "Bioprocess", certs: ["VO-6B9E6ACC8A5F", "VO-885746636F33", "VO-1410D25DB7E2", "VO-6FC497AD7987"] },
];

const STEPS = [
  { zh: "算法上架", en: "Publish", dzh: "算法打包为镜像,按 SHA-256 digest 钉死,经平台审核 + 信任后上架货架。", den: "Each algorithm is a digest-pinned image, audited and trusted before it reaches the shelf." },
  { zh: "沙箱计算", en: "Compute", dzh: "在 --network=none、只读、非特权沙箱内对数据计算;数据可用不可见,只产出聚合结果。", den: "Runs over the data in a no-network, read-only, unprivileged sandbox — data usable but unseen; only aggregates leave." },
  { zh: "签发存证", en: "Certify", dzh: "把结果指纹(SHA-256)绑定到产出它的算法镜像 digest 与源数据集,签发存证 VO-…。", den: "Binds the result fingerprint (SHA-256) to the producing algorithm digest and the source dataset; issues a VO-… certificate." },
  { zh: "公开验证", en: "Verify", dzh: "任何人可重算下载结果的 SHA-256 与存证比对,或在公开验证页核验。", den: "Anyone re-hashes the downloaded result against the certificate, or checks it on the public verify page." },
];

// A real registered certificate, embedded to showcase the buyer-facing credential.
const EXAMPLE_CERT: Record<string, unknown> = {
  certificate_id: "VO-6CB8181EBD2C",
  algorithm: { name: "PaperGuard data-integrity screen", image_digest: "sha256:6d0ad32bcf0327468fa2e9b1219c22722a617f2de4093ecbaecef0cece689a59", version: 1 },
  dataset_id: "4915afe6-fb00-49ae-841d-053bd13712c0",
  integrity: { algorithm: "SHA-256", verifiable: true },
  output_sha256: "c84aa30538181142fb7c34903b93ad918fa1d3f24742810733b3834be9b9e08b",
  operator: "杭州科农绿洲生物科技有限公司",
  registered_at: "2026-06-21 18:18:31",
  output_bytes: 977,
  status: "registered",
  statement_zh: "本凭证由平台基于「可用不可见」计算结果的内容指纹(SHA-256)、产出该结果的已审核算法(镜像 digest 钉死)与源数据集出具,用于结果完整性校验与计算溯源存证。买方可对下载结果重新计算 SHA-256 与本凭证比对。",
  statement_en: "Platform-issued provenance & integrity record for a compute-to-data result: it binds the output fingerprint (SHA-256) to the audited algorithm (pinned image digest) that produced it and the source dataset. Buyers can re-hash the downloaded result and compare.",
};

// Real, openly-licensed public data run through the SAME sandbox — both certs are
// live + re-hash-verifiable (UCI Wine Quality red, 1,599 rows).
const REAL_DATA = [
  {
    cert: "VO-6CB8181EBD2C",
    zh: { name: "完整性筛查", result: "8 个探测器 / 5 个触发 / 46 项发现 / verdict: anomalies_flagged。对真实数据集的统计完整性体检。" },
    en: { name: "Integrity screen", result: "8 detectors / 5 flagged / 46 findings / verdict: anomalies_flagged. A statistical-integrity check of a real dataset." },
  },
  {
    cert: "VO-DF2EA8BF09F4",
    zh: { name: "因果效应估计 (ATE)", result: "alcohol → quality(调 pH/硫酸盐/密度):OLS 0.365(95% CI 0.33–0.40),交叉拟合 DML 0.321。" },
    en: { name: "Causal effect (ATE)", result: "alcohol → quality (adj. pH/sulphates/density): OLS 0.365 (95% CI 0.33–0.40), cross-fitted DML 0.321." },
  },
];

export default function C2DShowcasePage() {
  const { t, lang } = useT();
  const L = <T,>(o: { zh: T; en: T }) => (lang === "en" ? o.en : o.zh);

  return (
    <div className="space-y-14 pb-20">
      <PageHeader
        kicker={t("绿洲 · 隐私计算 · 可信证据", "Verdant Oasis · compute-to-data · verifiable evidence")}
        title={t("可验证、隐私保护的科研计算", "Verifiable, privacy-preserving research compute")}
        subtitle={t(
          "把科研分析做成「可用不可见」的算法:在沙箱内对你看不到的数据计算,只产出聚合结果,并签发可独立核验的溯源存证。下面九个算法都已在本平台真实跑通、各自签发了一张真证书。",
          "Research analyses as compute-to-data algorithms: they run over data you never see, emit only aggregates, and issue an independently verifiable provenance certificate. All nine algorithms below have run for real on this platform — each with a live certificate.",
        )}
      />

      {/* How it works */}
      <section className="space-y-4">
        <h2 className="font-display text-xl tracking-tight text-ink">{t("工作原理", "How it works")}</h2>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {STEPS.map((s, i) => (
            <Card key={s.en} className="!p-4">
              <div className="font-mono text-kicker uppercase tracking-widest text-forest-700">{`0${i + 1}`}</div>
              <div className="mt-1 font-medium text-ink">{L({ zh: s.zh, en: s.en })}</div>
              <p className="mt-1.5 text-xs leading-relaxed text-ink/70">{L({ zh: s.dzh, en: s.den })}</p>
            </Card>
          ))}
        </div>
      </section>

      {/* Flagship algorithms — grouped into families so the grid tells a story */}
      <section className="space-y-6">
        <h2 className="font-display text-xl tracking-tight text-ink">{t("旗舰算法", "Flagship algorithms")}</h2>
        {FAMILIES.map((fam) => {
          const items = ALGOS.filter((a) => fam.certs.includes(a.cert));
          return (
            <div key={fam.key} className="space-y-3">
              <p className="font-mono text-kicker uppercase tracking-widest text-muted">
                {L({ zh: fam.zh, en: fam.en })} · {items.length}
              </p>
              <div className="grid gap-3 sm:grid-cols-2">
                {items.map((a, i) => (
                  <Reveal key={a.cert} delay={(i % 2) * 80}>
                    <Card className="lift flex h-full flex-col gap-2">
                      <div className="flex items-baseline justify-between gap-2">
                        <span className="font-display text-lg leading-snug text-ink">{L(a)?.name}</span>
                        <span className="shrink-0 font-mono text-[10px] uppercase tracking-wider text-muted">{a.origin}</span>
                      </div>
                      <p className="text-xs leading-relaxed text-ink/70">{L(a)?.desc}</p>
                      <div className="mt-auto flex items-center justify-between gap-2 pt-2">
                        <Link href={`/verify?cert=${a.cert}`} className="rounded font-mono text-xs text-forest-700 hover:underline focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ink">
                          {a.cert}
                        </Link>
                        <VerifyChip certId={a.cert} />
                      </div>
                    </Card>
                  </Reveal>
                ))}
              </div>
            </div>
          );
        })}
      </section>

      {/* Verified on REAL public data */}
      <section className="space-y-4">
        <h2 className="font-display text-xl tracking-tight text-ink">{t("在真实公开数据上验证", "Verified on real public data")}</h2>
        <p className="max-w-2xl text-sm leading-relaxed text-ink/70">
          {t(
            "不止合成 demo 数据——下面两张证书,是把开放许可的真实数据集 UCI Wine Quality(1599 行)喂进同一个 --network=none 沙箱跑出来的,每一张都可重算核验。",
            "Not just synthetic demo data — these two certificates were produced by running the openly-licensed real UCI Wine Quality dataset (1,599 rows) through the same --network=none sandbox; each is re-hash-verifiable.",
          )}
        </p>
        <div className="grid gap-3 sm:grid-cols-2">
          {REAL_DATA.map((r, i) => (
            <Reveal key={r.cert} delay={(i % 2) * 80}>
              <Card className="lift flex h-full flex-col gap-2">
                <div className="flex items-baseline justify-between gap-2">
                  <span className="font-display text-lg leading-snug text-ink">{L(r)?.name}</span>
                  <span className="shrink-0 font-mono text-[10px] uppercase tracking-wider text-muted">UCI Wine Quality</span>
                </div>
                <p className="text-xs leading-relaxed text-ink/70">{L(r)?.result}</p>
                <div className="mt-auto flex items-center justify-between gap-2 pt-2">
                  <Link href={`/verify?cert=${r.cert}`} className="rounded font-mono text-xs text-forest-700 hover:underline focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ink">
                    {r.cert}
                  </Link>
                  <VerifyChip certId={r.cert} />
                </div>
              </Card>
            </Reveal>
          ))}
        </div>
        <p className="text-xs leading-relaxed text-ink/55">
          {t(
            "诚实:完整性筛查标的是「异常」而非「造假」;因果证书证的是「溯源」而非「因果假设成立」。",
            "Honest: the integrity screen flags anomalies, not fraud; the causal cert proves provenance, not that the causal assumptions hold.",
          )}
        </p>
      </section>

      {/* Example credential */}
      <section className="space-y-4">
        <h2 className="font-display text-xl tracking-tight text-ink">{t("一张真实的结果存证", "A real result certificate")}</h2>
        <p className="max-w-2xl text-sm leading-relaxed text-ink/70">
          {t(
            "买家在每次计算后拿到的凭证长这样——它把结果指纹绑定到产出它的算法镜像 digest 与源数据集,可独立重算核验。",
            "This is the credential a buyer receives after every computation — it binds the result fingerprint to the producing algorithm's image digest and the source dataset, and is independently re-hashable.",
          )}
        </p>
        <div className="rounded-2xl border border-rule bg-paper/60 px-4 py-8 sm:px-8">
          <div className="mx-auto max-w-md">
            <ComputeCertificateCard cert={EXAMPLE_CERT} />
          </div>
        </div>
      </section>

      {/* Deeper surfaces */}
      <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <Link href="/c2d/dossier" className="group">
          <Card className="lift h-full group-hover:border-forest-700">
            <div className="font-medium text-ink">{t("可验证研究档案 →", "A verifiable research dossier →")}</div>
            <p className="mt-1 text-xs leading-relaxed text-ink/70">{t("对同一个数据集做的全部分析,五张证书串成一条完整证据链。", "Every analysis on one dataset — five certificates forming a complete evidence chain.")}</p>
          </Card>
        </Link>
        <Link href="/c2d/reproduce" className="group">
          <Card className="lift h-full group-hover:border-forest-700">
            <div className="font-medium text-ink">{t("可复现性:自己重跑一遍 →", "Reproducibility: re-run it yourself →")}</div>
            <p className="mt-1 text-xs leading-relaxed text-ink/70">{t("证书是可重跑、防篡改的记录——方法+数据+结果,三种独立复现方式。", "The cert is a re-runnable, tamper-evident record — method+data+result, three independent ways to reproduce.")}</p>
          </Card>
        </Link>
        <Link href="/c2d/honesty" className="group">
          <Card className="lift h-full group-hover:border-forest-700">
            <div className="font-medium text-ink">{t("诚实分级:什么已验证,什么受限 →", "Honest status: what's verified, what's gated →")}</div>
            <p className="mt-1 text-xs leading-relaxed text-ink/70">{t("逐条标明每项能力的真实状态——我们不 over-claim。", "The real status of each capability, item by item — we don't over-claim.")}</p>
          </Card>
        </Link>
      </section>

      <section>
        <Card className="flex flex-col items-start gap-3 bg-paper sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="font-medium text-ink">{t("核验任意一张存证", "Verify any certificate")}</div>
            <p className="text-xs text-ink/70">{t("无需登录,输入 VO-… 编号即可独立核验。", "No login — enter a VO-… id to check it independently.")}</p>
          </div>
          <Link
            href="/verify"
            className="inline-flex items-center justify-center rounded-full bg-ink px-5 py-2 text-sm font-medium text-paper transition hover:bg-ink/85"
          >
            {t("前往验真 →", "Go to verify →")}
          </Link>
        </Card>
      </section>
    </div>
  );
}
