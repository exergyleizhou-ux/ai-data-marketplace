"use client";

import { useState } from "react";
import { useT } from "@/lib/i18n";
import {
  isTerminalRunStatus,
  type LabArtifact,
  type LabRuntimeDetail,
  type WorkbenchSnapshot,
} from "@/lib/workbench-runtime";

type Props = {
  snapshot: WorkbenchSnapshot | null;
  detail: LabRuntimeDetail | null;
  loading: boolean;
  error: string;
  canceling: boolean;
  onCancel: () => void;
};

function artifactHref(projectID: string, artifact: LabArtifact): string {
  const query = new URLSearchParams({ project_id: projectID, path: artifact.path });
  return `/lumen-lab/api/lab/files/download?${query.toString()}`;
}

export function WorkbenchRuntimePanel({
  snapshot,
  detail,
  loading,
  error,
  canceling,
  onCancel,
}: Props) {
  const { t } = useT();
  const [expanded, setExpanded] = useState(false);
  const run = detail?.run ?? null;
  const status = run?.status ?? (snapshot?.run ? t("同步中", "syncing") : t("空闲", "idle"));
  const terminal = run ? isTerminalRunStatus(run.status) : snapshot?.run?.terminal === true;
  const pending = snapshot?.pending_approvals ?? 0;

  return (
    <div className="relative">
      <button
        type="button"
        aria-label={t("运行详情", "Runtime details")}
        aria-expanded={expanded}
        onClick={() => setExpanded((value) => !value)}
        className="flex items-center gap-2 rounded-md border border-ink/10 bg-white px-2.5 py-1 text-xs text-ink/70 hover:border-forest/30 hover:text-forest"
      >
        <span
          aria-hidden="true"
          className={`h-2 w-2 rounded-full ${
            status === "running" || status === "waiting_approval" || status === "verifying"
              ? "bg-amber-500"
              : run && terminal
                ? run.status === "succeeded" ? "bg-forest" : "bg-red-500"
                : "bg-ink/25"
          }`}
        />
        <span>Runtime: {status}</span>
        {pending > 0 && <span className="rounded-full bg-amber-50 px-1.5 text-amber-800">{pending}</span>}
      </button>

      {expanded && (
        <section
          aria-label={t("共享 Runtime 状态", "Shared runtime status")}
          className="absolute right-0 top-full z-30 mt-2 w-[min(24rem,calc(100vw-1.5rem))] rounded-xl border border-ink/10 bg-white p-4 text-left shadow-xl"
        >
          <div className="mb-3 flex items-start justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold text-ink">{t("共享 Runtime", "Shared runtime")}</h2>
              <p className="text-xs text-ink/50">
                {snapshot?.project?.title ?? t("尚未选择项目", "No project selected")}
              </p>
            </div>
            {snapshot?.run && !terminal && (
              <button
                type="button"
                disabled={canceling}
                onClick={onCancel}
                className="rounded-md border border-red-200 px-2.5 py-1 text-xs text-red-700 disabled:cursor-wait disabled:opacity-50"
              >
                {canceling ? t("取消中…", "Canceling…") : t("取消 Run", "Cancel Run")}
              </button>
            )}
          </div>

          {!snapshot?.run ? (
            <p className="rounded-lg bg-ink/[0.03] p-3 text-sm text-ink/60">
              {t("暂无活跃 Run。启动 Lab 任务后会在这里显示可恢复状态。", "No active Run. Start a Lab task to see recoverable state here.")}
            </p>
          ) : (
            <>
              <dl className="grid grid-cols-[7rem_1fr] gap-x-2 gap-y-1.5 text-xs">
                <dt className="text-ink/45">Run ID</dt>
                <dd className="break-all font-mono text-ink">{snapshot.run.id}</dd>
                <dt className="text-ink/45">{t("状态", "Status")}</dt>
                <dd className="text-ink">{status}</dd>
                <dt className="text-ink/45">{t("事件", "Events")}</dt>
                <dd className="text-ink">{detail?.events.length ?? 0} {t("个事件", "events")}</dd>
                <dt className="text-ink/45">{t("审批", "Approvals")}</dt>
                <dd className={pending > 0 ? "text-amber-800" : "text-ink"}>
                  {pending} {t("个待审批", "pending approvals")}
                </dd>
                {run?.stop_reason && (
                  <>
                    <dt className="text-ink/45">{t("终止原因", "Stop reason")}</dt>
                    <dd className="break-words font-mono text-ink">{run.stop_reason}</dd>
                  </>
                )}
              </dl>
              {run?.error && (
                <p role="alert" className="mt-3 break-words rounded-lg bg-red-50 p-2 text-xs text-red-800">
                  {run.error}
                </p>
              )}
            </>
          )}

          {loading && <p className="mt-3 text-xs text-ink/50">{t("同步 Runtime…", "Syncing runtime…")}</p>}
          {error && <p role="alert" className="mt-3 text-xs text-red-700">{error}</p>}

          {snapshot?.project && detail && detail.artifacts.length > 0 && (
            <div className="mt-4 border-t border-ink/10 pt-3">
              <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink/45">
                {t("产物", "Artifacts")}
              </h3>
              <ul className="space-y-1.5">
                {detail.artifacts.map((artifact) => (
                  <li key={artifact.path} className="flex items-center justify-between gap-3 text-xs">
                    <a
                      href={artifactHref(snapshot.project!.id, artifact)}
                      target="_blank"
                      rel="noopener"
                      className="min-w-0 truncate text-forest underline-offset-2 hover:underline"
                    >
                      {artifact.name}
                    </a>
                    <span className="shrink-0 text-ink/40">{artifact.size} B</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </section>
      )}
    </div>
  );
}
