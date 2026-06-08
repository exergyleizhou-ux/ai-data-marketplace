"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, yuan, type Dataset, type KYC, type Order, type ComputeAlgorithm, type ComputeJob, type OutboxEntry } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Protected } from "@/components/Protected";
import { Alert, Badge, Button, Card, Empty, Input, Spinner } from "@/components/ui";

type Tab = "review" | "kyc" | "tx" | "compute" | "outbox";

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
  };
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">{t("运营后台", "Ops Console")}</h1>
      <div className="flex flex-wrap gap-2">
        {(["review", "kyc", "tx", "compute", "outbox"] as const).map((tb) => (
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
      {tab === "review" && <ReviewQueue />}
      {tab === "kyc" && <KYCQueue />}
      {tab === "tx" && <Transactions />}
      {tab === "compute" && <ComputeJobs />}
      {tab === "outbox" && <SettlementOutbox />}
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
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");
  const load = useCallback(async () => {
    setErr("");
    try {
      const [txs, r] = await Promise.all([
        api.adminTransactions(),
        api.adminReconciliation(),
      ]);
      setItems(txs.items);
      setRec(r);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, []);
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
