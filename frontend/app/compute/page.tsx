"use client";

import { useState } from "react";
import { useT } from "@/lib/i18n";
import { Protected } from "@/components/Protected";
import {
  MyEntitlementsPanel,
  MyComputeJobsPanel,
  FederatedComputePanel,
  PSIComputePanel,
  MyAlgorithmRequestsPanel,
} from "@/components/Compute";

type Tab = "entitlements" | "jobs" | "federated" | "psi" | "algorithms";

export default function ComputePage() {
  return (
    <Protected>
      <ComputeInner />
    </Protected>
  );
}

function ComputeInner() {
  const { t } = useT();
  const [tab, setTab] = useState<Tab>("jobs");
  const labels: Record<Tab, string> = {
    jobs: t("计算作业", "Compute jobs"),
    entitlements: t("算力权益", "Entitlements"),
    federated: t("联邦学习", "Federated"),
    psi: t("隐私求交 PSI", "PSI"),
    algorithms: t("申请算法", "Submit algorithm"),
  };
  return (
    <div className="space-y-8 pt-2">
      <div>
        <p className="font-mono text-kicker uppercase text-forest-700">
          {t("可用不可见 · 沙箱计算", "Available-but-invisible · sandbox compute")}
        </p>
        <h1 className="mt-3 font-display text-display-sm leading-tight tracking-tight">
          {t("隐私计算", "Privacy compute")}
        </h1>
        <p className="mt-3 max-w-2xl text-sm leading-relaxed text-ink/70">
          {t(
            "统一入口:发起并跟踪你的常规计算、联邦学习与隐私求交作业,管理算力权益,查看存证与远程证明。",
            "One home: start and track your regular, federated, and PSI jobs, manage entitlements, and view provenance & attestation.",
          )}
        </p>
      </div>
      <div className="flex flex-wrap gap-2 border-b border-rule pb-4">
        {(["jobs", "entitlements", "federated", "psi", "algorithms"] as const).map((tb) => (
          <button
            key={tb}
            onClick={() => setTab(tb)}
            className={`rounded-full px-4 py-1.5 text-sm transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2 ${
              tab === tb ? "bg-ink text-paper" : "border border-rule bg-white text-ink/70 hover:bg-paper"
            }`}
          >
            {labels[tb]}
          </button>
        ))}
      </div>
      {tab === "jobs" && <MyComputeJobsPanel />}
      {tab === "entitlements" && <MyEntitlementsPanel />}
      {tab === "federated" && <FederatedComputePanel />}
      {tab === "psi" && <PSIComputePanel />}
      {tab === "algorithms" && <MyAlgorithmRequestsPanel />}
    </div>
  );
}
