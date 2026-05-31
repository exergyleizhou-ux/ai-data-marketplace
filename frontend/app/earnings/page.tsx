"use client";

import { useEffect, useState } from "react";
import { api, yuan, type Earnings } from "@/lib/api";
import { Protected } from "@/components/Protected";
import { Card, Spinner } from "@/components/ui";

export default function EarningsPage() {
  return (
    <Protected>
      <EarningsInner />
    </Protected>
  );
}

function EarningsInner() {
  const [e, setE] = useState<Earnings | null>(null);
  useEffect(() => {
    api.earnings().then(setE).catch(() => setE({ settled_cents: 0, pending_cents: 0, withdrawable_cents: 0, settled_orders: 0, pending_orders: 0 }));
  }, []);
  if (!e) return <Spinner />;
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">卖家收益</h1>
      <div className="grid gap-4 sm:grid-cols-3">
        <Stat label="可提现（已结算）" value={yuan(e.withdrawable_cents)} sub={`${e.settled_orders} 笔已结算`} highlight />
        <Stat label="待结算" value={yuan(e.pending_cents)} sub={`${e.pending_orders} 笔进行中`} />
        <Stat label="累计已结算" value={yuan(e.settled_cents)} sub="确认收货后自动分账" />
      </div>
      <p className="text-sm text-neutral-500">
        资金由持牌方存管，平台不碰资金。买家确认收货后按 卖家 90% / 平台 10% 自动分账。
      </p>
    </div>
  );
}

function Stat({ label, value, sub, highlight }: { label: string; value: string; sub: string; highlight?: boolean }) {
  return (
    <Card className={highlight ? "border-green-200 bg-green-50" : ""}>
      <div className="text-sm text-neutral-500">{label}</div>
      <div className="mt-1 text-3xl font-semibold">{value}</div>
      <div className="mt-1 text-xs text-neutral-400">{sub}</div>
    </Card>
  );
}
