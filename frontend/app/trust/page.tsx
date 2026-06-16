"use client";

import Link from "next/link";
import { useT } from "@/lib/i18n";
import { ComputeFlowDiagram } from "@/components/ComputeFlowDiagram";

type Tier = {
  tag: string;
  name: [string, string];
  promise: [string, string];
  guarantees: [string, string][];
  limits: [string, string][];
};

const TIERS: Tier[] = [
  {
    tag: "L1",
    name: ["数据沙箱 · 买方不可见", "Data sandbox · invisible to the buyer"],
    promise: [
      "买方在平台沙箱内对数据运行经审核的算法,只取走计算结果,拿不到原始数据。",
      "The buyer runs reviewed algorithms against the data in a sandbox and takes only the result — never the raw data.",
    ],
    guarantees: [
      ["原始数据从不下载、不外传", "Raw data is never downloaded or shipped out"],
      ["可选差分隐私(DP)对输出加噪", "Optional differential-privacy noise on the output"],
      ["输出闸门 + 大小上限,防止把整库塞进结果", "An output gate and size cap stop dumping the whole dataset"],
      ["每次结果出具存证凭证(见下)", "Every result gets a provenance certificate (below)"],
    ],
    limits: [
      ["平台运营方仍可访问原始数据 —— 这是内部威胁面,需 L2 才能消除", "The platform operator can still access the raw data — an insider surface that only L2 removes"],
    ],
  },
  {
    tag: "L3",
    name: ["数据不出域 · 联邦 / 求交", "Data-stays-home · federated / PSI"],
    promise: [
      "多方数据各自留在自己的沙箱,只交换模型参数(联邦)或集合交集(PSI),原始数据互不可见、不出域。",
      "Each party's data stays in its own sandbox; only model params (federated) or set intersections (PSI) are exchanged. Raw data stays home, invisible to others.",
    ],
    guarantees: [
      ["联邦:各方本地训练,平台聚合 FedAvg 联合模型", "Federated: each party trains locally; the platform aggregates a FedAvg joint model"],
      ["安全聚合原语已实现:ECDH 成对掩码,平台聚合时看不到单方参数(semi-honest 模型,见代码 secagg.go)", "Secure-aggregation primitive shipped: ECDH pairwise masks hide each party's params from the aggregator (semi-honest model, see secagg.go)"],
      ["可叠加差分隐私缓解联合模型的参数泄漏", "Differential privacy can be layered on to mitigate param leakage via the joint model"],
    ],
    limits: [
      ["PSI 当前为编排式(平台求交时可见各方集合),真密码学 PSI(SPU)需多节点,规划中", "PSI is currently orchestrated (the platform sees each set during intersection); cryptographic PSI (SPU) needs multiple nodes — planned"],
      ["安全聚合的沙箱内密钥协商、掉队恢复、抗恶意服务器为后续阶段", "In-sandbox key agreement, dropout recovery, and malicious-server defence for secure aggregation are later stages"],
    ],
  },
  {
    tag: "L2",
    name: ["机密计算 / TEE · 连平台也不可见", "Confidential computing / TEE · invisible to the platform too"],
    promise: [
      "计算在可信执行环境(TEE)内进行,连平台运营方都无法读取数据;作业附远程证明。",
      "Compute runs inside a trusted execution environment (TEE) so even the platform operator cannot read the data; jobs carry a remote attestation.",
    ],
    guarantees: [
      ["远程证明链路在:度量值绑定入隔离区运行的算法镜像", "Remote-attestation path exists: the measurement binds the algorithm image that ran in the enclave"],
      ["UI 对 L2 作业展示已验证 / 未通过的证明状态", "The UI shows attested / failed verdicts for L2 jobs"],
    ],
    limits: [
      ["真硬件 TEE(Intel TDX / AMD SEV)需机密计算云,当前 gated;未接入前 L2 标注为规划部署", "Real hardware TEE (Intel TDX / AMD SEV) needs a confidential-compute cloud and is currently gated; until deployed, L2 is marked as planned"],
    ],
  },
];

