"use client";

import { use, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { API_ORIGIN, api, yuan, type Order } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useT } from "@/lib/i18n";
import { Protected } from "@/components/Protected";
import { Alert, Badge, Button, Card, Field, Input, Spinner, Textarea } from "@/components/ui";
import { StripeCheckout, stripeConfigured } from "@/components/StripeCheckout";

type PayInfo = { pay_url: string; channel_txn_id: string; amount_cents: number; channel: string };

export default function OrderDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  return (
    <Protected>
      <OrderInner id={id} />
    </Protected>
  );
}

function OrderInner({ id }: { id: string }) {
  const { user } = useAuth();
  const { t } = useT();
  const [o, setO] = useState<Order | null>(null);
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState("");
  const [pay, setPay] = useState<PayInfo | null>(null);
  const [downloadUrl, setDownloadUrl] = useState("");

  const load = useCallback(async () => {
    try {
      setO(await api.getOrder(id));
    } catch (e) {
      setErr((e as Error).message);
    }
  }, [id]);

  // After a real charge, the webhook (not the client) flips the order to "paid".
  // Poll getOrder until the status leaves "created", then surface the result.
  const waitForPaid = useCallback(async () => {
    for (let i = 0; i < 20; i++) {
      const fresh = await api.getOrder(id);
      if (fresh.status !== "created") {
        setO(fresh);
        setPay(null);
        setMsg(t("支付成功，资金已冻结在持牌方。", "Payment successful; funds are held in escrow by the licensed custodian."));
        return;
      }
      await new Promise((r) => setTimeout(r, 1500));
    }
    // Charge captured at Stripe but the webhook hasn't landed yet; let the user retry the read.
    await load();
    setMsg(t("支付已提交，正在等待结算回调确认，请稍后刷新。", "Payment submitted; awaiting the settlement callback — please refresh shortly."));
  }, [id, load, t]);

  useEffect(() => {
    void load();
  }, [load]);

  const act = async (label: string, fn: () => Promise<void>) => {
    setErr("");
    setMsg("");
    setBusy(label);
    try {
      await fn();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy("");
    }
  };

  if (err && !o) return <Alert>{err}</Alert>;
  if (!o) return <Spinner />;

  const isBuyer = user?.id === o.buyer_id;

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">{t("订单详情", "Order details")}</h1>
        <Badge>{o.status}</Badge>
      </div>

      <Card>
        <dl className="grid grid-cols-2 gap-3 text-sm">
          <Row label={t("订单号", "Order #")} value={`#${o.id.slice(0, 8)}`} />
          <Row label={t("数据集", "Dataset")} value={<Link href={`/datasets/${o.dataset_id}`} className="text-neutral-900 underline">{t("查看", "View")}</Link>} />
          <Row label={t("金额", "Amount")} value={yuan(o.amount_cents)} />
          <Row label={t("平台佣金", "Platform fee")} value={yuan(o.platform_fee_cents)} />
          <Row label={t("卖家所得", "Seller receives")} value={yuan(o.seller_amount_cents)} />
          <Row label={t("许可", "License")} value={o.license_type} />
        </dl>
      </Card>

      {msg && <Alert kind="success">{msg}</Alert>}
      {err && <Alert>{err}</Alert>}

      {isBuyer && (
        <Card>
          <h2 className="mb-3 font-semibold">{t("操作", "Actions")}</h2>
          <div className="space-y-3">
            {o.status === "created" && (
              <div className="space-y-2">
                {!pay ? (
                  <Button
                    className="w-full"
                    disabled={!!busy}
                    onClick={() =>
                      act("pay", async () => {
                        setPay(await api.createPayment(o.id));
                      })
                    }
                  >
                    {busy === "pay" ? t("创建支付…", "Creating payment…") : t("去支付", "Pay")}
                  </Button>
                ) : pay.channel === "stripe" && stripeConfigured() ? (
                  // Real Stripe.js card form bound to the PaymentIntent client secret.
                  <StripeCheckout
                    clientSecret={pay.pay_url}
                    amountLabel={yuan(o.amount_cents)}
                    onPaid={waitForPaid}
                  />
                ) : (
                  // No real gateway configured (mock channel / missing pk): sandbox shortcut.
                  <>
                    <Alert kind="info">{t(`沙箱支付单已创建（${pay.channel_txn_id}）。真实环境会跳转至收银台。`, `Sandbox payment created (${pay.channel_txn_id}). In production you'd be redirected to the checkout.`)}</Alert>
                    <Button
                      className="w-full"
                      disabled={!!busy}
                      onClick={() =>
                        act("paid", async () => {
                          await api.devMarkPaid(o.id);
                          setPay(null);
                          await load();
                          setMsg(t("支付成功，资金已冻结在持牌方。", "Payment successful; funds are held in escrow by the licensed custodian."));
                        })
                      }
                    >
                      {busy === "paid" ? t("确认中…", "Confirming…") : t("模拟支付成功（沙箱）", "Simulate payment (sandbox)")}
                    </Button>
                  </>
                )}
              </div>
            )}

            {(o.status === "paid" || o.status === "delivered") && (
              <div className="space-y-2">
                {o.product_type === "compute" ? (
                  <div className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
                    {t("计算权益已发放。前往", "Compute entitlement granted. Go to the")}{" "}
                    <a className="font-medium underline" href={`/datasets/${o.dataset_id}`}>
                      {t("数据集页", "dataset page")}
                    </a>{" "}
                    {t("使用「可用不可见」沙箱计算（提交作业、下载结果）。本订单不交付原始数据。", "to use available-but-invisible sandbox compute (submit jobs, download results). This order does not deliver raw data.")}
                  </div>
                ) : !downloadUrl ? (
                  <Button
                    className="w-full"
                    disabled={!!busy}
                    onClick={() =>
                      act("dl", async () => {
                        const d = await api.requestDownload(o.id);
                        setDownloadUrl(API_ORIGIN + d.download_url);
                        await load();
                      })
                    }
                  >
                    {busy === "dl" ? t("生成链接…", "Generating link…") : t("签署许可并获取下载链接", "Sign the license & get the download link")}
                  </Button>
                ) : (
                  <a
                    href={downloadUrl}
                    className="block w-full rounded-md bg-neutral-900 px-4 py-2 text-center text-sm font-medium text-white hover:bg-neutral-700"
                    download
                  >
                    {t("下载数据（一次性链接，15 分钟有效）", "Download data (one-time link, valid 15 min)")}
                  </a>
                )}
                <Button
                  variant="secondary"
                  className="w-full"
                  disabled={!!busy}
                  onClick={() =>
                    act("confirm", async () => {
                      await api.confirmDelivery(o.id);
                      await load();
                      setMsg(t("已确认收货，平台已自动结算给卖家。", "Receipt confirmed; the platform has auto-settled the seller."));
                    })
                  }
                >
                  {t("确认收货（触发分账结算）", "Confirm receipt (trigger settlement)")}
                </Button>
              </div>
            )}

            {o.status === "settled" && <ReviewBox orderId={o.id} />}

            {["paid", "delivered", "confirmed"].includes(o.status) && (
              <DisputeBox orderId={o.id} onDone={load} />
            )}
          </div>
        </Card>
      )}
    </div>
  );
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div>
      <dt className="text-neutral-400">{label}</dt>
      <dd className="font-medium text-neutral-800">{value}</dd>
    </div>
  );
}

