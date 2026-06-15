"use client";

import { useState } from "react";
import { useT } from "@/lib/i18n";
import { Protected } from "@/components/Protected";
import {
  MyEntitlementsPanel,
  MyComputeJobsPanel,
  FederatedComputePanel,
  PSIComputePanel,
} from "@/components/Compute";

type Tab = "entitlements" | "jobs" | "federated" | "psi";

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
  };
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">{t("隐私计算", "Privacy Compute")}</h1>
        <p className="mt-1 text-sm text-neutral-500">
          {t(
            "「可用不可见」沙箱计算的统一入口:发起并跟踪你的常规计算、联邦学习与隐私求交作业,管理算力权益,查看存证与远程证明。",
            "One home for available-but-invisible sandbox compute: start and track your regular, federated, and PSI jobs, manage entitlements, and view provenance & attestation.",
          )}
        </p>
      </div>
      <div className="flex flex-wrap gap-2">
        {(["jobs", "entitlements", "federated", "psi"] as const).map((tb) => (
          <button
            key={tb}
            onClick={() => setTab(tb)}
            className={`rounded-md px-4 py-1.5 text-sm ${
              tab === tb ? "bg-neutral-900 text-white" : "border border-neutral-300 bg-white text-neutral-700"
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
    </div>
  );
}
