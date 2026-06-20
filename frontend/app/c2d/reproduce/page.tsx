"use client";

import Link from "next/link";
import { useT } from "@/lib/i18n";
import { PageHeader } from "@/components/ui";
import { Reveal } from "@/components/Reveal";

const REPRO_DOC_URL = encodeURI(
  "https://github.com/exergyleizhou-ux/ai-data-marketplace/blob/main/docs/复现-独立验证C2D结果.md",
);

// The three reproduction paths, lightest → most thorough.
const PATHS: { n: string; zh: { t: string; d: string }; en: { t: string; d: string }; code: string }[] = [
  {
    n: "01",
    zh: { t: "重算哈希(最快)", d: "下载这次计算的结果,本地重算 SHA-256,与证书里的 output_sha256 比对。一致 ⇒ 结果未被篡改、确由该证书所述的计算产生。" },
    en: { t: "Re-hash (fastest)", d: "Download the result, re-hash it locally, compare to the certificate's output_sha256. Match ⇒ the result is untampered and produced by exactly the computation the cert describes." },
    code: "shasum -a 256 output.bin   # == cert.output_sha256 ?",
  },
  {
    n: "02",
    zh: { t: "公开验证页(无需登录)", d: "打开 /verify,输入 VO-… 编号。页面显示该存证是否已登记/可验证、资源类型与登记时间。常规结果与联邦/PSI 联合结果都在这个公开索引里。" },
    en: { t: "Public verify page (no login)", d: "Open /verify and enter the VO-… id. It shows whether the cert is registered/verifiable, its resource type and registration time. Regular and federated/PSI results are all in this public index." },
    code: "open /verify?cert=VO-…",
  },
  {
    n: "03",
    zh: { t: "在你自己的数据上复跑(最彻底)", d: "算法按 digest 钉死、源码逐行可审。拉取证书里那一行镜像,用与生产完全相同的沙箱姿态(--network=none --read-only --user 65534),在一份你已知答案的数据上复跑,确认它确实在做它声称的事。" },
    en: { t: "Re-run on your own data (most thorough)", d: "The algorithm is digest-pinned and source-auditable. Pull the exact image from the cert and re-run it on data whose answer you already know, in the identical production sandbox posture — confirm it does what it claims." },
    code:
      "docker run --rm --network=none --read-only --user 65534:65534 \\\n" +
      "  -v \"$PWD/data:/data:ro\" -v \"$PWD/out:/out\" \\\n" +
      "  -v \"$PWD/params.json:/params.json:ro\" \\\n" +
      "  <registry>/vo-<algo>@sha256:<digest>",
  },
];

