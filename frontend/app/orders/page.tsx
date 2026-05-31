"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, yuan, type Order } from "@/lib/api";
import { Protected } from "@/components/Protected";
import { Badge, Card, Empty, Spinner } from "@/components/ui";

export default function OrdersPage() {
  return (
    <Protected>
      <OrdersInner />
    </Protected>
  );
}

function OrdersInner() {
  const [tab, setTab] = useState<"buyer" | "seller">("buyer");
  const [items, setItems] = useState<Order[] | null>(null);

  const load = useCallback(async () => {
    setItems(null);
    const res = await api.listOrders(tab === "seller" ? "seller" : undefined);
    setItems(res.items);
  }, [tab]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">订单</h1>
      <div className="flex gap-2">
        {(["buyer", "seller"] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`rounded-md px-4 py-1.5 text-sm ${
              tab === t ? "bg-neutral-900 text-white" : "border border-neutral-300 bg-white text-neutral-700"
            }`}
          >
            {t === "buyer" ? "我买的" : "我卖的"}
          </button>
        ))}
      </div>

      {items === null ? (
        <Spinner />
      ) : items.length === 0 ? (
        <Empty>暂无订单</Empty>
      ) : (
        <div className="space-y-3">
          {items.map((o) => (
            <Link key={o.id} href={`/orders/${o.id}`}>
              <Card className="flex items-center justify-between transition hover:shadow-md">
                <div>
                  <div className="font-mono text-xs text-neutral-400">#{o.id.slice(0, 8)}</div>
                  <div className="mt-1 text-sm text-neutral-600">{o.license_type} · {yuan(o.amount_cents)}</div>
                </div>
                <Badge>{o.status}</Badge>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