function ReviewBox({ orderId }: { orderId: string }) {
  const { t } = useT();
  const [score, setScore] = useState(5);
  const [comment, setComment] = useState("");
  const [done, setDone] = useState(false);
  const [err, setErr] = useState("");

  if (done) return <Alert kind="success">{t("感谢评价！", "Thanks for your review!")}</Alert>;
  return (
    <div className="rounded-lg border border-neutral-200 p-4">
      <div className="mb-2 font-medium">{t("评价这单数据", "Review this purchase")}</div>
      {err && <div className="mb-2"><Alert>{err}</Alert></div>}
      <div className="mb-2 flex gap-1 text-2xl" role="radiogroup" aria-label={t("评分", "Rating")}>
        {[1, 2, 3, 4, 5].map((s) => (
          <button
            key={s}
            type="button"
            onClick={() => setScore(s)}
            role="radio"
            aria-checked={s === score}
            aria-label={t(`${s} 星`, `${s} star${s > 1 ? "s" : ""}`)}
            className={s <= score ? "text-amber-500" : "text-neutral-300"}
          >
            ★
          </button>
        ))}
      </div>
      <Textarea rows={2} placeholder={t("说点什么（可选）", "Say something (optional)")} value={comment} onChange={(e) => setComment(e.target.value)} />
      <div className="mt-2">
        <Button
          onClick={async () => {
            setErr("");
            try {
              await api.review(orderId, score, comment, score <= 2);
              setDone(true);
            } catch (e) {
              setErr((e as Error).message);
            }
          }}
        >
          {t("提交评价", "Submit review")}
        </Button>
      </div>
    </div>
  );
}

function DisputeBox({ orderId, onDone }: { orderId: string; onDone: () => Promise<void> }) {
  const { t } = useT();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [err, setErr] = useState("");
  if (!open)
    return (
      <Button variant="ghost" className="w-full text-red-600" onClick={() => setOpen(true)}>
        {t("对该订单有问题？发起纠纷", "Issue with this order? Open a dispute")}
      </Button>
    );
  return (
    <div className="rounded-lg border border-red-200 p-4">
      {err && <div className="mb-2"><Alert>{err}</Alert></div>}
      <Field label={t("纠纷原因", "Dispute reason")}>
        <Input value={reason} onChange={(e) => setReason(e.target.value)} />
      </Field>
      <div className="mt-2 flex gap-2">
        <Button
          variant="danger"
          onClick={async () => {
            setErr("");
            try {
              await api.dispute(orderId, reason);
              await onDone();
            } catch (e) {
              setErr((e as Error).message);
            }
          }}
        >
          {t("提交纠纷", "Submit dispute")}
        </Button>
        <Button variant="ghost" onClick={() => setOpen(false)}>
          {t("取消", "Cancel")}
        </Button>
      </div>
    </div>
  );
}
