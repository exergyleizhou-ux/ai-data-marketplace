"use client";
import { useState } from "react";
import { useT } from "@/lib/i18n";
import { isTerminalRunStatus, runtimeLinks, type LabArtifact, type LabRuntimeDetail, type WorkbenchSnapshot } from "@/lib/workbench-runtime";

export type RuntimeService = { status: "ok" | "degraded" | "down"; reason_code: string; next_action: string };
type Props = { snapshot: WorkbenchSnapshot | null; detail: LabRuntimeDetail | null; loading: boolean; error: string; canceling: boolean; retrying?: boolean; services?: Record<string, RuntimeService>; onCancel: () => void; onRetry?: (prompt: string) => void };
function artifactHref(snapshot: WorkbenchSnapshot, artifact: LabArtifact) {
  if (snapshot.surface === "code" && snapshot.run) return `/api/lumen/code/v1/runs/${snapshot.run.id}/artifacts/${encodeURIComponent(artifact.path)}`;
  const query = new URLSearchParams({ project_id: snapshot.project?.id ?? "", path: artifact.path });
  return `/api/lumen/lab/api/lab/files/download?${query}`;
}
const verificationLabel = (value: string | undefined, t: (zh: string, en: string) => string) => ({ passed: t("已通过", "passed"), failed: t("失败", "failed"), not_run: t("未运行", "not run"), running: t("验证中", "running"), idle: t("空闲", "idle") })[value ?? "idle"];

