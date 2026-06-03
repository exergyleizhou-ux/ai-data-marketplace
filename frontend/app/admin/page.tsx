"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, yuan, type Dataset, type KYC, type Order } from "@/lib/api";
import { useT } from "@/lib/i18n";
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
  const { t } = useT();
  const [tab, setTab] = useState<Tab>("review");
  const labels: Record<Tab, string> = {
    review: t("数据集审核", "Dataset review"),
    kyc: t("实名审核", "KYC review"),
    tx: t("交易 / 纠纷", "Transactions / disputes"),
  };
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">{t("运营后台", "Ops Console")}</h1>
      <div className="flex gap-2">
        {(["review", "kyc", "tx"] as const).map((tb) => (
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
      await api.adminResolveDispute(id, refund, refund ? "ops resolve: refund" : "ops resolve: release");
      await load();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy("");
    }
  }

  if (items === null) return <Spinner />;
  if (items.length === 0) return <Empty>{t("暂无交易", "No transactions yet")}</Empty>;

  const totalGmv = items.reduce((s, o) => s + (o.status === "settled" ? o.amount_cents : 0), 0);
  const totalFee = items.reduce((s, o) => s + (o.status === "settled" ? o.platform_fee_cents : 0), 0);
  const disputes = items.filter((o) => o.status === "disputed");

  return (
    <div className="space-y-4">
      {err && <Alert>{err}</Alert>}
      <div className="grid gap-4 sm:grid-cols-3">
        <Card>
          <div className="text-sm text-neutral-500">{t("已结算 GMV", "Settled GMV")}</div>
          <div className="text-2xl font-semibold">{yuan(totalGmv)}</div>
        </Card>
        <Card>
          <div className="text-sm text-neutral-500">{t("平台累计佣金", "Platform fees")}</div>
          <div className="text-2xl font-semibold">{yuan(totalFee)}</div>
        </Card>
        <Card>
          <div className="text-sm text-neutral-500">{t("待裁决纠纷", "Open disputes")}</div>
          <div className="text-2xl font-semibold">{disputes.length}</div>
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
