"use client";

import { useCallback, useEffect, useState } from "react";
import { api, yuan, type Earnings, type Withdrawal } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Protected } from "@/components/Protected";
import { Card, PageHeader, Spinner } from "@/components/ui";
import { WithdrawalForm } from "@/components/WithdrawalForm";
import { WithdrawalHistory } from "@/components/WithdrawalHistory";

export default function EarningsPage() {
  return (
    <Protected>
      <EarningsInner />
    </Protected>
  );
}

function EarningsInner() {
  const { t } = useT();
  const [e, setE] = useState<Earnings | null>(null);
  // pending+approved withdrawals reduce the *effective* withdrawable balance —
  // the server enforces this and returns 409 'insufficient settled balance',
  // but we mirror the math client-side so the user sees the right cap up-front.
  const [openSum, setOpenSum] = useState(0);
  const [refreshKey, setRefreshKey] = useState(0);

  const reload = useCallback(async () => {
    const [earnings, hist] = await Promise.all([
      api.earnings().catch(() => ({
        settled_cents: 0, pending_cents: 0, withdrawable_cents: 0,
        settled_orders: 0, pending_orders: 0,
      }) as Earnings),
      api.listMyWithdrawals(100, 0).catch(() => ({ items: [] as Withdrawal[] })),
    ]);
    setE(earnings);
    setOpenSum(
      hist.items
        .filter((w) => w.status === "pending" || w.status === "approved")
        .reduce((acc, w) => acc + (w.amount_cents || 0), 0),
    );
  }, []);

  useEffect(() => { void reload(); }, [reload]);

  if (!e) return <Spinner label={t("加载中…", "Loading…")} />;

  const available = Math.max(0, e.withdrawable_cents - openSum);

  return (
    <div className="space-y-6">
      <PageHeader kicker={t("结算", "Settlement")} title={t("卖家收益", "Seller earnings")} />

      <div className="grid gap-4 sm:grid-cols-3">
        <Stat
          label={t("可提现(扣除未到账)", "Available (net of in-flight)")}
          value={yuan(available)}
          sub={t(`${e.settled_orders} 笔已结算`, `${e.settled_orders} settled orders`)}
          highlight
        />
        <Stat
          label={t("待结算", "Pending")}
          value={yuan(e.pending_cents)}
          sub={t(`${e.pending_orders} 笔进行中`, `${e.pending_orders} in progress`)}
        />
        <Stat
          label={t("累计已结算", "Total settled")}
          value={yuan(e.settled_cents)}
          sub={t("确认收货后自动分账", "Auto-split after buyer confirms receipt")}
        />
      </div>

      <p className="text-sm text-neutral-500">
        {t(
          "资金由持牌方存管,平台不碰资金。买家确认收货后按 卖家 90% / 平台 10% 自动分账。",
          "Funds are held by a licensed custodian; the platform never touches them. After buyer confirmation, settlement auto-splits 90% seller / 10% platform.",
        )}
      </p>

      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <h2 className="mb-3 text-lg font-semibold">{t("申请提现", "Request a withdrawal")}</h2>
          <WithdrawalForm
            withdrawableCents={available}
            onCreated={() => {
              setRefreshKey((k) => k + 1);
              void reload();
            }}
          />
        </Card>
        <Card>
          <h2 className="mb-3 text-lg font-semibold">{t("提现记录", "Withdrawal history")}</h2>
          <WithdrawalHistory refreshKey={refreshKey} />
        </Card>
      </div>
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