export function WorkbenchRuntimePanel({ snapshot, detail, loading, error, canceling, retrying = false, services = {}, onCancel, onRetry }: Props) {
  const { t } = useT(); const [expanded, setExpanded] = useState(false); const [retryPrompt, setRetryPrompt] = useState(""); const run = detail?.run ?? null;
  const status = run?.status ?? snapshot?.run?.status ?? (snapshot?.run ? t("同步中", "syncing") : t("空闲", "idle"));
  const terminal = run ? isTerminalRunStatus(run.status) : snapshot?.run?.terminal === true; const pending = snapshot?.pending_approvals ?? 0; const links = snapshot ? runtimeLinks(snapshot) : null;
  const unhealthy = Object.entries(services).filter(([, service]) => service.status !== "ok");
  return <div className="relative">
    <button type="button" aria-label={t("运行详情", "Runtime details")} aria-expanded={expanded} onClick={() => setExpanded(v => !v)} className="flex min-h-11 items-center gap-2 rounded-md border border-ink/10 bg-white px-3 text-xs text-ink/70 hover:border-forest/30 focus-visible:outline focus-visible:outline-2 focus-visible:outline-forest">
      <span aria-hidden="true" className={`h-2 w-2 rounded-full ${status === "running" || status === "waiting_approval" || status === "verifying" ? "bg-amber-500" : run && terminal ? run.status === "succeeded" ? "bg-forest" : "bg-red-500" : "bg-ink/25"}`} />
      <span>{snapshot?.surface === "code" ? "Code" : "Lab"}: {status}</span>{pending > 0 && <span className="rounded-full bg-amber-50 px-1.5 text-amber-800">{pending}</span>}
    </button>
    {expanded && <section aria-label={t("共享 Runtime 状态", "Shared runtime status")} className="fixed inset-x-3 top-24 z-30 max-h-[calc(100dvh-7rem)] overflow-y-auto rounded-xl border border-ink/10 bg-white p-4 text-left shadow-xl sm:absolute sm:inset-x-auto sm:right-0 sm:top-full sm:mt-2 sm:w-96">
      <header className="mb-3"><h2 className="text-sm font-semibold text-ink">{snapshot?.surface === "code" ? "Lumen Code" : "Lumen Lab"}</h2><p className="text-xs text-ink/50">{snapshot?.project?.title ?? snapshot?.workspace?.id ?? t("尚未选择项目", "No project selected")}</p></header>
      {!snapshot?.run ? <p className="rounded-lg bg-ink/[0.03] p-3 text-sm text-ink/60">{t("暂无活跃 Run。", "No active Run.")}</p> : <>
        <dl className="grid grid-cols-[6rem_minmax(0,1fr)] gap-2 text-xs"><dt className="text-ink/45">Run ID</dt><dd className="break-all font-mono">{snapshot.run.id}</dd><dt className="text-ink/45">{t("状态", "Status")}</dt><dd>{status}{terminal ? ` · ${t("终态", "terminal")}` : ""}</dd><dt className="text-ink/45">{t("事件", "Events")}</dt><dd>{detail?.events.length ?? 0} {t("个事件", "events")}</dd><dt className="text-ink/45">{t("验证", "Verification")}</dt><dd>{verificationLabel(snapshot.verification, t)}</dd><dt className="text-ink/45">{t("产物", "Artifacts")}</dt><dd>{snapshot.artifact_count ?? detail?.artifacts.length ?? 0}</dd></dl>
        <div className="mt-3 flex flex-wrap gap-2">{!terminal && <button type="button" disabled={canceling} onClick={onCancel} className="min-h-11 rounded-md border border-red-200 px-3 text-xs text-red-700 disabled:opacity-50">{canceling ? t("取消中…", "Canceling…") : t("取消 Run", "Cancel Run")}</button>}{links && <a className="flex min-h-11 items-center rounded-md border border-ink/15 px-3 text-xs text-forest" href={links.evidence} download>{t("下载证据", "Download evidence")}</a>}</div>
        {terminal && onRetry && <form className="mt-3 space-y-2" onSubmit={event => { event.preventDefault(); if (retryPrompt.trim()) onRetry(retryPrompt.trim()); }}><label htmlFor="retry-prompt" className="block text-xs font-medium">{t("新的重试指令", "New retry prompt")}</label><textarea id="retry-prompt" required value={retryPrompt} onChange={event => setRetryPrompt(event.target.value)} maxLength={8000} rows={3} className="w-full rounded-md border border-ink/20 p-2 text-sm" placeholder={t("输入新的指令；不会复用旧 prompt", "Enter a new prompt; the previous prompt is never reused")} /><button type="submit" disabled={retrying || !retryPrompt.trim()} className="min-h-11 rounded-md border border-forest/30 px-3 text-xs text-forest disabled:opacity-50">{retrying ? t("重试中…", "Retrying…") : t("创建关联的新 Run", "Create linked Run")}</button></form>}
        {pending > 0 && links && <a href={links.approvals} className="mt-3 block rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900 focus-visible:outline focus-visible:outline-2 focus-visible:outline-forest"><strong>{pending} {t("个待审批", "pending approvals")}</strong><span className="ml-2 underline">{t("立即审阅", "Review now")}</span></a>}
        {run?.stop_reason && <p className="mt-3 break-words text-xs"><span className="text-ink/45">{t("终止原因", "Stop reason")}: </span>{run.stop_reason}</p>}{run?.error && <p role="alert" className="mt-2 break-words rounded-lg bg-red-50 p-2 text-xs text-red-800">{run.error}</p>}
      </>}
      {loading && <p role="status" className="mt-3 text-xs text-ink/50">{t("同步 Runtime…", "Syncing runtime…")}</p>}{error && <p role="alert" className="mt-3 text-xs text-red-700">{error}</p>}
      {detail && detail.events.length > 0 && <section className="mt-4 border-t border-ink/10 pt-3"><h3 className="text-xs font-semibold uppercase tracking-wide text-ink/45">{t("时间线", "Timeline")}</h3><ol className="mt-2 max-h-36 space-y-1 overflow-y-auto" aria-label={t("Run 时间线", "Run timeline")}>{detail.events.map(e => <li key={e.seq} className="text-xs"><span className="mr-2 font-mono text-ink/40">#{e.seq}</span>{e.kind}</li>)}</ol></section>}
      {snapshot && detail?.artifacts.length ? <section className="mt-4 border-t border-ink/10 pt-3"><h3 className="text-xs font-semibold uppercase tracking-wide text-ink/45">{t("产物下载", "Artifact downloads")}</h3><ul className="mt-2 space-y-1">{detail.artifacts.map(a => <li key={a.path} className="flex items-center gap-2"><a href={artifactHref(snapshot, a)} download className="min-h-11 min-w-0 flex-1 break-all py-2 text-xs text-forest underline">{a.name}</a><span className="shrink-0 text-xs text-ink/40">({a.size} B)</span></li>)}</ul></section> : null}
      {unhealthy.length > 0 && <section className="mt-4 border-t border-ink/10 pt-3"><h3 className="text-xs font-semibold uppercase tracking-wide text-ink/45">{t("服务修复动作", "Service recovery actions")}</h3><ul className="mt-2 space-y-2">{unhealthy.map(([name, service]) => <li key={name} className="rounded bg-amber-50 p-2 text-xs"><strong>{name}: {service.status}</strong><p>{service.reason_code}</p><p>{t("下一步", "Next action")}: {service.next_action}</p></li>)}</ul></section>}
    </section>}
  </div>;
}
