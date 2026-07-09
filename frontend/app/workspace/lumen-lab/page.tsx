"use client";

import { useEffect, useState } from "react";

type LabHealth = {
  status?: string;
  version?: string;
  science_mode?: string;
  research_pack?: { healthy?: boolean; domain_tools?: number; skills?: number };
  fleet?: { connected_total?: number; cs_domains?: number; lumen_native?: number };
  provider?: { set?: boolean; masked?: string; adapter?: string };
};

/**
 * Lumen Lab workspace embed.
 * Production: Caddy routes /lumen-lab/* and /api/lab/* to the Lab service (:18992).
 * Shows honest degraded state when Lab is offline (no silent blank iframe).
 */
export default function LumenLabWorkspacePage() {
  const [health, setHealth] = useState<LabHealth | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const r = await fetch("/api/lab/health", { cache: "no-store" });
        if (!r.ok) throw new Error(`Lab health HTTP ${r.status}`);
        const j = (await r.json()) as LabHealth;
        if (!cancelled) {
          setHealth(j);
          setErr(null);
          setReady(true);
        }
      } catch (e) {
        if (!cancelled) {
          setErr(e instanceof Error ? e.message : String(e));
          setReady(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  if (err) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 bg-paper p-8 text-center">
        <p className="text-lg font-semibold text-ink">Lumen Lab 未连通</p>
        <p className="max-w-md text-sm text-ink/70">
          无法访问 <code className="rounded bg-ink/5 px-1">/api/lab/health</code>
          。请确认 Caddy 已把 <code className="rounded bg-ink/5 px-1">/api/lab/*</code> 反代到 Lab
          服务（:18992），且 <code className="rounded bg-ink/5 px-1">lumen science lab</code>{" "}
          在跑。
        </p>
        <p className="text-xs text-ink/50">{err}</p>
        <a
          href="/datasets"
          className="mt-2 text-sm font-medium text-forest underline-offset-2 hover:underline"
        >
          ← 返回数据市场
        </a>
      </div>
    );
  }

  if (!ready) {
    return (
      <div className="flex h-full items-center justify-center bg-paper text-sm text-ink/60">
        正在连接 Lumen Lab…
      </div>
    );
  }

  const pack = health?.research_pack;
  const fleet = health?.fleet;

  return (
    <div className="flex h-full min-h-0 flex-col bg-paper">
      <div className="flex flex-wrap items-center gap-3 border-b border-ink/10 px-4 py-2 text-xs text-ink/70">
        <span className="font-semibold text-ink">Lumen Science Lab</span>
        <span className="rounded-full bg-forest/10 px-2 py-0.5 text-forest">
          {health?.status === "ok" ? "在线" : "降级"}
        </span>
        {health?.version && <span>v{health.version}</span>}
        {pack && (
          <span>
            Research {pack.healthy ? "✓" : "✗"} · {pack.domain_tools ?? 0} tools ·{" "}
            {pack.skills ?? 0} skills
          </span>
        )}
        {fleet && (
          <span>
            fleet {fleet.connected_total ?? 0}/{fleet.cs_domains ?? 0}
          </span>
        )}
        {health?.provider?.masked && <span>模型 {health.provider.masked}</span>}
        <span className="text-ink/40">Agent 模式默认开启审批卡</span>
      </div>
      <iframe
        src="/lumen-lab/?embed=1&oasis=1"
        title="Lumen Science 实验室"
        className="min-h-0 w-full flex-1 border-0 bg-paper"
        allow="clipboard-read; clipboard-write"
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups"
      />
    </div>
  );
}
