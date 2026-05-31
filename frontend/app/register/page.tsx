"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useAuth } from "@/lib/auth";
import { Alert, Button, Card, Field, Input, Select } from "@/components/ui";

export default function RegisterPage() {
  const { register } = useAuth();
  const router = useRouter();
  const [account, setAccount] = useState("");
  const [accountType, setAccountType] = useState("email");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      await register(account, accountType, password);
      router.push("/account");
    } catch (e) {
      setErr((e as Error).message || "注册失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mx-auto max-w-sm">
      <Card>
        <h1 className="mb-4 text-xl font-semibold">注册</h1>
        <form onSubmit={submit} className="space-y-4">
          {err && <Alert>{err}</Alert>}
          <Field label="账号类型">
            <Select value={accountType} onChange={(e) => setAccountType(e.target.value)}>
              <option value="email">邮箱</option>
              <option value="phone">手机号</option>
            </Select>
          </Field>
          <Field label="账号">
            <Input value={account} onChange={(e) => setAccount(e.target.value)} required />
          </Field>
          <Field label="密码" hint="至少 8 位">
            <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required />
          </Field>
          <Button type="submit" disabled={busy} className="w-full">
            {busy ? "注册中…" : "注册"}
          </Button>
        </form>
        <p className="mt-4 text-sm text-neutral-500">
          注册后需完成实名认证才能买卖。已有账号？{" "}
          <Link href="/login" className="font-medium text-neutral-900 hover:underline">
            登录
          </Link>
        </p>
      </Card>
    </div>
  );
}
