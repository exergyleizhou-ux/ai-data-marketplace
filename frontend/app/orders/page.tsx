"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, yuan, type Order } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Protected } from "@/components/Protected";
import { Alert, Badge, Button, Card, Empty, Spinner } from "@/components/ui";

export default function OrdersPage() {
  return (
    <Protected>
      <OrdersInner />
    </Protected>
  );
}

function OrdersInner() {
  const { t } = useT();
  const [tab, setTab] = useState<"buyer" | "seller">("buyer");
  const [items, setItems] = useState<Order[] | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [bundleErr, setBundleErr] = useState("");
  const [bundling, setBundling] = useState(false);

  const load = useCallback(async () => {
    setItems(null);
    setSelected(new Set());
    const res = await api.listOrders(tab === "seller" ? "seller" : undefined);
    setItems(res.items);
  }, [tab]);

  useEffect(() => {
    void load();
  }, [load]);

  const settledDownloads = (items ?? []).filter(
    (o) => o.status === "settled" && o.product_type !== "compute",
  );

  function toggle(id: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }

  async function bundleDownload() {
    if (selected.size === 0) return;
    setBundleErr("");
    setBundling(true);
    try {
      await api.bundleDownload([...selected]);
    } catch (e) {
      setBundleErr((e as Error).message);
    } finally {
      setBundling(false);
    }
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">{t("订单", "Orders")}</h1>
      <div className="flex gap-2">
        {(["buyer", "seller"] as const).map((tb) => (
          <button
            key={tb}
            onClick={() => setTab(tb)}
            className={`rounded-md px-4 py-1.5 text-sm ${
              tab === tb ? "bg-neutral-900 text-white" : "border border-neutral-300 bg-white text-neutral-700"
            }`}
          >
            {tb === "buyer" ? t("我买的", "Bought") : t("我卖的", "Sold")}
          </button>
        ))}
      </div>

      {items === null ? (
        <Spinner />
      ) : items.length === 0 ? (
        <Empty>{t("暂无订单", "No orders yet")}</Empty>
      ) : (
        <div className="space-y-3">
          {tab === "buyer" && settledDownloads.length > 0 && (
            <div className="flex items-center gap-3">
              {selected.size > 0 && (
                <Button onClick={bundleDownload} disabled={bundling}>
                  {bundling ? t("打包中…", "Packaging…") : t(`打包下载 (${selected.size})`, `Bundle download (${selected.size})`)}
                </Button>
              )}
              <span className="text-xs text-neutral-400">
                {t(
                  "勾选已结算订单后可打包下载为 zip 文件（最多 20 个）",
                  "Select settled orders to bundle into a zip (max 20)",
                )}
              </span>
            </div>
          )}
          {bundleErr && <Alert>{bundleErr}</Alert>}
          {items.map((o) => {
            const canSelect = tab === "buyer" && o.status === "settled" && o.product_type !== "compute";
            return (
              <div key={o.id} className="flex items-center gap-3">
                {canSelect && (
                  <input
                    type="checkbox"
                    checked={selected.has(o.id)}
                    onChange={() => toggle(o.id)}
                    className="h-4 w-4"
                    disabled={!selected.has(o.id) && selected.size >= 20}
                  />
                )}
                <Link href={`/orders/${o.id}`} className="flex-1">
                  <Card className="flex items-center justify-between transition hover:shadow-md">
                    <div>
                      <div className="font-mono text-xs text-neutral-400">#{o.id.slice(0, 8)}</div>
                      <div className="mt-1 text-sm text-neutral-600">{o.license_type} · {yuan(o.amount_cents)}</div>
                    </div>
                    <Badge>{o.status}</Badge>
                  </Card>
                </Link>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
