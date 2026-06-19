"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Badge, Card, PageHeader } from "@/components/ui";
import { ComputeCertificateCard } from "@/components/ComputeCertificate";

// The four flagship compute-to-data algorithms, each with a REAL certificate
// issued on this platform (verified live below).
const ALGOS = [
  {
    cert: "VO-795A4D76D4FE",
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

const STEPS = [
  { zh: "算法上架", en: "Publish", dzh: "算法打包为镜像,按 SHA-256 digest 钉死,经平台审核 + 信任后上架货架。", den: "Each algorithm is a digest-pinned image, audited and trusted before it reaches the shelf." },
  { zh: "沙箱计算", en: "Compute", dzh: "在 --network=none、只读、非特权沙箱内对数据计算;数据可用不可见,只产出聚合结果。", den: "Runs over the data in a no-network, read-only, unprivileged sandbox — data usable but unseen; only aggregates leave." },
  { zh: "签发存证", en: "Certify", dzh: "把结果指纹(SHA-256)绑定到产出它的算法镜像 digest 与源数据集,签发存证 VO-…。", den: "Binds the result fingerprint (SHA-256) to the producing algorithm digest and the source dataset; issues a VO-… certificate." },
  { zh: "公开验证", en: "Verify", dzh: "任何人可重算下载结果的 SHA-256 与存证比对,或在公开验证页核验。", den: "Anyone re-hashes the downloaded result against the certificate, or checks it on the public verify page." },
];

// A real registered certificate, embedded to showcase the buyer-facing credential.
const EXAMPLE_CERT: Record<string, unknown> = {
  certificate_id: "VO-795A4D76D4FE",
  algorithm: { name: "PaperGuard data-integrity screen", image_digest: "sha256:46ca9a23e080ca2bdf4ba010b400341ecc30b587f3b72810196f7c2ed4692eb3", version: 1 },
  dataset_id: "08a8b100-41cd-4067-a824-3036d2b13a5b",
  integrity: { algorithm: "SHA-256", verifiable: true },
  output_sha256: "92dfc19aaa38766d250325c0df1569e618833ababba6c065224b2c1775afe1ed",
  operator: "杭州科农绿洲生物科技有限公司",
  registered_at: "2026-06-19 04:37:45",
  output_bytes: 942,
  status: "registered",
  statement_zh: "本凭证由平台基于「可用不可见」计算结果的内容指纹(SHA-256)、产出该结果的已审核算法(镜像 digest 钉死)与源数据集出具,用于结果完整性校验与计算溯源存证。买方可对下载结果重新计算 SHA-256 与本凭证比对。",
  statement_en: "Platform-issued provenance & integrity record for a compute-to-data result: it binds the output fingerprint (SHA-256) to the audited algorithm (pinned image digest) that produced it and the source dataset. Buyers can re-hash the downloaded result and compare.",
};

// Live verification chip: hits the public /verify endpoint to prove the cert is
// real and registered right now.
function VerifyChip({ certId }: { certId: string }) {
  const { t } = useT();
  const [st, setSt] = useState<"loading" | "ok" | "err">("loading");
  useEffect(() => {
    let on = true;
    api
      .verifyCertificate(certId)
      .then((r) => on && setSt(r.verifiable ? "ok" : "err"))
      .catch(() => on && setSt("err"));
    return () => {
      on = false;
    };
  }, [certId]);
  const cls =
    st === "ok"
      ? "bg-emerald-50 text-emerald-700"
      : st === "err"
        ? "bg-neutral-100 text-neutral-500"
        : "bg-neutral-100 text-neutral-400";
  return (
    <span className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-medium ${cls}`}>
      {st === "loading" ? t("核验中…", "checking…") : st === "ok" ? t("实时可验证 ✓", "live · verifiable ✓") : t("不可用", "unavailable")}
    </span>
  );
}

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

      {/* Flagship algorithms */}
      <section className="space-y-4">
        <h2 className="font-display text-xl tracking-tight text-ink">{t("旗舰算法", "Flagship algorithms")}</h2>
        <div className="grid gap-3 sm:grid-cols-2">
          {ALGOS.map((a) => (
            <Card key={a.cert} className="flex flex-col gap-2">
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium text-ink">{L(a)?.name}</span>
                <Badge>{a.origin}</Badge>
              </div>
              <p className="text-xs leading-relaxed text-ink/70">{L(a)?.desc}</p>
              <div className="mt-auto flex items-center justify-between gap-2 pt-2">
                <Link href={`/verify?cert=${a.cert}`} className="font-mono text-xs text-forest-700 hover:underline">
                  {a.cert}
                </Link>
                <VerifyChip certId={a.cert} />
              </div>
            </Card>
          ))}
        </div>
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
        <div className="max-w-md">
          <ComputeCertificateCard cert={EXAMPLE_CERT} />
        </div>
      </section>

      {/* Deeper surfaces */}
      <section className="grid gap-3 sm:grid-cols-2">
        <Link href="/c2d/dossier" className="group">
          <Card className="h-full transition group-hover:border-forest-700">
            <div className="font-medium text-ink">{t("可验证研究档案 →", "A verifiable research dossier →")}</div>
            <p className="mt-1 text-xs leading-relaxed text-ink/70">{t("对同一个数据集做的全部分析,五张证书串成一条完整证据链。", "Every analysis on one dataset — five certificates forming a complete evidence chain.")}</p>
          </Card>
        </Link>
        <Link href="/c2d/honesty" className="group">
          <Card className="h-full transition group-hover:border-forest-700">
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
