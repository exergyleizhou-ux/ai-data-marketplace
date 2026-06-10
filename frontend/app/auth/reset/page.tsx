"use client";

import { useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Alert, Button, Card, Field, Input } from "@/components/ui";

export default function ResetPasswordPage() {
  const { t } = useT();
  const [step, setStep] = useState<"request" | "complete" | "done">("request");
  const [account, setAccount] = useState("");
  const [token, setToken] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function request(e: React.FormEvent) {
    e.preventDefault();
    setErr(""); setBusy(true);
    try {
      await api.requestPasswordReset(account);
      setStep("complete");
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(false); }
  }

  async function complete(e: React.FormEvent) {
    e.preventDefault();
    setErr(""); setBusy(true);
    try {
      await api.completePasswordReset(token, newPassword);
      setStep("done");
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(false); }
  }

  if (step === "done") {
    return (
      <div className="mx-auto max-w-sm">
        <Card>
          <h1 className="mb-4 text-xl font-semibold">{t("密码已重置", "Password reset")}</h1>
          <p className="mb-4 text-neutral-600">{t("请使用新密码登录。", "Please sign in with your new password.")}</p>
          <Link href="/login"><Button className="w-full">{t("去登录", "Sign in")}</Button></Link>
        </Card>
      </div>
    );
  }

  if (step === "complete") {
    return (
      <div className="mx-auto max-w-sm">
        <Card>
          <h1 className="mb-4 text-xl font-semibold">{t("设置新密码", "Set new password")}</h1>
          <form onSubmit={complete} className="space-y-4">
            {err && <Alert>{err}</Alert>}
            <Field label={t("重置令牌", "Reset token")}>
              <Input value={token} onChange={(e) => setToken(e.target.value)} required autoFocus />
            </Field>
            <Field label={t("新密码", "New password")}>
              <Input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} required />
            </Field>
            <Button type="submit" disabled={busy} className="w-full">{t("重置", "Reset")}</Button>
          </form>
        </Card>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-sm">
      <Card>
        <h1 className="mb-4 text-xl font-semibold">{t("找回密码", "Forgot password")}</h1>
        <form onSubmit={request} className="space-y-4">
          {err && <Alert>{err}</Alert>}
          <Field label={t("账号（手机号 / 邮箱）", "Account (phone / email)")}>
            <Input value={account} onChange={(e) => setAccount(e.target.value)} required autoFocus />
          </Field>
          <Button type="submit" disabled={busy} className="w-full">{t("发送重置令牌", "Send reset token")}</Button>
        </form>
        <p className="mt-4 text-sm text-neutral-500">
          <Link href="/login">{t("← 返回登录", "← Back to sign in")}</Link>
        </p>
      </Card>
    </div>
  );
}
