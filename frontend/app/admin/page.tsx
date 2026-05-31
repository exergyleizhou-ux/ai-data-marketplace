"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, yuan, type Dataset, type KYC, type Order } from "@/lib/api";
import { Protected } from "@/components/Protected";
import { Alert, Badge, Button, Card, Empty, Input, Spinner } from "@/components/ui";

type Tab = "review" | "kyc" | "tx";

export default function AdminPage() {
  return (
    <Protected requireOps>
      <AdminInner />
    </Protected>
  );
}

function AdminInner() {
  const [tab, setTab] = useState<Tab>("review");
  const labels: Record<Tab, string> = { review: "数据集审核", kyc: "实名审核", tx: "交易 / 纠纷" };
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">运营后台</h1>
      <div className="flex gap-2">
        {(["review", "kyc", "tx"] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`rounded-md px-4 py-1.5 text-sm ${
              tab === t ? "bg-neutral-900 text-white" : "border border-neutral-300 bg-white text-neutral-700"
            }`}
          >
            {labels[t]}
          </button>
        ))}
      </div>
      {tab === "review" && <ReviewQueue />}
      {tab === "kyc" && <KYCQueue />}
      {tab === "tx" && <Transactions />}
    </div>
  );
}

function ReviewQueue() {
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
        <Empty>审核队列为空 🎉（质检通过的数据集会出现在这里）</Empty>
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
              {ds.data_type} · {yuan(ds.final_price_cents ?? ds.suggested_price_cents)} · {ds.sample_count} 样本 ·
              来源签约 {ds.source_signed_at ? "✓" : "✗"} ·
              {ds.source_declaration?.contains_pii ? " 声明含PII" : " 声明无PII"}
            </div>
            <div className="mt-3 flex flex-col gap-2 sm:flex-row">
              <Input
                value={notes[ds.id] ?? ""}
                onChange={(e) => setNotes((n) => ({ ...n, [ds.id]: e.target.value }))}
                placeholder="审核备注（可选）"
              />
              <div className="flex gap-2">
                <Button disabled={busy === ds.id} onClick={() => decide(ds.id, true)}>
                  通过上架
                </Button>
                <Button variant="danger" disabled={busy === ds.id} onClick={() => decide(ds.id, false)}>
                  拒绝
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
        <Empty>没有待审实名申请</Empty>
      ) : (
        items.map((k) => (
          <Card key={k.id}>
            <div className="flex items-center justify-between">
              <div className="font-medium">
                {k.type === "company" ? `企业：${k.company_name || "—"}` : `个人：${k.real_name || "—"}`}
              </div>
              <Badge>{k.type}</Badge>
            </div>
            <div className="mt-1 font-mono text-xs text-neutral-500">用户 {k.user_id?.slice(0, 8)} · 提交于 {k.created_at?.slice(0, 19) || "—"}</div>
            {k.material_urls && k.material_urls.length > 0 && (
              <div className="mt-1 text-xs text-neutral-500">材料：{k.material_urls.length} 份</div>
            )}
            <div className="mt-3 flex gap-2">
              <Button disabled={busy === k.id} onClick={() => decide(k.id, true)}>
                通过实名
              </Button>
              <Button variant="danger" disabled={busy === k.id} onClick={() => decide(k.id, false)}>
                拒绝
              </Button>
            </div>
          </Card>
        ))
      )}
    </div>
  );
}

function Transactions() {
  const [items, setItems] = useState<Order[] | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");
  const load = useCallback(async () => {
    setErr("");
    try {
      setItems((await api.adminTransactions()).items);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  async function resolve(id: string, refund: boolean) {
    setBusy(id);
    setErr("");
    try {
      await api.adminResolveDispute(id, refund, refund ? "ops 裁决退款" : "ops 裁决放行");
      await load();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy("");
    }
  }

  if (items === null) return <Spinner />;
  if (items.length === 0) return <Empty>暂无交易</Empty>;

  const totalGmv = items.reduce((s, o) => s + (o.status === "settled" ? o.amount_cents : 0), 0);
  const totalFee = items.reduce((s, o) => s + (o.status === "settled" ? o.platform_fee_cents : 0), 0);
  const disputes = items.filter((o) => o.status === "disputed");

  return (
    <div className="space-y-4">
      {err && <Alert>{err}</Alert>}
      <div className="grid gap-4 sm:grid-cols-3">
        <Card>
          <div className="text-sm text-neutral-500">已结算 GMV</div>
          <div className="text-2xl font-semibold">{yuan(totalGmv)}</div>
        </Card>
        <Card>
          <div className="text-sm text-neutral-500">平台累计佣金</div>
          <div className="text-2xl font-semibold">{yuan(totalFee)}</div>
        </Card>
        <Card>
          <div className="text-sm text-neutral-500">待裁决纠纷</div>
          <div className="text-2xl font-semibold">{disputes.length}</div>
        </Card>
      </div>

      {disputes.length > 0 && (
        <Card>
          <h3 className="mb-2 font-semibold text-amber-700">纠纷待裁决</h3>
          <div className="space-y-2">
            {disputes.map((o) => (
              <div key={o.id} className="flex items-center justify-between rounded-md border border-amber-200 bg-amber-50 p-3">
                <Link href={`/orders/${o.id}`} className="font-mono text-xs hover:underline">
                  #{o.id.slice(0, 8)} · {yuan(o.amount_cents)}
                </Link>
                <div className="flex gap-2">
                  <Button variant="danger" disabled={busy === o.id} onClick={() => resolve(o.id, true)}>
                    退款
                  </Button>
                  <Button disabled={busy === o.id} onClick={() => resolve(o.id, false)}>
                    放行结算
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
              <th className="px-4 py-2 font-medium">订单</th>
              <th className="px-4 py-2 font-medium">金额</th>
              <th className="px-4 py-2 font-medium">佣金</th>
              <th className="px-4 py-2 font-medium">卖家</th>
              <th className="px-4 py-2 font-medium">状态</th>
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
