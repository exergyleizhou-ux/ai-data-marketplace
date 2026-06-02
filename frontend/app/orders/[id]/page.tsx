"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { API_ORIGIN, api, yuan, type Order } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Protected } from "@/components/Protected";
import { Alert, Badge, Button, Card, Field, Input, Spinner, Textarea } from "@/components/ui";
import { StripeCheckout, stripeConfigured } from "@/components/StripeCheckout";

type PayInfo = { pay_url: string; channel_txn_id: string; amount_cents: number; channel: string };

export default function OrderDetailPage({ params }: { params: { id: string } }) {
  const { id } = params;
  return (
    <Protected>
      <OrderInner id={id} />
    </Protected>
  );
}

function OrderInner({ id }: { id: string }) {
  const { user } = useAuth();
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
        setMsg("支付成功，资金已冻结在持牌方。");
        return;
      }
      await new Promise((r) => setTimeout(r, 1500));
    }
    // Charge captured at Stripe but the webhook hasn't landed yet; let the user retry the read.
    await load();
    setMsg("支付已提交，正在等待结算回调确认，请稍后刷新。");
  }, [id, load]);

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
        <h1 className="text-xl font-semibold">订单详情</h1>
        <Badge>{o.status}</Badge>
      </div>

      <Card>
        <dl className="grid grid-cols-2 gap-3 text-sm">
          <Row label="订单号" value={`#${o.id.slice(0, 8)}`} />
          <Row label="数据集" value={<Link href={`/datasets/${o.dataset_id}`} className="text-neutral-900 underline">查看</Link>} />
          <Row label="金额" value={yuan(o.amount_cents)} />
          <Row label="平台佣金" value={yuan(o.platform_fee_cents)} />
          <Row label="卖家所得" value={yuan(o.seller_amount_cents)} />
          <Row label="许可" value={o.license_type} />
        </dl>
      </Card>

      {msg && <Alert kind="success">{msg}</Alert>}
      {err && <Alert>{err}</Alert>}

      {isBuyer && (
        <Card>
          <h2 className="mb-3 font-semibold">操作</h2>
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
                    {busy === "pay" ? "创建支付…" : "去支付"}
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
                    <Alert kind="info">沙箱支付单已创建（{pay.channel_txn_id}）。真实环境会跳转至收银台。</Alert>
                    <Button
                      className="w-full"
                      disabled={!!busy}
                      onClick={() =>
                        act("paid", async () => {
                          await api.devMarkPaid(o.id);
                          setPay(null);
                          await load();
                          setMsg("支付成功，资金已冻结在持牌方。");
                        })
                      }
                    >
                      {busy === "paid" ? "确认中…" : "模拟支付成功（沙箱）"}
                    </Button>
                  </>
                )}
              </div>
            )}

            {(o.status === "paid" || o.status === "delivered") && (
              <div className="space-y-2">
                {o.product_type === "compute" ? (
                  <div className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
                    计算权益已发放。前往{" "}
                    <a className="font-medium underline" href={`/datasets/${o.dataset_id}`}>
                      数据集页
                    </a>{" "}
                    使用「可用不可见」沙箱计算（提交作业、下载结果）。本订单不交付原始数据。
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
                    {busy === "dl" ? "生成链接…" : "签署许可并获取下载链接"}
                  </Button>
                ) : (
                  <a
                    href={downloadUrl}
                    className="block w-full rounded-md bg-neutral-900 px-4 py-2 text-center text-sm font-medium text-white hover:bg-neutral-700"
                    download
                  >
                    下载数据（一次性链接，15 分钟有效）
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
                      setMsg("已确认收货，平台已自动结算给卖家。");
                    })
                  }
                >
                  确认收货（触发分账结算）
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
  const [score, setScore] = useState(5);
  const [comment, setComment] = useState("");
  const [done, setDone] = useState(false);
  const [err, setErr] = useState("");

  if (done) return <Alert kind="success">感谢评价！</Alert>;
  return (
    <div className="rounded-lg border border-neutral-200 p-4">
      <div className="mb-2 font-medium">评价这单数据</div>
      {err && <div className="mb-2"><Alert>{err}</Alert></div>}
      <div className="mb-2 flex gap-1 text-2xl">
        {[1, 2, 3, 4, 5].map((s) => (
          <button key={s} onClick={() => setScore(s)} className={s <= score ? "text-amber-500" : "text-neutral-300"}>
            ★
          </button>
        ))}
      </div>
      <Textarea rows={2} placeholder="说点什么（可选）" value={comment} onChange={(e) => setComment(e.target.value)} />
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
          提交评价
        </Button>
      </div>
    </div>
  );
}

function DisputeBox({ orderId, onDone }: { orderId: string; onDone: () => Promise<void> }) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [err, setErr] = useState("");
  if (!open)
    return (
      <Button variant="ghost" className="w-full text-red-600" onClick={() => setOpen(true)}>
        对该订单有问题？发起纠纷
      </Button>
    );
  return (
    <div className="rounded-lg border border-red-200 p-4">
      {err && <div className="mb-2"><Alert>{err}</Alert></div>}
      <Field label="纠纷原因">
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
          提交纠纷
        </Button>
        <Button variant="ghost" onClick={() => setOpen(false)}>
          取消
        </Button>
      </div>
    </div>
  );
}
