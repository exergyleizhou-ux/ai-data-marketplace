"use client";

import Link from "next/link";
import { useT } from "@/lib/i18n";
import { PageHeader } from "@/components/ui";

// /build — positions Lumen (the security-first terminal coding agent) as the
// official toolchain for authoring the audited algorithms that run inside the
// Oasis C2D sandbox. The story: Lumen writes the code → Oasis runs it privately
// on the seller's data → the buyer gets a verifiable result. Two projects, one
// pipeline. Content is accurate to the real algorithm contract (algorithms/*).

function Code({ children }: { children: string }) {
  return (
    <pre className="overflow-x-auto rounded-xl border border-rule bg-white p-4 font-mono text-[13px] leading-relaxed text-ink">
      {children}
    </pre>
  );
}

export default function BuildPage() {
  const { t } = useT();
  return (
    <div className="space-y-20 pb-16">
      <PageHeader
        kicker={t("工具链 · 开源", "Toolchain · open source")}
        title={t("用 Lumen 构建算法", "Build with Lumen")}
        subtitle={t(
          "Lumen 是一个安全优先的终端编码 agent。用它编写在 Oasis 沙箱里运行的、经审核的算法——数据不动,你的代码走进去,买家只取走可验真的结果。",
          "Lumen is a security-first terminal coding agent. Use it to write the audited algorithms that run inside the Oasis sandbox — the data stays put, your code goes to it, and the buyer takes only a verifiable result.",
        )}
      />

      {/* The pipeline */}
      <section>
        <p className="font-mono text-kicker uppercase text-forest-700">{t("一条流水线", "One pipeline")}</p>
        <div className="mt-5 grid gap-px overflow-hidden rounded-2xl border border-rule bg-rule sm:grid-cols-3">
          {[
            { n: "01", h: t("Lumen 写算法", "Lumen writes it"), d: t("在你本机用 Lumen 编写并本地测试算法——plan 模式、细粒度权限、5 层命令防护。", "Author and test the algorithm locally with Lumen — plan mode, fine-grained permissions, a 5-layer command guard.") },
            { n: "02", h: t("Oasis 跑算法", "Oasis runs it"), d: t("提交审核后,算法在卖家域的隔离沙箱内对数据运行,原始数据不出域。", "After review, it runs against the data in an isolated sandbox in the seller's domain — raw data never leaves.") },
            { n: "03", h: t("买家取结果", "Buyer gets the result"), d: t("只有结果出域,绑定算法镜像 digest 的存证可独立核验。", "Only the result exits, with a certificate bound to the algorithm's image digest — independently verifiable.") },
          ].map((s) => (
            <article key={s.n} className="bg-white p-6">
              <p className="font-mono text-2xl leading-none text-forest-700">{s.n}</p>
              <h3 className="mt-3 font-display text-2xl leading-snug tracking-tight">{s.h}</h3>
              <p className="mt-2 text-sm leading-relaxed text-ink/70">{s.d}</p>
            </article>
          ))}
        </div>
      </section>

      {/* Why Lumen */}
      <section>
        <p className="font-mono text-kicker uppercase text-muted">{t("为什么是 Lumen", "Why Lumen")}</p>
        <h2 className="mt-4 max-w-3xl font-display text-display-sm leading-tight tracking-tight">
          {t("安全优先的 agent,配安全优先的市场", "A security-first agent for a security-first marketplace")}
        </h2>
        <ul className="mt-6 grid gap-x-10 gap-y-4 border-t border-rule pt-6 sm:grid-cols-2">
          {/* Kept evergreen on purpose: Lumen iterates fast, so these state
              stable architecture, not volatile counts (exact model/provider
              numbers + binary size live in Lumen's own README, linked below). */}
          {[
            t("多种权限模式 + 分层 bash 防护 + 文件系统边界——它不无条件信任模型", "Layered permission modes + a bash command guard + filesystem boundaries — it doesn't trust the model unconditionally"),
            t("多模型、多供应商(DeepSeek / OpenAI / Grok / Ollama…),一个会话内随时切换", "Many models across providers (DeepSeek / OpenAI / Grok / Ollama…), switchable within a single session"),
            t("单个 Go 二进制(~10MB),无运行时依赖", "A single Go binary (~10 MB), no runtime dependency"),
            t("会话时间线 + /replay + 变更收件箱,全程可观测", "Session timeline + /replay + a change inbox — fully observable"),
          ].map((li) => (
            <li key={li} className="flex gap-3 text-sm leading-relaxed text-ink/85">
              <span className="mt-1.5 inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-forest-600" aria-hidden />
              <span>{li}</span>
            </li>
          ))}
        </ul>
      </section>

      {/* Install */}
      <section>
        <p className="font-mono text-kicker uppercase text-muted">{t("安装", "Install")}</p>
        <h2 className="mt-4 font-display text-display-sm leading-tight tracking-tight">{t("拿到 Lumen", "Get Lumen")}</h2>
        <p className="mt-3 max-w-2xl text-sm leading-relaxed text-ink/70">
          {t("需要 Go 1.22+。从源码构建出单个二进制:", "Needs Go 1.22+. Build the single binary from source:")}
        </p>
        <div className="mt-5">
          <Code>{`git clone https://github.com/exergyleizhou-ux/lumen
cd lumen && go build -o bin/lumen ./cmd/lumen
./bin/lumen run --plan "scaffold an Oasis C2D algorithm"`}</Code>
        </div>
      </section>

      {/* The contract */}
      <section>
        <p className="font-mono text-kicker uppercase text-muted">{t("算法契约", "The contract")}</p>
        <h2 className="mt-4 max-w-3xl font-display text-display-sm leading-tight tracking-tight">
          {t("一个沙箱算法要遵守什么", "What a sandbox algorithm must honor")}
        </h2>
        <p className="mt-3 max-w-2xl text-sm leading-relaxed text-ink/70">
          {t(
            "容器以无网络方式运行:数据集只读挂载在 /data,算法把单一结果对象写到 /out/output.bin。让 Lumen 照这个契约生成骨架:",
            "The container runs with no network: the dataset is mounted read-only at /data, and the algorithm writes a single result object to /out/output.bin. Have Lumen scaffold to this contract:",
          )}
        </p>
        <div className="mt-5">
          <Code>{`# start from the canonical scaffold (runs out of the box):
cp -r algorithms/_template algorithms/myalgo
#   → edit algorithms/myalgo/train.py : compute(df, params)

# the sandbox invokes your image like this (no network, read-only data):
docker run --network=none --read-only --tmpfs=/tmp:rw,size=64m \\
  -v <dataset>:/data:ro -v <out>:/out -v <params>:/params.json:ro \\
  your-algorithm:tag

# the contract:
#   1. read the first tabular file under /data        (read-only)
#   2. compute AGGREGATES only — never per-row outputs (leakage)
#   3. write /out/output.bin  =  zip(model.json, metrics.json)
# the platform hashes output.bin and binds it to your image digest
# in the buyer's certificate (VO-xxxxxxxx).`}</Code>
        </div>
        <p className="mt-4 text-sm leading-relaxed text-ink/60">
          {t(
            "起点是 algorithms/_template(开箱即跑的脚手架);完整范例:logreg(分类)、kmeans(聚类)、dp_stats(差分隐私统计)、fed-logreg(联邦)。让 Lumen 读它们当模板。",
            "Start from algorithms/_template (a scaffold that runs out of the box); full worked examples: logreg (classification), kmeans (clustering), dp_stats (DP statistics), fed-logreg (federated). Have Lumen read them as templates.",
          )}
        </p>
      </section>

      {/* Honest note + CTA */}
      <section className="rounded-2xl border border-rule bg-paper/60 p-6">
        <p className="text-sm leading-relaxed text-ink/70">
          {t(
            "诚实标注:Lumen 是独立的开源 CLI,在你本机运行;它不接管 Oasis 沙箱。你提交的算法仍要过平台审核、按既有信任分级(L1/L2/L3)在沙箱内运行——是这套审核 + 沙箱 + 存证给了买家担保,不是写代码的工具。",
            "Honest note: Lumen is a standalone open-source CLI that runs on your machine; it does not take over the Oasis sandbox. The algorithm you submit still goes through platform review and runs in the sandbox under the existing trust tiers (L1/L2/L3) — it's that review + sandbox + certificate chain that gives the buyer the guarantee, not the tool that wrote the code.",
          )}
        </p>
        <div className="mt-6 flex flex-wrap gap-x-6 gap-y-2">
          <Link href="/compute" className="text-sm font-medium text-ink cue-underline hover:text-forest-700">
            {t("去提交算法(隐私计算中心)→", "Submit an algorithm (privacy-compute hub) →")}
          </Link>
          <Link href="/trust" className="text-sm font-medium text-ink cue-underline hover:text-forest-700">
            {t("信任分级如何保证 →", "How the trust tiers guarantee it →")}
          </Link>
          <a
            href="https://github.com/exergyleizhou-ux/lumen"
            target="_blank"
            rel="noreferrer"
            className="text-sm font-medium text-ink cue-underline hover:text-forest-700"
          >
            {t("Lumen 源码 →", "Lumen source →")}
          </a>
        </div>
      </section>
    </div>
  );
}
