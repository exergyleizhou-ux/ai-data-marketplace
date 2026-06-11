"use client";

import { useEffect, useState } from "react";
import { api, yuan, type Withdrawal } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Alert, Badge, Empty, Spinner } from "@/components/ui";

// Backend caps page size to ≤100 (repo.go); we request 50 and keep it simple:
// no pagination yet — sellers rarely have hundreds of withdrawals open. If they
// do, the admin console has the full ops view.
export function WithdrawalHistory({ refreshKey }: { refreshKey: number }) {
  const { t } = useT();
  const [items, setItems] = useState<Withdrawal[] | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    setItems(null); setErr("");
    api.listMyWithdrawals(50, 0)
      .then((r) => setItems(r.items))
      .catch((e) => setErr((e as Error).message));
  }, [refreshKey]);

  function channelDisplay(c: string): string {
    if (c === "alipay") return t("支付宝", "Alipay");
    if (c === "wechat") return t("微信", "WeChat");
    if (c === "bank") return t("银行卡", "Bank card");
    return c;
  }

  if (err) return <Alert>{err}</Alert>;
  if (items === null) return <Spinner label={t("加载中…", "Loading…")} />;
  if (items.length === 0) return <Empty>{t("暂无提现记录", "No withdrawals yet")}</Empty>;

  return (
    <ul className="divide-y divide-neutral-200">
      {items.map((w) => (
        <li key={w.id} className="flex items-start justify-between gap-4 py-3">
          <div>
            <div className="text-lg font-semibold">{yuan(w.amount_cents)}</div>
            <div className="mt-1 text-xs text-neutral-500">
              {channelDisplay(w.channel)} · {w.account_label} · {w.requested_at?.slice(0, 10)}
            </div>
            {w.ops_note && (
              <div className="mt-1 text-xs text-neutral-400">
                {t("运营备注", "Note")}:{w.ops_note}
              </div>
            )}
          </div>
          <Badge>{w.status}</Badge>
        </li>
      ))}
    </ul>
  );
}
