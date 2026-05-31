"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { API_ORIGIN, api, yuan, type Order } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Protected } from "@/components/Protected";
import { Alert, Badge, Button, Card, Field, Input, Spinner, Textarea } from "@/components/ui";

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
  const [payTxn, setPayTxn] = useState("");
  const [downloadUrl, setDownloadUrl] = useState("");

  const load = useCallback(async () => {
    try {
      setO(await api.getOrder(id));
    } catch (e) {
      setErr((e as Error).message);
    }
  }, [id]);

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
                {!payTxn ? (
                  <Button
                    className="w-full"
                    disabled={!!busy}
                    onClick={() =>
                      act("pay", async () => {
                        const p = await api.createPayment(o.id);
                        setPayTxn(p.channel_txn_id);
                      })
                    }
                  >
                    {busy === "pay" ? "创建支付…" : "去支付"}
                  </Button>
                ) : (
                  <>
                    <Alert kind="info">沙箱支付单已创建（{payTxn}）。真实环境会跳转至微信/支付宝收银台。</Alert>
                    <Button
                      className="w-full"
                      disabled={!!busy}
                      onClick={() =>
                        act("paid", async () => {
                          await api.devMarkPaid(o.id);
                          setPayTxn("");
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
                {!downloadUrl ? (
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
