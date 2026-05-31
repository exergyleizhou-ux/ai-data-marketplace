"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, yuan, type Dataset, type Order } from "@/lib/api";
import { Protected } from "@/components/Protected";
import { Alert, Badge, Button, Card, Empty, Input, Spinner } from "@/components/ui";

export default function AdminPage() {
  return (
    <Protected requireOps>
      <AdminInner />
    </Protected>
  );
}

function AdminInner() {
  const [tab, setTab] = useState<"review" | "tx">("review");
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">运营后台</h1>
      <div className="flex gap-2">
        {(["review", "tx"] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`rounded-md px-4 py-1.5 text-sm ${
              tab === t ? "bg-neutral-900 text-white" : "border border-neutral-300 bg-white text-neutral-700"
            }`}
          >
            {t === "review" ? "审核队列" : "交易流水"}
          </button>
        ))}
      </div>
      {tab === "review" ? <ReviewQueue /> : <Transactions />}
    </div>
  );
}

function ReviewQueue() {
  // The catalog API only lists published datasets, so the ops review queue is
  // driven by reviewing-state datasets fetched per-id is not exposed; instead we
  // approve/reject by dataset id pasted from the seller flow. For the demo we
  // list nothing and provide an id box. (A dedicated /admin/datasets?status=
  // endpoint is a natural follow-up.)
  const [id, setId] = useState("");
  const [ds, setDs] = useState<Dataset | null>(null);
  const [note, setNote] = useState("");
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");

  async function lookup() {
    setErr("");
    setMsg("");
    setDs(null);
    try {
      setDs(await api.getDataset(id.trim()));
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  async function decide(approve: boolean) {
    setErr("");
    try {
      const d = await api.adminReviewDataset(id.trim(), approve, note);
      setDs(d);
      setMsg(approve ? "已通过并上架" : "已拒绝");
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  return (
    <Card>
      <h2 className="mb-2 font-semibold">数据集审核</h2>
      <p className="mb-3 text-sm text-neutral-500">输入处于 reviewing 状态的数据集 ID 进行审核。</p>
      <div className="flex gap-2">
        <Input value={id} onChange={(e) => setId(e.target.value)} placeholder="dataset id" />
        <Button variant="secondary" onClick={lookup}>
          查询
        </Button>
      </div>
      {err && <div className="mt-3"><Alert>{err}</Alert></div>}
      {msg && <div className="mt-3"><Alert kind="success">{msg}</Alert></div>}
      {ds && (
        <div className="mt-4 rounded-lg border border-neutral-200 p-4">
          <div className="flex items-center justify-between">
            <Link href={`/datasets/${ds.id}`} className="font-medium hover:underline">
              {ds.title}
            </Link>
            <Badge>{ds.status}</Badge>
          </div>
          <div className="mt-1 text-sm text-neutral-500">
            {ds.data_type} · {yuan(ds.final_price_cents ?? ds.suggested_price_cents)} · {ds.sample_count} 样本 ·
            来源签约 {ds.source_signed_at ? "✓" : "✗"}
          </div>
          {ds.status === "reviewing" && (
            <div className="mt-3 space-y-2">
              <Input value={note} onChange={(e) => setNote(e.target.value)} placeholder="审核备注（可选）" />
              <div className="flex gap-2">
                <Button onClick={() => decide(true)}>通过上架</Button>
                <Button variant="danger" onClick={() => decide(false)}>
                  拒绝
                </Button>
              </div>
            </div>
          )}
        </div>
      )}
    </Card>
  );
}

function Transactions() {
  const [items, setItems] = useState<Order[] | null>(null);
  const load = useCallback(async () => {
    setItems((await api.adminTransactions()).items);
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  if (items === null) return <Spinner />;
  if (items.length === 0) return <Empty>暂无交易</Empty>;

  const totalGmv = items.reduce((s, o) => s + (o.status === "settled" ? o.amount_cents : 0), 0);
  const totalFee = items.reduce((s, o) => s + (o.status === "settled" ? o.platform_fee_cents : 0), 0);

  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Card>
          <div className="text-sm text-neutral-500">已结算 GMV</div>
          <div className="text-2xl font-semibold">{yuan(totalGmv)}</div>
        </Card>
        <Card>
          <div className="text-sm text-neutral-500">平台累计佣金</div>
          <div className="text-2xl font-semibold">{yuan(totalFee)}</div>
        </Card>
      </div>
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