export default function ReproducePage() {
  const { t, lang } = useT();
  const L = <T,>(o: { zh: T; en: T }) => (lang === "en" ? o.en : o.zh);
  return (
    <div className="max-w-2xl space-y-10 pb-20">
      <PageHeader
        kicker={t("绿洲 · 可复现性工具", "Verdant Oasis · reproducibility instrument")}
        title={t("证书是一份可重跑的、防篡改的记录", "The certificate is a re-runnable, tamper-evident record")}
        subtitle={t(
          "每张结果存证不只是「验真」——它把方法、数据、结果钉在一起,任何人都能独立重跑或重算。可信的核心不是相信我们,而是你能自己复现。",
          "Every result certificate is more than a checkmark — it pins the method, the data, and the result together so anyone can independently re-run or re-hash. Trust here is not believing us; it is being able to reproduce it yourself.",
        )}
      />

      {/* method + data + result */}
      <Reveal className="rounded-xl border border-forest-200 bg-forest-50/40 p-5">
        <div className="font-mono text-kicker uppercase tracking-widest text-forest-700">
          {t("方法 + 数据 + 结果", "method + data + result")}
        </div>
        <p className="mt-2 font-mono text-xs leading-relaxed text-ink/80">
          {t("结果指纹 (output SHA-256)", "result fingerprint (output SHA-256)")}
          <span className="text-muted"> ⟵ {t("由…产出", "produced by")} ⟶ </span>
          {t("已审核算法(镜像 digest 钉死)+ 源数据集", "audited algorithm (pinned image digest) + source dataset")}
        </p>
        <p className="mt-2 text-xs leading-relaxed text-ink/70">
          {t(
            "镜像 digest 钉死 ⇒ 代码不可被悄悄替换;--network=none 只读沙箱 ⇒ 数据可用不可见,只有聚合离开;固定种子 ⇒ 同方法同数据同结果。",
            "Pinned image digest ⇒ the code can't be silently swapped; --network=none read-only sandbox ⇒ data is usable-but-unseen, only aggregates leave; fixed seeds ⇒ same method + data → same result.",
          )}
        </p>
      </Reveal>

      {/* three paths */}
      <section className="space-y-3">
        <h2 className="font-display text-lg text-ink">{t("三种独立复现方式", "Three ways to reproduce, independently")}</h2>
        <ul className="space-y-2.5">
          {PATHS.map((p, i) => (
            <Reveal as="li" key={p.n} delay={i * 50} className="rounded-xl border border-rule bg-white p-4">
              <div className="flex items-baseline gap-3">
                <span className="font-mono text-kicker uppercase tracking-widest text-forest-700">{p.n}</span>
                <span className="font-medium text-ink">{L(p).t}</span>
              </div>
              <p className="mt-1.5 text-xs leading-relaxed text-ink/70">{L(p).d}</p>
              <pre className="mt-2.5 overflow-x-auto rounded-lg bg-ink/95 p-3 text-[11px] leading-relaxed text-paper">
                <code>{p.code}</code>
              </pre>
            </Reveal>
          ))}
        </ul>
      </section>

      {/* worked example: the PaperGuard validation */}
      <Reveal className="rounded-xl border border-rule bg-paper/60 p-5">
        <h3 className="text-sm font-semibold text-ink">{t("一个做到底的例子:可复现的方法学验证", "A worked example: a reproducible methodology validation")}</h3>
        <p className="mt-2 text-xs leading-relaxed text-ink/70">
          {t(
            "完整性筛查算法的统计操作特性,是在 digest 钉死的生产镜像里、用固定种子跑出来的,逐字节可复现——灵敏度与差分响应都是 1.00,而假阳性率则被诚实地标为「需真实数据才能测」。方法、数据、结果与诚实边界一并公开。",
            "The integrity screen's statistical operating characteristics were produced inside the digest-pinned production image with a fixed seed — byte-for-byte reproducible: sensitivity and differential response both 1.00, while the false-positive rate is honestly marked 'needs real data to measure'. Method, data, result, and honest scope are all published.",
          )}
        </p>
        <p className="mt-2 font-mono text-[11px] text-muted">algorithms/paperguard/validate.py · validation_results.json</p>
      </Reveal>

      {/* honest scope */}
      <section className="grid gap-3 sm:grid-cols-2">
        <Reveal className="rounded-xl border border-forest-200 bg-forest-50/40 p-4">
          <h3 className="text-sm font-semibold text-forest-800">{t("复现能证明", "Reproducing proves")}</h3>
          <p className="mt-2 text-xs leading-relaxed text-ink/80">
            {t("结果完整性(未篡改)、计算溯源(哪个钉死算法、对哪份数据)、隐私姿态(数据不出沙箱、只出聚合)。", "Result integrity (untampered), provenance (which pinned algorithm, on which dataset), and the privacy posture (data stays in the sandbox, only aggregates leave).")}
          </p>
        </Reveal>
        <Reveal delay={60} className="rounded-xl border border-rule bg-white p-4">
          <h3 className="text-sm font-semibold text-ink">{t("复现不证明", "Reproducing does NOT prove")}</h3>
          <p className="mt-2 text-xs leading-relaxed text-ink/70">
            {t("算法的科学结论正确;第三方/链上公证(本证书为平台自出);硬件级机密性(L2 真 TEE 仍受限)。详见诚实分级。", "That the algorithm's scientific conclusion is correct; third-party/on-chain notarization (the cert is platform-issued); hardware confidentiality (real L2 TEE is gated). See the honest status page.")}
          </p>
        </Reveal>
      </section>

      <div className="flex flex-wrap items-center gap-4 pt-1">
        <Link href="/c2d" className="font-medium text-forest-700 hover:underline">{t("← 返回可信计算总览", "← Back to the compute-to-data overview")}</Link>
        <a href={REPRO_DOC_URL} target="_blank" rel="noreferrer" className="text-xs font-medium text-forest-700 hover:underline">{t("完整复现指南 →", "Full reproduce guide →")}</a>
        <Link href="/c2d/honesty" className="text-xs font-medium text-forest-700 hover:underline">{t("诚实分级 →", "Honest status →")}</Link>
      </div>
    </div>
  );
}
