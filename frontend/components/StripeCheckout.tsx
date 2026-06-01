"use client";

// Real Stripe.js checkout (test mode). Mounts a Payment Element bound to the
// PaymentIntent client secret the backend returns from POST /payments/create,
// then confirms the charge in-browser. On success Stripe fires
// payment_intent.succeeded → our webhook flips the order to "paid"; the parent
// polls getOrder to observe that transition (it never trusts the client alone).

import { useState } from "react";
import { Elements, PaymentElement, useElements, useStripe } from "@stripe/react-stripe-js";
import { loadStripe, type Stripe } from "@stripe/stripe-js";
import { Alert, Button } from "@/components/ui";

const PUBLISHABLE_KEY = process.env.NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY;

// loadStripe injects js.stripe.com once; cache the promise across mounts.
let stripePromise: Promise<Stripe | null> | null = null;
function getStripe(): Promise<Stripe | null> | null {
  if (!PUBLISHABLE_KEY) return null;
  if (!stripePromise) stripePromise = loadStripe(PUBLISHABLE_KEY);
  return stripePromise;
}

/** Whether a publishable key is configured (controls whether we offer real card pay). */
export const stripeConfigured = (): boolean => !!PUBLISHABLE_KEY;

export function StripeCheckout({
  clientSecret,
  amountLabel,
  onPaid,
}: {
  clientSecret: string;
  amountLabel: string;
  /** Confirm succeeded client-side; resolve once the order is observed paid. */
  onPaid: () => Promise<void>;
}) {
  const stripe = getStripe();
  if (!stripe) {
    return <Alert>未配置 Stripe 公钥（NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY），无法发起真实支付。</Alert>;
  }
  return (
    <Elements stripe={stripe} options={{ clientSecret, appearance: { theme: "stripe" } }}>
      <CheckoutForm amountLabel={amountLabel} onPaid={onPaid} />
    </Elements>
  );
}

function CheckoutForm({ amountLabel, onPaid }: { amountLabel: string; onPaid: () => Promise<void> }) {
  const stripe = useStripe();
  const elements = useElements();
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!stripe || !elements) return;
    setErr("");
    setBusy(true);
    try {
      // redirect: "if_required" keeps card payments inline; only methods that
      // truly need a redirect (e.g. 3DS challenge) leave the page.
      const { error, paymentIntent } = await stripe.confirmPayment({
        elements,
        confirmParams: { return_url: window.location.href },
        redirect: "if_required",
      });
      if (error) {
        setErr(error.message ?? "支付失败，请重试。");
        return;
      }
      const status = paymentIntent?.status;
      if (status === "succeeded" || status === "processing") {
        // Funds captured at Stripe; wait for the webhook to flip the order.
        await onPaid();
      } else {
        setErr(`支付未完成（状态：${status ?? "unknown"}）。`);
      }
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="space-y-3">
      <PaymentElement onReady={() => setReady(true)} />
      {err && <Alert>{err}</Alert>}
      <Button type="submit" className="w-full" disabled={!stripe || !ready || busy}>
        {busy ? "支付处理中…" : `支付 ${amountLabel}`}
      </Button>
      <p className="text-xs text-neutral-400">
        测试卡号 4242 4242 4242 4242，任意未来有效期 / CVC / 邮编。资金冻结在持牌方，确认收货后分账。
      </p>
    </form>
  );
}
