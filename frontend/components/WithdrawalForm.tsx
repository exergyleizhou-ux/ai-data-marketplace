"use client";

import { useState } from "react";
import { api, yuan } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Alert, Button, Field, Input, Select } from "@/components/ui";

// channel and amount caps mirror backend/internal/modules/withdrawal/service.go
// (channel enum + 1_000_000 yuan cap). Errors are surfaced inline; the backend
// returns the same generic ErrAmountInvalid for empty account_label as for bad
// amount — we pre-validate label client-side so the message stays specific.
const CHANNELS = ["alipay", "wechat", "bank"] as const;
const MAX_YUAN = 1_000_000;

export function WithdrawalForm({
  withdrawableCents,
  onCreated,
}: {
  withdrawableCents: number;
  onCreated: () => void;
}) {
  const { t } = useT();
  const [amount, setAmount] = useState("");
  const [channel, setChannel] = useState<(typeof CHANNELS)[number]>("alipay");
  const [accountLabel, setAccountLabel] = useState("");
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState(false);

  function channelLabel(c: string): string {
    if (c === "alipay") return t("支付宝", "Alipay");
    if (c === "wechat") return t("微信", "WeChat");
    if (c === "bank") return t("银行卡", "Bank card");
    return c;
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(""); setOk("");
    const yuanNum = Number(amount);
    if (!Number.isFinite(yuanNum) || yuanNum <= 0) {
      setErr(t("请输入有效金额", "Please enter a valid amount"));
      return;
    }
    if (yuanNum > MAX_YUAN) {
      setErr(t("单笔提现不能超过 100 万元", "Single withdrawal cannot exceed ¥1,000,000"));
      return;
    }
    const cents = Math.round(yuanNum * 100);
    if (cents > withdrawableCents) {
      setErr(t("超过可提现余额", "Exceeds withdrawable balance"));
      return;
    }
    const label = accountLabel.trim();
    if (label.length === 0) {
      setErr(t("请填写收款账号备注", "Please enter the payout account label"));
      return;
    }
    if (label.length > 200) {
      setErr(t("收款账号备注不能超过 200 字符", "Account label must not exceed 200 chars"));
      return;
    }

    setBusy(true);
    try {
      await api.requestWithdrawal({ amount_cents: cents, channel, account_label: label });
      setOk(t("已提交,等待运营审核", "Submitted, pending operator review"));
      setAmount(""); setAccountLabel("");
      onCreated();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      {err && <Alert>{err}</Alert>}
      {ok && <Alert kind="success">{ok}</Alert>}
      <Field
        label={t("金额(元)", "Amount (¥)")}
        hint={t(
          `可提现 ${yuan(withdrawableCents)};单笔上限 ¥1,000,000`,
          `Withdrawable ${yuan(withdrawableCents)}; single-request cap ¥1,000,000`,
        )}
      >
        <Input
          type="number"
          step="0.01"
          min="0"
          max={MAX_YUAN}
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
          required
        />
      </Field>
      <Field label={t("收款渠道", "Payout channel")}>
        <Select value={channel} onChange={(e) => setChannel(e.target.value as (typeof CHANNELS)[number])}>
          {CHANNELS.map((c) => (
            <option key={c} value={c}>
              {channelLabel(c)}
            </option>
          ))}
        </Select>
      </Field>
      <Field
        label={t("收款账号备注", "Account label")}
        hint={t(
          "如:支付宝绑定手机尾号 1234 / 工行卡尾号 5678(运营按此打款)",
          "e.g. Alipay phone ending 1234 / ICBC card ending 5678 (ops use this to pay out)",
        )}
      >
        <Input
          value={accountLabel}
          onChange={(e) => setAccountLabel(e.target.value)}
          maxLength={200}
          required
        />
      </Field>
      <Button type="submit" disabled={busy} className="w-full">
        {busy ? t("提交中…", "Submitting…") : t("申请提现", "Request withdrawal")}
      </Button>
    </form>
  );
}
