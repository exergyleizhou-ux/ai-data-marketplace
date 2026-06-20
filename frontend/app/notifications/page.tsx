"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, type Notification } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Protected } from "@/components/Protected";
import { Alert, Badge, Button, Card, Empty } from "@/components/ui";
import { Reveal } from "@/components/Reveal";

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
  dataset_updated: ["数据集有更新", "Dataset updated"],
  question_asked: ["数据集有新提问", "New question on your dataset"],
  question_answered: ["您的提问已被回答", "Your question was answered"],
  withdrawal_approved: ["提现已批准", "Withdrawal approved"],
  withdrawal_completed: ["提现已到账", "Withdrawal completed"],
  withdrawal_rejected: ["提现被拒", "Withdrawal rejected"],
  data_export_ready: ["数据导出已就绪", "Data export ready"],
  account_deletion_cooling: ["账号注销冷静期已开始", "Account deletion cooling period started"],
  account_deletion_approved: ["账号注销已批准", "Account deletion approved"],
  account_deletion_rejected: ["账号注销被拒", "Account deletion rejected"],
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

  if (items === null)
    return (
      <div className="space-y-2 pt-2" aria-hidden>
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="skeleton h-20 w-full rounded-2xl" />
        ))}
      </div>
    );

  const unreadN = items.filter((n) => !n.is_read).length;

  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between pt-2">
        <div>
          <p className="font-mono text-kicker uppercase text-muted">{t("消息", "Inbox")}</p>
          <h1 className="mt-3 font-display text-display-sm leading-tight tracking-tight">{t("通知", "Notifications")}</h1>
        </div>
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
          {items.map((n, i) => {
            const label = KIND[n.kind] ?? [n.kind, n.kind];
            const href = n.resource_type === "order" && n.resource_id
              ? `/orders/${n.resource_id}`
              : n.resource_type === "dataset" && n.resource_id
                ? `/datasets/${n.resource_id}`
                : null;
            return (
              <Reveal key={n.id} delay={Math.min(i, 8) * 40}>
                <Card className={`lift ${!n.is_read ? "border-l-2 border-l-forest-600" : ""}`}>
                  <div className="flex items-start justify-between">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <Badge>{t(label[0], label[1])}</Badge>
                        {!n.is_read && <span className="h-2 w-2 rounded-full bg-forest" />}
                      </div>
                      <p className="mt-1 text-sm font-medium text-ink">{n.title}</p>
                      {n.body && <p className="mt-0.5 text-sm text-ink/60">{n.body}</p>}
                      <div className="mt-1 font-mono text-xs text-muted">{n.created_at?.slice(0, 19)}</div>
                    </div>
                    <div className="ml-3 flex shrink-0 gap-2">
                      {href && (
                        <Link href={href} className="rounded text-xs text-forest-700 hover:underline focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ink">
                          {t("查看", "View")}
                        </Link>
                      )}
                      {!n.is_read && (
                        <button
                          disabled={busy === n.id}
                          onClick={() => markRead(n.id)}
                          className="rounded text-xs text-muted transition hover:text-ink focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ink"
                        >
                          {t("标为已读", "Read")}
                        </button>
                      )}
                    </div>
                  </div>
                </Card>
              </Reveal>
            );
          })}
        </div>
      )}
    </div>
  );
}
