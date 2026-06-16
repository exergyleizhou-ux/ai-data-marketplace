"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, yuan, type Dataset, type KYC, type Order, type ComputeAlgorithm, type ComputeJob, type OutboxEntry, type ReconciliationPoint, type AuditLogEntry, type Withdrawal, type Anomaly, type DeletionRequest, type Report } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Protected } from "@/components/Protected";
import { Alert, Badge, Button, Card, Empty, Input, PageHeader, Spinner, Tabs } from "@/components/ui";
import { MiniChart } from "@/components/MiniChart";

type Tab = "review" | "kyc" | "tx" | "compute" | "outbox" | "audit" | "withdraw" | "anomaly" | "deletion" | "moderation";

export default function AdminPage() {
  return (
    <Protected requireOps>
      <AdminInner />
    </Protected>
  );
}

function AdminInner() {
  const { t } = useT();
  const [tab, setTab] = useState<Tab>("review");
  const labels: Record<Tab, string> = {
    review: t("数据集审核", "Dataset review"),
    kyc: t("实名审核", "KYC review"),
    tx: t("交易 / 纠纷", "Transactions / disputes"),
    compute: t("计算作业", "Compute jobs"),
    outbox: t("结算队列", "Settlement outbox"),
    audit: t("审计日志", "Audit logs"),
    withdraw: t("提现审批", "Withdrawals"),
    anomaly: t("异常告警", "Anomalies"),
    deletion: t("注销审批", "Deletions"),
    moderation: t("内容审核", "Moderation"),
  };
  return (
    <div className="space-y-6">
      <PageHeader kicker={t("运营", "Operations")} title={t("运营后台", "Ops Console")} />
      <Tabs
        active={tab}
        onChange={setTab}
        tabs={(["review", "kyc", "tx", "compute", "outbox", "audit", "withdraw", "anomaly", "deletion", "moderation"] as const).map(
          (id) => ({ id, label: labels[id] }),
        )}
      />
      {tab === "review" && <ReviewQueue />}
      {tab === "kyc" && <KYCQueue />}
      {tab === "tx" && <Transactions />}
      {tab === "compute" && <ComputeJobs />}
      {tab === "outbox" && <SettlementOutbox />}
      {tab === "audit" && <AuditLogs />}
      {tab === "withdraw" && <WithdrawalAdmin />}
      {tab === "anomaly" && <AnomalyList />}
      {tab === "deletion" && <DeletionAdmin />}
      {tab === "moderation" && <ContentModerationTab />}
    </div>
  );
}

