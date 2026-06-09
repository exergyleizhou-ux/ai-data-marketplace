"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, type Notification } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Protected } from "@/components/Protected";
import { Alert, Badge, Button, Card, Empty, Spinner } from "@/components/ui";

export default function NotificationsPage() {
  return (
    <Protected>
      <Inner />
    </Protected>
  );
}

const KIND: Record<string, [string, string]> = {
  order_paid: ["订单已支付", "Order paid"],
  order_settled: ["订单已结算", "Order settled"],
  order_disputed: ["订单纠纷", "Order disputed"],
  quality_done: ["质检完成", "Quality done"],
  compute_released: ["计算结果已放行", "Compute released"],
};

function Inner() {
  const { t } = useT();
  const [items, setItems] = useState<Notification[] | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");

  const load = useCallback(async () => {
    setErr("");
    try {
      setItems((await api.listNotifications(100)).items);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, []);
  useEffect(() => { void load(); }, [load]);

  async function markRead(id: string) {
    setBusy(id);
    try { await api.markNotificationRead(id); await load(); }
    catch (e) { setErr((e as Error).message); }
    finally { setBusy(""); }
  }

  async function markAll() {
    setBusy("all");
    try { await api.markAllNotificationsRead(); await load(); }
    catch (e) { setErr((e as Error).message); }
    finally { setBusy(""); }
  }

  if (items === null) return <Spinner />;

  const unreadN = items.filter((n) => !n.is_read).length;

  return (
    <div className="mx-auto max-w-2xl space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">{t("通知", "Notifications")}</h1>
        {unreadN > 0 && (
          <Button variant="secondary" disabled={busy === "all"} onClick={markAll}>
            {t(`${unreadN} 条未读全部标为已读`, `Mark ${unreadN} as read`)}
          </Button>
        )}
      </div>
      {err && <Alert>{err}</Alert>}
      {items.length === 0 ? (
        <Empty>{t("暂无通知", "No notifications yet")}</Empty>
      ) : (
        <div className="space-y-2">
          {items.map((n) => {
            const label = KIND[n.kind] ?? [n.kind, n.kind];
            const href = n.resource_type === "order" && n.resource_id
              ? `/orders/${n.resource_id}`
              : n.resource_type === "dataset" && n.resource_id
                ? `/datasets/${n.resource_id}`
                : null;
            return (
              <Card key={n.id} className={!n.is_read ? "border-l-2 border-l-blue-500" : ""}>
                <div className="flex items-start justify-between">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <Badge>{t(label[0], label[1])}</Badge>
                      {!n.is_read && <span className="h-2 w-2 rounded-full bg-blue-500" />}
                    </div>
                    <p className="mt-1 text-sm font-medium">{n.title}</p>
                    {n.body && <p className="mt-0.5 text-sm text-neutral-500">{n.body}</p>}
                    <div className="mt-1 text-xs text-neutral-400">
                      {n.created_at?.slice(0, 19)}
                    </div>
                  </div>
                  <div className="flex gap-2 shrink-0 ml-3">
                    {href && (
                      <Link href={href} className="text-xs text-blue-600 hover:underline">
                        {t("查看", "View")}
                      </Link>
                    )}
                    {!n.is_read && (
                      <button
                        disabled={busy === n.id}
                        onClick={() => markRead(n.id)}
                        className="text-xs text-neutral-400 hover:text-neutral-700"
                      >
                        {t("标为已读", "Read")}
                      </button>
                    )}
                  </div>
                </div>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}