export default function TrustPage() {
  const { t } = useT();
  return (
    <div className="space-y-20 pb-16 pt-10">
      <section>
        <p className="font-mono text-kicker uppercase text-muted">
          {t("白皮书 · 一页式", "Whitepaper · single page")}
        </p>
        <h1 className="mt-4 max-w-3xl font-display text-display-md leading-[1.04] tracking-tight sm:text-display-lg">
          {t("可验证性与信任分级", "Verifiability & trust tiers")}
        </h1>
        <p className="mt-6 max-w-2xl text-base leading-relaxed text-ink/80 sm:text-lg">
          {t(
            "招牌是「数据可用不可见」。但隐私保证不该靠口号——下面诚实写清每一档现在真正保证什么、还差什么,以及任何第三方如何独立核验一次计算的结果。",
            "Our signature is available-but-invisible data. But privacy guarantees shouldn't rest on slogans — below is an honest account of what each tier really guarantees today, what's still missing, and how any third party can independently verify a result.",
          )}
        </p>
      </section>

      <section className="rounded-2xl border border-rule bg-white px-6 py-8 sm:px-10">
        <ComputeFlowDiagram />
      </section>

      <section className="space-y-12">
        {TIERS.map((tier) => (
          <article key={tier.tag} className="border-t border-rule pt-10">
            <div className="flex items-baseline gap-4">
              <span className="font-mono text-[11px] uppercase tracking-[0.24em] text-forest-700">{tier.tag}</span>
              <h2 className="font-display text-3xl leading-tight tracking-tight sm:text-display-sm">
                {t(tier.name[0], tier.name[1])}
              </h2>
            </div>
            <p className="mt-4 max-w-3xl text-base leading-relaxed text-ink/75">{t(tier.promise[0], tier.promise[1])}</p>
            <div className="mt-6 grid gap-x-10 gap-y-8 md:grid-cols-2">
              <div>
                <p className="font-mono text-kicker uppercase text-forest-700">
                  {t("真实保证", "What it really guarantees")}
                </p>
                <ul className="mt-3 space-y-2.5 border-t border-rule pt-3">
                  {tier.guarantees.map(([zh, en]) => (
                    <li key={en} className="flex gap-3 text-sm leading-relaxed text-ink/85">
                      <span className="mt-1.5 inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-forest-600" aria-hidden />
                      <span>{t(zh, en)}</span>
                    </li>
                  ))}
                </ul>
              </div>
              <div>
                <p className="font-mono text-kicker uppercase text-gold-700">{t("诚实边界", "Honest limits")}</p>
                <ul className="mt-3 space-y-2.5 border-t border-rule pt-3">
                  {tier.limits.map(([zh, en]) => (
                    <li key={en} className="flex gap-3 text-sm leading-relaxed text-ink/85">
                      <span className="mt-1.5 inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-gold-600" aria-hidden />
                      <span>{t(zh, en)}</span>
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          </article>
        ))}
      </section>

      <section>
        <p className="font-mono text-kicker uppercase text-muted">
          {t("公开协议", "Open protocol")}
        </p>
        <h2 className="mt-4 max-w-3xl font-display text-display-sm leading-tight tracking-tight">
          {t("如何独立核验一次计算", "How to independently verify a computation")}
        </h2>
        <ol className="mt-8 space-y-7 border-t border-rule pt-7">
          {[
            [
              "每次放行的计算结果都出具一张存证凭证(VO-…),把输出的 SHA-256 绑定到已审核算法的镜像 digest。",
              "Every released result issues a certificate (VO-…) binding the output's SHA-256 to the audited algorithm's image digest.",
            ],
            [
              "任何人——无需登录——可在验真页输入凭证号核验:它确认这份输出确由声明的算法、对声明的数据产生。",
              "Anyone — no login — can enter the certificate ID on the verify page: it confirms the output came from the stated algorithm over the stated data.",
            ],
            [
              "L2 作业额外附远程证明,其度量值应等于声明的算法镜像 digest —— 度量值对不上即证明造假。",
              "L2 jobs additionally carry a remote attestation whose measurement should equal the stated algorithm image digest — a mismatch means a forged attestation.",
            ],
          ].map(([zh, en], i) => (
            <li key={en} className="grid grid-cols-[3rem_1fr] items-baseline gap-x-6">
              <span className="font-mono text-2xl leading-none text-forest-700">{String(i + 1).padStart(2, "0")}</span>
              <span className="text-base leading-relaxed text-ink/85">{t(zh, en)}</span>
            </li>
          ))}
        </ol>
        <div className="mt-10 border-t border-rule pt-6">
          <Link href="/verify" className="inline-flex items-center gap-2 rounded-full bg-ink px-5 py-2.5 text-sm font-medium text-paper hover:bg-ink/85">
            {t("去验真一张凭证", "Verify a certificate")}
            <span aria-hidden>→</span>
          </Link>
        </div>
      </section>
    </div>
  );
}