function ReviewQueue() {
  const { t } = useT();
  const [items, setItems] = useState<Dataset[] | null>(null);
  const [notes, setNotes] = useState<Record<string, string>>({});
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");

  const load = useCallback(async () => {
    setErr("");
    try {
      setItems((await api.adminListDatasets("reviewing")).items);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  async function decide(id: string, approve: boolean) {
    setBusy(id);
    setErr("");
    try {
      await api.adminReviewDataset(id, approve, notes[id] ?? "");
      await load();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy("");
    }
  }

  if (items === null) return <Spinner />;
  return (
    <div className="space-y-3">
      {err && <Alert>{err}</Alert>}
      {items.length === 0 ? (
        <Empty>{t("审核队列为空 🎉（质检通过的数据集会出现在这里）", "Review queue is empty 🎉 (datasets that pass quality checks appear here)")}</Empty>
      ) : (
        items.map((ds) => (
          <Card key={ds.id}>
            <div className="flex items-center justify-between">
              <Link href={`/datasets/${ds.id}`} className="font-medium hover:underline">
                {ds.title}
              </Link>
              <Badge>{ds.status}</Badge>
            </div>
            <div className="mt-1 text-sm text-neutral-500">
              {ds.data_type} · {yuan(ds.final_price_cents ?? ds.suggested_price_cents)} · {t(`${ds.sample_count} 样本`, `${ds.sample_count} samples`)} ·
              {" "}{t("来源签约", "Provenance")} {ds.source_signed_at ? "✓" : "✗"} ·
              {ds.source_declaration?.contains_pii ? t(" 声明含PII", " declared PII") : t(" 声明无PII", " declared no PII")}
            </div>
            <div className="mt-3 flex flex-col gap-2 sm:flex-row">
              <Input
                value={notes[ds.id] ?? ""}
                onChange={(e) => setNotes((n) => ({ ...n, [ds.id]: e.target.value }))}
                placeholder={t("审核备注（可选）", "Review note (optional)")}
              />
              <div className="flex gap-2">
                <Button disabled={busy === ds.id} onClick={() => decide(ds.id, true)}>
                  {t("通过上架", "Approve & list")}
                </Button>
                <Button variant="danger" disabled={busy === ds.id} onClick={() => decide(ds.id, false)}>
                  {t("拒绝", "Reject")}
                </Button>
              </div>
            </div>
          </Card>
        ))
      )}
    </div>
  );
}

function KYCQueue() {
  const { t } = useT();
  const [items, setItems] = useState<KYC[] | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");

  const load = useCallback(async () => {
    setErr("");
    try {
      setItems((await api.adminListKYC()).items);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  async function decide(kycId: string, approve: boolean) {
    setBusy(kycId);
    setErr("");
    try {
      await api.adminReviewKYC(kycId, approve);
      await load();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy("");
    }
  }

  if (items === null) return <Spinner />;
  return (
    <div className="space-y-3">
      {err && <Alert>{err}</Alert>}
      {items.length === 0 ? (
        <Empty>{t("没有待审实名申请", "No pending KYC applications")}</Empty>
      ) : (
        items.map((k) => (
          <Card key={k.id}>
            <div className="flex items-center justify-between">
              <div className="font-medium">
                {k.type === "company"
                  ? t(`企业：${k.company_name || "—"}`, `Company: ${k.company_name || "—"}`)
                  : t(`个人：${k.real_name || "—"}`, `Individual: ${k.real_name || "—"}`)}
              </div>
              <Badge>{k.type}</Badge>
            </div>
            <div className="mt-1 font-mono text-xs text-neutral-500">{t("用户", "User")} {k.user_id?.slice(0, 8)} · {t("提交于", "submitted")} {k.created_at?.slice(0, 19) || "—"}</div>
            {k.material_urls && k.material_urls.length > 0 && (
              <div className="mt-1 text-xs text-neutral-500">{t(`材料：${k.material_urls.length} 份`, `Materials: ${k.material_urls.length}`)}</div>
            )}
            <div className="mt-3 flex gap-2">
              <Button disabled={busy === k.id} onClick={() => decide(k.id, true)}>
                {t("通过实名", "Approve KYC")}
              </Button>
              <Button variant="danger" disabled={busy === k.id} onClick={() => decide(k.id, false)}>
                {t("拒绝", "Reject")}
              </Button>
            </div>
          </Card>
        ))
      )}
    </div>
  );
}

function Transactions() {
  const { t } = useT();
  const [items, setItems] = useState<Order[] | null>(null);
  const [rec, setRec] = useState<{
    total_gmv: number; settled_gmv: number; platform_fees: number;
    total_orders: number; settled_orders: number; pending_orders: number;
    disputed_orders: number; refunded_orders: number; refunded_amount: number;
    failed_settlements: number;
  } | null>(null);
  const [tsDays, setTsDays] = useState(7);
  const [tsPoints, setTsPoints] = useState<ReconciliationPoint[] | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");
  const load = useCallback(async () => {
    setErr("");
    try {
      const [txs, r, ts] = await Promise.all([
        api.adminTransactions(),
        api.adminReconciliation(),
        api.adminReconciliationTimeseries(tsDays),
      ]);
      setItems(txs.items);
      setRec(r);
      setTsPoints(ts.points);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, [tsDays]);
  useEffect(() => {
    void load();
  }, [load]);

  async function resolve(id: string, refund: boolean) {
    setBusy(id); setErr("");
    try {
      await api.adminResolveDispute(id, refund, refund ? "ops resolve: refund" : "ops resolve: release");
      await load();
    } catch (e) {
      setErr((e as Error).message);
    } finally { setBusy(""); }
  }

  if (items === null) return <Spinner />;
  if (items.length === 0) return <Empty>{t("暂无交易", "No transactions yet")}</Empty>;

  const disputes = items.filter((o) => o.status === "disputed");

  return (
    <div className="space-y-4">
      {err && <Alert>{err}</Alert>}
      <div className="grid gap-4 sm:grid-cols-4">
        <Card>
          <div className="text-sm text-neutral-500">{t("总 GMV", "Total GMV")}</div>
          <div className="text-2xl font-semibold">{rec ? yuan(rec.total_gmv) : "—"}</div>
        </Card>
        <Card>
          <div className="text-sm text-neutral-500">{t("已结算 GMV", "Settled GMV")}</div>
          <div className="text-2xl font-semibold">{rec ? yuan(rec.settled_gmv) : "—"}</div>
        </Card>
        <Card>
          <div className="text-sm text-neutral-500">{t("平台佣金", "Platform fees")}</div>
          <div className="text-2xl font-semibold">{rec ? yuan(rec.platform_fees) : "—"}</div>
        </Card>
        <Card>
          <div className="text-sm text-neutral-500">{t("结算失败", "Failed settlements")}</div>
          <div className={`text-2xl font-semibold ${rec && rec.failed_settlements > 0 ? "text-red-600" : ""}`}>
            {rec ? rec.failed_settlements : "—"}
          </div>
        </Card>
      </div>
      <div className="grid gap-4 sm:grid-cols-4">
        <Card>
          <div className="text-sm text-neutral-500">{t("总订单", "Total orders")}</div>
          <div className="text-xl font-semibold">{rec ? rec.total_orders : "—"}</div>
        </Card>
        <Card>
          <div className="text-sm text-neutral-500">{t("待结算", "Pending")}</div>
          <div className="text-xl font-semibold">{rec ? rec.pending_orders : "—"}</div>
        </Card>
        <Card>
          <div className="text-sm text-neutral-500">{t("纠纷中", "Disputed")}</div>
          <div className={`text-xl font-semibold ${rec && rec.disputed_orders > 0 ? "text-amber-600" : ""}`}>
            {rec ? rec.disputed_orders : "—"}
          </div>
        </Card>
        <Card>
          <div className="text-sm text-neutral-500">{t("已退款", "Refunded")}</div>
          <div className="text-xl font-semibold">{rec ? rec.refunded_orders : "—"} {rec ? `(${yuan(rec.refunded_amount)})` : ""}</div>
        </Card>
      </div>

      {/* Timeseries trend charts */}
      {tsPoints && tsPoints.length > 0 && (
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-neutral-600">{t("最近趋势", "Recent trend")}</span>
            {([7, 30, 90] as const).map((d) => (
              <button
                key={d}
                onClick={() => setTsDays(d)}
                className={`rounded px-2 py-0.5 text-xs ${tsDays === d ? "bg-neutral-900 text-white" : "border text-neutral-500"}`}
              >
                {d}d
              </button>
            ))}
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <Card>
              <div className="text-xs text-neutral-500 mb-1">{t("GMV", "GMV")}</div>
              <MiniChart
                points={tsPoints.map((p) => ({ date: p.date, value: p.gmv_cents / 100 }))}
                color="#3b82f6"
                height={60}
                label="GMV trend"
              />
              <div className="mt-1 text-xs text-neutral-400 text-right">
                {t("总", "Total")}: {yuan(tsPoints.reduce((s, p) => s + p.gmv_cents, 0))}
              </div>
            </Card>
            <Card>
              <div className="text-xs text-neutral-500 mb-1">{t("已结算 GMV", "Settled GMV")}</div>
              <MiniChart
                points={tsPoints.map((p) => ({ date: p.date, value: p.settled_gmv_cents / 100 }))}
                color="#22c55e"
                height={60}
                label="Settled GMV trend"
              />
              <div className="mt-1 text-xs text-neutral-400 text-right">
                {t("总", "Total")}: {yuan(tsPoints.reduce((s, p) => s + p.settled_gmv_cents, 0))}
              </div>
            </Card>
            <Card>
              <div className="text-xs text-neutral-500 mb-1">{t("结算失败", "Failed settlements")}</div>
              <MiniChart
                points={tsPoints.map((p) => ({ date: p.date, value: p.failed_settlements }))}
                color="#ef4444"
                height={60}
                label="Failed settlements trend"
              />
              <div className="mt-1 text-xs text-neutral-400 text-right">
                {t("总", "Total")}: {tsPoints.reduce((s, p) => s + p.failed_settlements, 0)}
              </div>
            </Card>
          </div>
        </div>
      )}

      {disputes.length > 0 && (
        <Card>
          <h3 className="mb-2 font-semibold text-amber-700">{t("纠纷待裁决", "Disputes to resolve")}</h3>
          <div className="space-y-2">
            {disputes.map((o) => (
              <div key={o.id} className="flex items-center justify-between rounded-md border border-amber-200 bg-amber-50 p-3">
                <Link href={`/orders/${o.id}`} className="font-mono text-xs hover:underline">
                  #{o.id.slice(0, 8)} · {yuan(o.amount_cents)}
                </Link>
                <div className="flex gap-2">
                  <Button variant="danger" disabled={busy === o.id} onClick={() => resolve(o.id, true)}>
                    {t("退款", "Refund")}
                  </Button>
                  <Button disabled={busy === o.id} onClick={() => resolve(o.id, false)}>
                    {t("放行结算", "Release & settle")}
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </Card>
      )}

      <Card className="overflow-x-auto p-0">
        <table className="w-full text-sm">
          <thead className="border-b border-neutral-200 text-left text-neutral-500">
            <tr>
              <th className="px-4 py-2 font-medium">{t("订单", "Order")}</th>
              <th className="px-4 py-2 font-medium">{t("金额", "Amount")}</th>
              <th className="px-4 py-2 font-medium">{t("佣金", "Fee")}</th>
              <th className="px-4 py-2 font-medium">{t("卖家", "Seller")}</th>
              <th className="px-4 py-2 font-medium">{t("状态", "Status")}</th>
            </tr>
          </thead>
          <tbody>
            {items.map((o) => (
              <tr key={o.id} className="border-b border-neutral-100 last:border-0">
                <td className="px-4 py-2 font-mono text-xs">
                  <Link href={`/orders/${o.id}`} className="hover:underline">
                    #{o.id.slice(0, 8)}
                  </Link>
                </td>
                <td className="px-4 py-2">{yuan(o.amount_cents)}</td>
                <td className="px-4 py-2">{yuan(o.platform_fee_cents)}</td>
                <td className="px-4 py-2">{yuan(o.seller_amount_cents)}</td>
                <td className="px-4 py-2">
                  <Badge>{o.status}</Badge>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
    </div>
  );
}

// --- Compute Jobs (admin: algorithm registry + job output review) ---

function ComputeJobs() {
  const { t } = useT();
  const [sub, setSub] = useState<"algos" | "jobs">("jobs");
  return (
    <div className="space-y-4">
      <div className="flex gap-2">
        <button
          onClick={() => setSub("jobs")}
          className={`rounded-md px-3 py-1 text-sm ${sub === "jobs" ? "bg-neutral-900 text-white" : "border"}`}
        >
          {t("输出审核", "Output review")}
        </button>
        <button
          onClick={() => setSub("algos")}
          className={`rounded-md px-3 py-1 text-sm ${sub === "algos" ? "bg-neutral-900 text-white" : "border"}`}
        >
          {t("算法注册", "Algorithm registry")}
        </button>
      </div>
      {sub === "jobs" && <JobReviewQueue />}
      {sub === "algos" && <AlgorithmRegistry />}
    </div>
  );
}

// ComputeSLOSummary derives an at-a-glance ops view from the recent jobs already
// loaded: throughput, success rate, latency percentiles, and the top failure
// reasons — no extra backend call.
function ComputeSLOSummary({ jobs }: { jobs: ComputeJob[] }) {
  const { t } = useT();
  if (jobs.length === 0) return null;
  const TERMINAL = new Set(["released", "failed", "rejected", "canceled"]);
  const byStatus: Record<string, number> = {};
  for (const j of jobs) byStatus[j.status] = (byStatus[j.status] ?? 0) + 1;
  const terminal = jobs.filter((j) => TERMINAL.has(j.status));
  const released = byStatus["released"] ?? 0;
  const successRate = terminal.length ? Math.round((released / terminal.length) * 100) : null;

  // Parse the first 19 chars as UTC for BOTH timestamps; same real zone → the
  // offset cancels in the difference, so latency is correct without tz handling.
  const parse = (s?: string) => (s ? Date.parse(s.slice(0, 19).replace(" ", "T") + "Z") : NaN);
  const lat = jobs
    .map((j) => (parse(j.finished_at) - parse(j.created_at)) / 1000)
    .filter((x) => isFinite(x) && x >= 0)
    .sort((a, b) => a - b);
  const pctl = (p: number) => (lat.length ? lat[Math.min(lat.length - 1, Math.floor(p * lat.length))] : null);
  const fmtSec = (s: number | null) => (s === null ? "—" : s < 1 ? `${Math.round(s * 1000)}ms` : `${s.toFixed(1)}s`);

  const failReasons: Record<string, number> = {};
  for (const j of jobs) {
    if (j.status === "failed") {
      const r = (j.error || "unknown").slice(0, 48);
      failReasons[r] = (failReasons[r] ?? 0) + 1;
    }
  }
  const topFails = Object.entries(failReasons).sort((a, b) => b[1] - a[1]).slice(0, 4);

  const stats: { label: string; value: string }[] = [
    { label: t("最近作业", "Recent jobs"), value: String(jobs.length) },
    { label: t("成功率(终态)", "Success rate (terminal)"), value: successRate === null ? "—" : `${successRate}%` },
    { label: t("延迟 p50", "Latency p50"), value: fmtSec(pctl(0.5)) },
    { label: t("延迟 p95", "Latency p95"), value: fmtSec(pctl(0.95)) },
  ];

  return (
    <Card>
      <div className="text-sm font-semibold text-neutral-700">{t("计算 SLO 概览", "Compute SLO overview")}</div>
      <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
        {stats.map((s) => (
          <div key={s.label} className="rounded-lg border border-neutral-200 p-3 text-center">
            <div className="text-xs text-neutral-400">{s.label}</div>
            <div className="mt-1 text-lg font-semibold">{s.value}</div>
          </div>
        ))}
      </div>
      <div className="mt-3 flex flex-wrap gap-1.5">
        {Object.entries(byStatus)
          .sort((a, b) => b[1] - a[1])
          .map(([st, n]) => (
            <span key={st} className="rounded-full bg-neutral-100 px-2 py-0.5 text-xs text-neutral-600">
              {st}: {n}
            </span>
          ))}
      </div>
      {topFails.length > 0 && (
        <div className="mt-3">
          <div className="text-xs font-medium text-neutral-500">{t("主要失败原因", "Top failure reasons")}</div>
          <ul className="mt-1 space-y-0.5">
            {topFails.map(([reason, n]) => (
              <li key={reason} className="flex justify-between gap-2 text-xs text-red-600">
                <span className="truncate">{reason}</span>
                <span className="shrink-0 text-neutral-400">×{n}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Card>
  );
}

function JobReviewQueue() {
  const { t } = useT();
  const [items, setItems] = useState<ComputeJob[] | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");

  const load = useCallback(async () => {
    setErr("");
    try {
      setItems((await api.adminListComputeJobs(undefined, 100)).items);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, []);
  useEffect(() => { void load(); }, [load]);

  async function act(id: string, release: boolean, reason?: string) {
    setBusy(id); setErr("");
    try {
      if (release) await api.adminReleaseComputeJob(id);
      else await api.adminRejectComputeJob(id, reason ?? "ops reject");
      await load();
    } catch (e) {
      setErr((e as Error).message);
    } finally { setBusy(""); }
  }

  if (items === null) return <Spinner />;
  return (
    <div className="space-y-3">
      {err && <Alert>{err}</Alert>}
      <ComputeSLOSummary jobs={items} />
      {items.length === 0 ? (
        <Empty>{t("暂无待审计算作业", "No compute jobs pending review")}</Empty>
      ) : (
        items.map((j) => (
          <Card key={j.id}>
            <div className="flex items-center justify-between">
              <div className="font-mono text-xs">#{j.id.slice(0, 8)}</div>
              <Badge>{j.status}</Badge>
            </div>
            <div className="mt-1 text-sm text-neutral-500">
              {t("算法", "Algorithm")}: {j.algorithm_id?.slice(0, 8) ?? "—"} · {t("数据集", "Dataset")}: {j.dataset_id.slice(0, 8)} · {j.output_kind ?? "?"}
              {j.output_bytes !== undefined && ` · ${(j.output_bytes / 1024).toFixed(1)}KB`}
              {j.error && <span className="text-red-500"> · {j.error}</span>}
            </div>
            <div className="mt-1 text-xs text-neutral-400">{j.created_at?.slice(0, 19)}</div>
            {j.status === "output_reviewing" && (
              <div className="mt-3 flex gap-2">
                <Button disabled={busy === j.id} onClick={() => act(j.id, true)}>
                  {t("放行输出", "Release output")}
                </Button>
                <Button variant="danger" disabled={busy === j.id} onClick={() => act(j.id, false)}>
                  {t("拒绝", "Reject")}
                </Button>
              </div>
            )}
          </Card>
        ))
      )}
    </div>
  );
}

function AlgorithmRegistry() {
  const { t } = useT();
  const [items, setItems] = useState<ComputeAlgorithm[] | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");
  const [showForm, setShowForm] = useState(false);

  const load = useCallback(async () => {
    setErr("");
    try { setItems((await api.adminListComputeAlgorithms()).items); }
    catch (e) { setErr((e as Error).message); }
  }, []);
  useEffect(() => { void load(); }, [load]);

  async function review(id: string, status: string, trusted: boolean) {
    setBusy(id); setErr("");
    try { await api.adminReviewAlgorithm(id, status, trusted); await load(); }
    catch (e) { setErr((e as Error).message); }
    finally { setBusy(""); }
  }

  if (items === null) return <Spinner />;
  return (
    <div className="space-y-3">
      {err && <Alert>{err}</Alert>}
      <Button onClick={() => setShowForm(!showForm)}>
        {showForm ? t("收起", "Collapse") : t("注册新算法", "Register algorithm")}
      </Button>
      {showForm && <AlgoRegisterForm onDone={() => { setShowForm(false); void load(); }} />}
      {items.length === 0 ? (
        <Empty>{t("暂无算法", "No algorithms registered")}</Empty>
      ) : (
        items.map((a) => (
          <Card key={a.id}>
            <div className="flex items-center justify-between">
              <div className="font-medium">{a.name}</div>
              <Badge>{a.status}</Badge>
            </div>
            <div className="mt-1 text-sm text-neutral-500">
              v{a.version} · {a.runtime} · {a.output_kind} · {t("可信", "Trusted")}: {a.trusted ? "✓" : "✗"}
            </div>
            {a.status !== "active" && (
              <div className="mt-3 flex gap-2">
                <Button disabled={busy === a.id} onClick={() => review(a.id, "active", true)}>
                  {t("审核通过", "Approve")}
                </Button>
                <Button variant="danger" disabled={busy === a.id} onClick={() => review(a.id, "rejected", false)}>
                  {t("拒绝", "Reject")}
                </Button>
              </div>
            )}
          </Card>
        ))
      )}
    </div>
  );
}

function AlgoRegisterForm({ onDone }: { onDone: () => void }) {
  const { t } = useT();
  const [f, setF] = useState({ name: "", runtime: "docker", image: "", image_digest: "", version: 1, source_ref: "", entrypoint: "", output_kind: "model", params_schema: "{}" });
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit() {
    setErr(""); setBusy(true);
    try {
      let ps: Record<string, unknown> = {};
      try { ps = JSON.parse(f.params_schema); } catch { /* use empty */ }
      await api.adminRegisterAlgorithm({ ...f, params_schema: ps });
      onDone();
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(false); }
  }

  return (
    <Card>
      <div className="grid gap-3 sm:grid-cols-2">
        <Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder={t("名称", "Name")} />
        <Input value={f.runtime} onChange={(e) => setF({ ...f, runtime: e.target.value })} placeholder="Runtime (docker)" />
        <Input value={f.image} onChange={(e) => setF({ ...f, image: e.target.value })} placeholder={t("镜像", "Image")} />
        <Input value={f.image_digest} onChange={(e) => setF({ ...f, image_digest: e.target.value })} placeholder={t("镜像摘要 sha256:...", "Digest sha256:...")} />
        <Input value={f.entrypoint} onChange={(e) => setF({ ...f, entrypoint: e.target.value })} placeholder={t("入口", "Entrypoint")} />
        <Input value={f.output_kind} onChange={(e) => setF({ ...f, output_kind: e.target.value })} placeholder={t("输出类型", "Output kind")} />
        <Input value={f.source_ref} onChange={(e) => setF({ ...f, source_ref: e.target.value })} placeholder={t("源码引用 URL", "Source ref URL")} />
        <Input type="number" value={String(f.version)} onChange={(e) => setF({ ...f, version: Number(e.target.value) || 1 })} placeholder="Version" />
      </div>
      <div className="mt-3">
        <textarea
          className="w-full rounded-md border border-neutral-300 p-2 text-sm"
          rows={3}
          value={f.params_schema}
          onChange={(e) => setF({ ...f, params_schema: e.target.value })}
          placeholder={t('参数 schema JSON', 'Params schema JSON (e.g. {"n_estimators":{"type":"integer","default":100}})')}
        />
      </div>
      {err && <Alert>{err}</Alert>}
      <div className="mt-3">
        <Button disabled={busy || !f.name || !f.image || !f.image_digest} onClick={submit}>
          {t("注册", "Register")}
        </Button>
      </div>
    </Card>
  );
}

// --- Settlement Outbox (admin: monitor failed/pending settlements) ---

function SettlementOutbox() {
  const { t } = useT();
  const [filter, setFilter] = useState("failed");
  const [items, setItems] = useState<OutboxEntry[] | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");

  const load = useCallback(async () => {
    setErr("");
    try {
      const s = filter === "all" ? undefined : filter;
      setItems((await api.adminListSettlementOutbox(s, 100)).items);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, [filter]);
  useEffect(() => { void load(); }, [load]);

  async function retry(orderId: string) {
    setBusy(orderId); setErr("");
    try {
      await api.adminRetrySettlementOutbox(orderId);
      await load();
    } catch (e) {
      setErr((e as Error).message);
    } finally { setBusy(""); }
  }

  if (items === null) return <Spinner />;
  const failedN = items.filter((e) => e.status === "failed").length;
  const pendingN = items.filter((e) => e.status === "pending").length;

  return (
    <div className="space-y-4">
      {err && <Alert>{err}</Alert>}
      <div className="grid gap-4 sm:grid-cols-3">
        <Card>
          <div className="text-sm text-neutral-500">{t("失败", "Failed")}</div>
          <div className="text-2xl font-semibold text-red-600">{failedN}</div>
        </Card>
        <Card>
          <div className="text-sm text-neutral-500">{t("重试中", "Pending retry")}</div>
          <div className="text-2xl font-semibold text-amber-600">{pendingN}</div>
        </Card>
        <Card>
          <div className="text-sm text-neutral-500">{t("总计", "Total shown")}</div>
          <div className="text-2xl font-semibold">{items.length}</div>
        </Card>
      </div>
      <div className="flex gap-2">
        {(["failed", "pending", "all"] as const).map((f) => (
          <button
            key={f}
            onClick={() => setFilter(f)}
            className={`rounded-md px-3 py-1 text-sm ${filter === f ? "bg-neutral-900 text-white" : "border"}`}
          >
            {f === "failed" ? t("失败", "Failed") : f === "pending" ? t("重试中", "Pending") : t("全部", "All")}
          </button>
        ))}
      </div>
      {items.length === 0 ? (
        <Empty>{t("结算队列为空 ✅", "Settlement outbox is empty ✅")}</Empty>
      ) : (
        <Card className="overflow-x-auto p-0">
          <table className="w-full text-sm">
            <thead className="border-b border-neutral-200 text-left text-neutral-500">
              <tr>
                <th className="px-4 py-2 font-medium">{t("订单", "Order")}</th>
                <th className="px-4 py-2 font-medium">{t("状态", "Status")}</th>
                <th className="px-4 py-2 font-medium">{t("重试次数", "Attempts")}</th>
                <th className="px-4 py-2 font-medium">{t("错误", "Error")}</th>
                <th className="px-4 py-2 font-medium">{t("下次重试", "Next attempt")}</th>
                <th className="px-4 py-2 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {items.map((e) => (
                <tr key={e.order_id} className="border-b border-neutral-100 last:border-0">
                  <td className="px-4 py-2 font-mono text-xs">
                    <Link href={`/orders/${e.order_id}`} className="hover:underline">
                      #{e.order_id.slice(0, 8)}
                    </Link>
                  </td>
                  <td className="px-4 py-2"><Badge>{e.status}</Badge></td>
                  <td className="px-4 py-2">{e.attempts}</td>
                  <td className="px-4 py-2 max-w-[200px] truncate text-xs text-red-600" title={e.last_error ?? ""}>
                    {e.last_error ?? "—"}
                  </td>
                  <td className="px-4 py-2 text-xs text-neutral-400">{e.next_attempt_at?.slice(0, 19)}</td>
                  <td className="px-4 py-2">
                    {e.status === "failed" && (
                      <button
                        disabled={busy === e.order_id}
                        onClick={() => retry(e.order_id)}
                        className="rounded-md bg-neutral-900 px-2 py-0.5 text-xs text-white disabled:opacity-50"
                      >
                        {t("手动重试", "Retry")}
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
    </div>
  );
}

function AuditLogs() {
  const { t } = useT();
  const [items, setItems] = useState<AuditLogEntry[]>([]);
  const [filters, setFilters] = useState({
    actor: "", action: "", resource_type: "", resource_id: "",
    from: "", to: "",
  });
  const [offset, setOffset] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [busy, setBusy] = useState(false);
  const [expanded, setExpanded] = useState<Set<number>>(new Set());

  const fetchPage = useCallback(async (nextOffset: number, append: boolean) => {
    setBusy(true);
    try {
      const r = await api.adminListAuditLogs({ ...filters, limit: 50, offset: nextOffset });
      setItems(prev => append ? [...prev, ...r.items] : r.items);
      setHasMore(r.next_offset !== undefined);
      setOffset(nextOffset);
    } finally {
      setBusy(false);
    }
  }, [filters]);

  useEffect(() => {
    void fetchPage(0, false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function applyFilters() { void fetchPage(0, false); }

  function toggleExpand(id: number) {
    setExpanded(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-2 sm:grid-cols-3">
        <Input value={filters.actor} onChange={(e) => setFilters({ ...filters, actor: e.target.value })} placeholder={t("操作者ID", "Actor ID")} />
        <Input value={filters.action} onChange={(e) => setFilters({ ...filters, action: e.target.value })} placeholder={t("动作", "Action")} />
        <Input value={filters.resource_type} onChange={(e) => setFilters({ ...filters, resource_type: e.target.value })} placeholder={t("资源类型", "Resource type")} />
        <Input value={filters.resource_id} onChange={(e) => setFilters({ ...filters, resource_id: e.target.value })} placeholder={t("资源ID", "Resource ID")} />
        <Input value={filters.from} onChange={(e) => setFilters({ ...filters, from: e.target.value })} placeholder={t("起始时间 (RFC3339)", "From (RFC3339)")} />
        <Input value={filters.to} onChange={(e) => setFilters({ ...filters, to: e.target.value })} placeholder={t("结束时间 (RFC3339)", "To (RFC3339)")} />
      </div>
      <Button onClick={applyFilters} disabled={busy}>
        {t("应用过滤", "Apply filters")}
      </Button>

      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead className="border-b border-neutral-200 text-left text-neutral-500">
            <tr>
              <th className="px-2 py-1 font-medium w-[140px]">{t("时间", "Time")}</th>
              <th className="px-2 py-1 font-medium">{t("操作者", "Actor")}</th>
              <th className="px-2 py-1 font-medium">{t("动作", "Action")}</th>
              <th className="px-2 py-1 font-medium">{t("资源", "Resource")}</th>
              <th className="px-2 py-1 font-medium">{t("详情", "Detail")}</th>
            </tr>
          </thead>
          <tbody>
            {items.map((e) => (
              <>
                <tr key={e.id} className="border-b border-neutral-100 last:border-0 hover:bg-neutral-50">
                  <td className="px-2 py-1 text-xs text-neutral-400">{e.created_at?.slice(0, 19)}</td>
                  <td className="px-2 py-1 text-xs max-w-[120px] truncate" title={e.actor_id}>
                    {e.actor_id?.slice(0, 8) || "—"}
                  </td>
                  <td className="px-2 py-1 text-xs font-mono">{e.action}</td>
                  <td className="px-2 py-1 text-xs text-neutral-500">
                    {e.resource_type || "—"}{e.resource_id ? `/${e.resource_id.slice(0, 8)}` : ""}
                  </td>
                  <td className="px-2 py-1 text-xs">
                    {e.detail && Object.keys(e.detail).length > 0 ? (
                      <button onClick={() => toggleExpand(e.id)} className="text-blue-600 hover:underline">
                        {expanded.has(e.id) ? t("收起 JSON", "Hide JSON") : t("查看 JSON", "View JSON")}
                      </button>
                    ) : "—"}
                  </td>
                </tr>
                {expanded.has(e.id) && e.detail && (
                  <tr key={`${e.id}-detail`}>
                    <td colSpan={5} className="px-2 py-2 bg-neutral-50">
                      <pre className="text-[11px] leading-relaxed text-neutral-600 whitespace-pre-wrap break-all">
                        {JSON.stringify(e.detail, null, 2)}
                      </pre>
                    </td>
                  </tr>
                )}
              </>
            ))}
          </tbody>
        </table>
      </div>
      {hasMore && !busy && (
        <div className="text-center">
          <Button variant="secondary" onClick={() => fetchPage(offset + 50, true)}>
            {t("加载更多", "Load more")}
          </Button>
        </div>
      )}
    </div>
  );
}

function WithdrawalAdmin() {
  const { t } = useT();
  const [items, setItems] = useState<Withdrawal[] | null>(null);
  const [filter, setFilter] = useState("pending");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");
  const [notes, setNotes] = useState<Record<string, string>>({});

  const load = useCallback(async () => {
    setErr("");
    try {
      setItems((await api.adminListWithdrawals(filter === "all" ? undefined : filter, 100)).items);
    } catch (e) { setErr((e as Error).message); }
  }, [filter]);
  useEffect(() => { void load(); }, [load]);

  async function act(id: string, action: "approve" | "reject" | "complete") {
    setBusy(id); setErr("");
    try {
      if (action === "approve") await api.adminApproveWithdrawal(id, notes[id] || "");
      else if (action === "reject") await api.adminRejectWithdrawal(id, notes[id] || "");
      else await api.adminCompleteWithdrawal(id, notes[id] || "");
      await load();
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(""); }
  }

  if (items === null) return <Spinner />;
  return (
    <div className="space-y-3">
      {err && <Alert>{err}</Alert>}
      <div className="flex gap-2">
        {(["pending", "approved", "all"] as const).map((f) => (
          <button key={f} onClick={() => setFilter(f)}
            className={`rounded-md px-3 py-1 text-sm ${filter === f ? "bg-neutral-900 text-white" : "border"}`}>
            {f === "pending" ? t("待审批", "Pending") : f === "approved" ? t("已批准", "Approved") : t("全部", "All")}
          </button>
        ))}
      </div>
      {items.length === 0 ? <Empty>{t("暂无提现申请", "No withdrawal requests")}</Empty> : items.map((r) => (
        <Card key={r.id}>
          <div className="flex items-center justify-between">
            <div>
              <div className="font-medium">{yuan(r.amount_cents)}</div>
              <div className="text-xs text-neutral-500">{r.channel} · {r.account_label} · {r.requested_at?.slice(0, 10)}</div>
            </div>
            <Badge>{r.status}</Badge>
          </div>
          <div className="mt-2 flex gap-2">
            <Input value={notes[r.id] || ""} onChange={(e) => setNotes((n) => ({ ...n, [r.id]: e.target.value }))}
              placeholder={t("备注", "Note")} />
            {r.status === "pending" && (
              <>
                <Button disabled={busy === r.id} onClick={() => act(r.id, "approve")}>{t("批准", "Approve")}</Button>
                <Button variant="danger" disabled={busy === r.id} onClick={() => act(r.id, "reject")}>
                  {t("拒绝", "Reject")}
                </Button>
              </>
            )}
            {r.status === "approved" && (
              <Button disabled={busy === r.id} onClick={() => act(r.id, "complete")}>{t("完成打款", "Complete")}</Button>
            )}
          </div>
        </Card>
      ))}
    </div>
  );
}

function AnomalyList() {
  const { t } = useT();
  const [items, setItems] = useState<Anomaly[] | null>(null);
  const [filter, setFilter] = useState("open");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");
  const [notes, setNotes] = useState<Record<string, string>>({});

  const load = useCallback(async () => {
    setErr("");
    try {
      setItems((await api.adminListAnomalies(filter === "all" ? undefined : filter, 100)).items);
    } catch (e) { setErr((e as Error).message); }
  }, [filter]);
  useEffect(() => { void load(); }, [load]);

  async function act(id: string, action: "acknowledge" | "resolve") {
    setBusy(id); setErr("");
    try {
      if (action === "acknowledge") await api.adminAcknowledgeAnomaly(id, notes[id] || "");
      else await api.adminResolveAnomaly(id, notes[id] || "");
      await load();
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(""); }
  }

  const KIND: Record<string, [string, string]> = {
    repeated_failure: [t("频繁失败", "Repeated failure"), "repeated_failure"],
    bulk_modification: [t("批量修改", "Bulk modification"), "bulk_modification"],
    high_risk_action: [t("高风险操作", "High risk action"), "high_risk_action"],
  };

  if (items === null) return <Spinner />;
  return (
    <div className="space-y-3">
      {err && <Alert>{err}</Alert>}
      <div className="flex gap-2">
        {(["open", "acknowledged", "all"] as const).map((f) => (
          <button key={f} onClick={() => setFilter(f)}
            className={`rounded-md px-3 py-1 text-sm ${filter === f ? "bg-neutral-900 text-white" : "border"}`}>
            {f === "open" ? t("待处理", "Open") : f === "acknowledged" ? t("已确认", "Acknowledged") : t("全部", "All")}
          </button>
        ))}
      </div>
      {items.length === 0 ? <Empty>{t("暂无异常", "No anomalies detected")}</Empty> : items.map((a) => (
        <Card key={a.id}>
          <div className="flex items-center justify-between">
            <div>
              <Badge>{KIND[a.kind]?.[0] || a.kind}</Badge>
              <span className="ml-2 text-xs text-neutral-500">{a.resource_pattern} · {t(`${a.count} 次`, `${a.count} times`)}</span>
            </div>
            <Badge>{a.status}</Badge>
          </div>
          <div className="mt-1 text-xs text-neutral-400">
            {a.actor_id?.slice(0, 8) || "—"} · {a.first_seen_at?.slice(0, 19)} ~ {a.last_seen_at?.slice(0, 19)}
          </div>
          {a.status === "open" && (
            <div className="mt-2 flex gap-2">
              <Input value={notes[a.id] || ""} onChange={(e) => setNotes((n) => ({ ...n, [a.id]: e.target.value }))}
                placeholder={t("备注", "Note")} />
              <Button disabled={busy === a.id} onClick={() => act(a.id, "acknowledge")}>{t("确认", "Ack")}</Button>
              <Button variant="secondary" disabled={busy === a.id} onClick={() => act(a.id, "resolve")}>{t("解决", "Resolve")}</Button>
            </div>
          )}
        </Card>
      ))}
    </div>
  );
}

function DeletionAdmin() {
  const { t } = useT();
  const [items, setItems] = useState<DeletionRequest[] | null>(null);
  const [filter, setFilter] = useState("cooling");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");
  const [notes, setNotes] = useState<Record<string, string>>({});

  const load = useCallback(async () => {
    setErr("");
    try {
      setItems((await api.adminListDeletions(filter === "all" ? undefined : filter, 100)).items);
    } catch (e) { setErr((e as Error).message); }
  }, [filter]);
  useEffect(() => { void load(); }, [load]);

  async function act(id: string, action: "approve" | "reject" | "execute") {
    setBusy(id); setErr("");
    try {
      if (action === "approve") await api.adminApproveDeletion(id, notes[id] || "");
      else if (action === "reject") await api.adminRejectDeletion(id, notes[id] || "");
      else await api.adminExecuteDeletion(id, notes[id] || "");
      await load();
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(""); }
  }

  if (items === null) return <Spinner />;
  return (
    <div className="space-y-3">
      {err && <Alert>{err}</Alert>}
      <div className="flex gap-2">
        {(["cooling", "approved", "all"] as const).map((f) => (
          <button key={f} onClick={() => setFilter(f)}
            className={`rounded-md px-3 py-1 text-sm ${filter === f ? "bg-neutral-900 text-white" : "border"}`}>
            {f === "cooling" ? t("冷静期", "Cooling") : f === "approved" ? t("已批准", "Approved") : t("全部", "All")}
          </button>
        ))}
      </div>
      {items.length === 0 ? <Empty>{t("暂无注销申请", "No deletion requests")}</Empty> : items.map((d) => (
        <Card key={d.id}>
          <div className="flex items-center justify-between">
            <div>
              <div className="font-medium">{d.user_id?.slice(0, 8) || "—"}</div>
              <div className="text-xs text-neutral-500">
                {d.reason ? `"${d.reason.slice(0, 50)}" · ` : ""}
                {t("冷静期至", "Cooling until")} {d.cooling_until?.slice(0, 19)}
              </div>
            </div>
            <Badge>{d.status}</Badge>
          </div>
          <div className="mt-2 flex gap-2">
            <Input value={notes[d.id] || ""} onChange={(e) => setNotes((n) => ({ ...n, [d.id]: e.target.value }))}
              placeholder={t("备注", "Note")} />
            {d.status === "cooling" && (
              <>
                <Button disabled={busy === d.id} onClick={() => act(d.id, "approve")}>{t("批准", "Approve")}</Button>
                <Button variant="danger" disabled={busy === d.id} onClick={() => act(d.id, "reject")}>
                  {t("拒绝", "Reject")}
                </Button>
              </>
            )}
            {d.status === "approved" && (
              <Button variant="danger" disabled={busy === d.id} onClick={() => act(d.id, "execute")}>
                {t("执行删除", "Execute deletion")}
              </Button>
            )}
          </div>
        </Card>
      ))}
    </div>
  );
}

function ContentModerationTab() {
  const { t } = useT();
  const [items, setItems] = useState<Report[] | null>(null);
  const [filter, setFilter] = useState("open");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");

  const load = useCallback(async () => {
    setErr("");
    try {
      setItems((await api.adminListReports(filter === "all" ? undefined : filter, 100)).items);
    } catch (e) { setErr((e as Error).message); }
  }, [filter]);
  useEffect(() => { void load(); }, [load]);

  async function act(id: string, action: "hide" | "dismiss") {
    setBusy(id); setErr("");
    try {
      await api.adminResolveReport(id, action);
      await load();
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(""); }
  }

  if (items === null) return <Spinner label={t("加载中…", "Loading…")} />;
  return (
    <div className="space-y-3">
      {err && <Alert>{err}</Alert>}
      <div className="flex gap-2">
        {(["open", "resolved", "all"] as const).map((f) => (
          <button key={f} onClick={() => setFilter(f)}
            className={`rounded-md px-3 py-1 text-sm ${filter === f ? "bg-neutral-900 text-white" : "border"}`}>
            {f === "open" ? t("待处理", "Open") : f === "resolved" ? t("已处理", "Resolved") : t("全部", "All")}
          </button>
        ))}
      </div>
      {items.length === 0 ? <Empty>{t("暂无举报", "No reports")}</Empty> : items.map((r) => (
        <Card key={r.id}>
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <Badge>{r.target_type === "question" ? t("问题", "Question") : t("评论", "Review")}</Badge>
              <span className="ml-2 text-xs text-neutral-400">{r.created_at?.slice(0, 10)}</span>
              <div className="mt-1 break-words text-sm">{r.reason}</div>
              <div className="mt-1 text-xs text-neutral-400">
                {t("目标", "Target")}: {r.target_id}
                {r.resolution && <> · {r.resolution === "hide" ? t("已隐藏", "Hidden") : t("已忽略", "Dismissed")}</>}
              </div>
            </div>
            {r.status === "open" && (
              <div className="flex shrink-0 gap-2">
                <Button variant="danger" disabled={busy === r.id} onClick={() => act(r.id, "hide")}>
                  {t("隐藏内容", "Hide")}
                </Button>
                <Button variant="secondary" disabled={busy === r.id} onClick={() => act(r.id, "dismiss")}>
                  {t("忽略举报", "Dismiss")}
                </Button>
              </div>
            )}
          </div>
        </Card>
      ))}
    </div>
  );
}
