"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useAuth } from "@/lib/auth";
import { useT } from "@/lib/i18n";
import { SIGNUP_AGREEMENTS } from "@/lib/legal";
import { Alert, AuthShell, Button, Field, Input, Select } from "@/components/ui";

export default function RegisterPage() {
  const { user, register } = useAuth();
  const { t } = useT();
  const router = useRouter();

  // Already signed in → skip the form.
  useEffect(() => {
    if (user) router.replace("/account");
  }, [user, router]);
  const [account, setAccount] = useState("");
  const [accountType, setAccountType] = useState("email");
  const [password, setPassword] = useState("");
  const [agreed, setAgreed] = useState(false);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!agreed) {
      setErr(t("请先阅读并同意《用户服务协议》和《隐私政策》", "Please read and accept the Terms of Service and Privacy Policy first"));
      return;
    }
    setErr("");
    setBusy(true);
    try {
      await register(account, accountType, password, SIGNUP_AGREEMENTS);
      router.push("/account");
    } catch (e) {
      setErr((e as Error).message || t("注册失败", "Sign-up failed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthShell
      kicker={t("加入绿洲", "Join Verdant Oasis")}
      title={t("注册", "Sign up")}
      footer={
        <>
          {t("注册后需完成实名认证才能买卖。已有账号?", "Real-name verification is required to buy or sell. Have an account?")}{" "}
          <Link href="/login" className="font-medium text-ink cue-underline">
            {t("登录", "Sign in")}
          </Link>
        </>
      }
    >
      <form onSubmit={submit} className="space-y-4">
        {err && <Alert>{err}</Alert>}
        <Field label={t("账号类型", "Account type")}>
          <Select value={accountType} onChange={(e) => setAccountType(e.target.value)}>
            <option value="email">{t("邮箱", "Email")}</option>
            <option value="phone">{t("手机号", "Phone")}</option>
          </Select>
        </Field>
        <Field label={t("账号", "Account")}>
          <Input value={account} onChange={(e) => setAccount(e.target.value)} required />
        </Field>
        <Field label={t("密码", "Password")} hint={t("至少 8 位", "At least 8 characters")}>
          <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required />
        </Field>
        <label className="flex items-start gap-2 text-sm text-ink/70">
          <input type="checkbox" checked={agreed} onChange={(e) => setAgreed(e.target.checked)} className="mt-0.5" />
          <span>
            {t("我已阅读并同意", "I have read and accept the")}{" "}
            <Link href="/terms" target="_blank" className="font-medium text-ink cue-underline">
              {t("《用户服务协议》", "Terms of Service")}
            </Link>{" "}
            {t("和", "and")}{" "}
            <Link href="/privacy" target="_blank" className="font-medium text-ink cue-underline">
              {t("《隐私政策》", "Privacy Policy")}
            </Link>
          </span>
        </label>
        <Button type="submit" disabled={busy || !agreed} className="w-full">
          {busy ? t("注册中…", "Signing up…") : t("注册", "Sign up")}
        </Button>
      </form>
    </AuthShell>
  );
}
