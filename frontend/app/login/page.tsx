"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useAuth } from "@/lib/auth";
import { useT } from "@/lib/i18n";
import { Alert, Button, Card, Field, Input } from "@/components/ui";

export default function LoginPage() {
  const { login } = useAuth();
  const { t } = useT();
  const router = useRouter();
  const [account, setAccount] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      await login(account, password);
      router.push("/datasets");
    } catch (e) {
      setErr((e as Error).message || t("登录失败", "Sign-in failed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mx-auto max-w-sm">
      <Card>
        <h1 className="mb-4 text-xl font-semibold">{t("登录", "Sign in")}</h1>
        <form onSubmit={submit} className="space-y-4">
          {err && <Alert>{err}</Alert>}
          <Field label={t("账号（手机号 / 邮箱）", "Account (phone / email)")}>
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
          {t("还没有账号？", "No account yet?")}{" "}
          <Link href="/register" className="font-medium text-neutral-900 hover:underline">
            {t("注册", "Sign up")}
          </Link>
        </p>
      </Card>
    </div>
  );
}
