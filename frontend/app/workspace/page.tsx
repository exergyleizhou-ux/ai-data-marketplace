"use client";

import Link from "next/link";
import { Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useT } from "@/lib/i18n";

type TabId = "coding" | "science" | "lab";

const TABS: { id: TabId; zh: string; en: string; src: string; blurbZh: string; blurbEn: string }[] = [
  {
    id: "coding",
    zh: "编程智能体",
    en: "Coding agent",
    src: "/lumen/",
    blurbZh: "对标 Claude Code · 终端/桌面编程",
    blurbEn: "Claude Code–class coding agent",
  },
  {
    id: "science",
    zh: "Science 桥",
    en: "Science bridge",
    src: "/lumen-science/?embed=1&oasis=1",
    blurbZh: "国产模型接入 Claude Science",
    blurbEn: "Domestic models → Claude Science",
  },
  {
    id: "lab",
    zh: "实验室",
    en: "Lab",
    src: "/lumen-lab/?embed=1&oasis=1",
    blurbZh: "自主科研工作台 · 审批 · 5-ship MCP",
    blurbEn: "Autonomous lab · approvals · 5-ship MCP",
  },
];

function parseTab(raw: string | null): TabId {
  if (raw === "science" || raw === "lumen-science") return "science";
  if (raw === "lab" || raw === "lumen-lab") return "lab";
  if (raw === "coding" || raw === "lumen" || raw === "code") return "coding";
  return "lab";
}

function WorkbenchInner() {
  const { t } = useT();
  const search = useSearchParams();
  const initial = useMemo(() => parseTab(search.get("tab")), [search]);
  const [tab, setTab] = useState<TabId>(initial);
  const [labHealth, setLabHealth] = useState<"ok" | "down" | "loading">("loading");

  useEffect(() => {
    setTab(parseTab(search.get("tab")));
  }, [search]);

  useEffect(() => {
    if (tab !== "lab") return;
    let cancelled = false;
    (async () => {
      try {
        const r = await fetch("/api/lab/health", { cache: "no-store" });
        if (!cancelled) setLabHealth(r.ok ? "ok" : "down");
      } catch {
        if (!cancelled) setLabHealth("down");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [tab]);

  const active = TABS.find((x) => x.id === tab) ?? TABS[2];

  const select = useCallback((id: TabId) => {
    setTab(id);
    const url = new URL(window.location.href);
    url.searchParams.set("tab", id);
    window.history.replaceState({}, "", `${url.pathname}?${url.searchParams.toString()}`);
  }, []);

  return (
    <div className="flex h-full min-h-0 flex-col bg-paper">
      <div className="flex flex-wrap items-center gap-2 border-b border-ink/10 px-3 py-2 sm:px-4">
        <span className="mr-1 text-sm font-semibold text-ink">{t("科研工作台", "Workbench")}</span>
        <div className="flex flex-wrap gap-1" role="tablist" aria-label={t("能力切换", "Capabilities")}>
          {TABS.map((tb) => {
            const on = tb.id === tab;
            return (
              <button
                key={tb.id}
                type="button"
                role="tab"
                aria-selected={on}
                onClick={() => select(tb.id)}
                className={`rounded-md px-3 py-1.5 text-sm transition ${
                  on
                    ? "bg-forest/15 font-medium text-forest"
                    : "text-ink/60 hover:bg-ink/5 hover:text-ink"
                }`}
              >
                {t(tb.zh, tb.en)}
              </button>
            );
          })}
        </div>
        <span className="hidden text-xs text-ink/45 sm:inline">{t(active.blurbZh, active.blurbEn)}</span>
        {tab === "lab" && (
          <span
            className={`rounded-full px-2 py-0.5 text-[11px] ${
              labHealth === "ok"
                ? "bg-forest/10 text-forest"
                : labHealth === "down"
                  ? "bg-red-50 text-red-700"
                  : "bg-ink/5 text-ink/50"
            }`}
          >
            {labHealth === "ok"
              ? t("Lab 在线", "Lab online")
              : labHealth === "down"
                ? t("Lab 未连通", "Lab offline")
                : t("探测中…", "Probing…")}
          </span>
        )}
        <div className="ml-auto flex gap-2 text-xs">
          <Link href="/datasets" className="text-ink/50 hover:text-forest">
            ← {t("数据市场", "Marketplace")}
          </Link>
        </div>
      </div>

      {tab === "lab" && labHealth === "down" ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 p-8 text-center">
          <p className="text-base font-semibold text-ink">{t("实验室服务未连通", "Lab service offline")}</p>
          <p className="max-w-md text-sm text-ink/60">
            {t(
              "无法访问 /api/lab/health。请确认 lumen-lab 服务与反代配置。",
              "Cannot reach /api/lab/health. Check lumen-lab service and reverse proxy."
            )}
          </p>
        </div>
      ) : (
        <iframe
          key={active.id}
          src={active.src}
          title={t(active.zh, active.en)}
          className="min-h-0 w-full flex-1 border-0 bg-paper"
          allow="clipboard-read; clipboard-write"
          sandbox="allow-scripts allow-same-origin allow-forms allow-popups"
        />
      )}
    </div>
  );
}

export default function WorkspacePage() {
  return (
    <Suspense
      fallback={
        <div className="flex h-full items-center justify-center text-sm text-ink/50">Loading workbench…</div>
      }
    >
      <WorkbenchInner />
    </Suspense>
  );
}
