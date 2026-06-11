"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useT } from "@/lib/i18n";
import { Alert, Button, Card, Field, Input } from "@/components/ui";

export default function LoginPage() {
  const { t } = useT();
  const router = useRouter();
  const { login, setSession } = useAuth();
  const [account, setAccount] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  // 2FA state
  const [need2FA, setNeed2FA] = useState(false);
  const [challenge, setChallenge] = useState("");
  const [code, setCode] = useState("");

  async function submitLogin(e: React.FormEvent) {
    e.preventDefault();
    setErr(""); setBusy(true);
    try {
      // Route through AuthProvider.login so the nav re-renders synchronously —
      // bypassing it (api.login + tokenStore.set) leaves user=null until reload.
      const res = await login(account, password);
      if (res.need_2fa && res.challenge_token) {
        setNeed2FA(true);
        setChallenge(res.challenge_token);
      } else if (res.tokens && res.user) {
        router.push("/datasets");
      }
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(false); }
  }

  async function submit2FA(e: React.FormEvent) {
    e.preventDefault();
    setErr(""); setBusy(true);
    try {
      const res = await api.verify2FA(challenge, code);
      // Same reason: route through the context, not a bare tokenStore.set.
      setSession(res.user, res.tokens);
      router.push("/datasets");
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(false); }
  }

  if (need2FA) {
    return (
      <div className="mx-auto max-w-sm">
        <Card>
          <h1 className="mb-4 text-xl font-semibold">{t("两步验证", "Two-factor authentication")}</h1>
          <form onSubmit={submit2FA} className="space-y-4">
            {err && <Alert>{err}</Alert>}
            <Field label={t("TOTP 验证码 或 恢复码", "TOTP code or recovery code")}>
              <Input value={code} onChange={(e) => setCode(e.target.value)} required autoFocus />
            </Field>
            <Button type="submit" disabled={busy} className="w-full">
              {t("验证", "Verify")}
            </Button>
          </form>
        </Card>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-sm">
      <Card>
        <h1 className="mb-4 text-xl font-semibold">{t("登录", "Sign in")}</h1>
        <form onSubmit={submitLogin} className="space-y-4">
          {err && <Alert>{err}</Alert>}
          <Field label={t("账号", "Account")}>
            <Input value={account} onChange={(e) => setAccount(e.target.value)} autoComplete="username" required />
          </Field>
          <Field label={t("密码", "Password")}>
            <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required />
          </Field>
          <Button type="submit" disabled={busy} className="w-full">
            {busy ? t("登录中…", "Signing in…") : t("登录", "Sign in")}
          </Button>
        </form>
        <p className="mt-4 text-sm text-neutral-500">
          <Link href="/auth/reset" className="text-neutral-500 hover:underline">{t("忘记密码?", "Forgot password?")}</Link>
          {" · "}
          <Link href="/register" className="text-neutral-900 hover:underline">{t("注册", "Sign up")}</Link>
        </p>
      </Card>
    </div>
  );
}
