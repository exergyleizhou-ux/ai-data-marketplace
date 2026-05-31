"use client";

import { useEffect, useState } from "react";
import { api, type KYC } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Protected } from "@/components/Protected";
import { Alert, Badge, Button, Card, Field, Input, Select } from "@/components/ui";

export default function AccountPage() {
  return (
    <Protected>
      <AccountInner />
    </Protected>
  );
}

function AccountInner() {
  const { user, refresh } = useAuth();
  const [kyc, setKyc] = useState<KYC | null>(null);
  const [msg, setMsg] = useState("");
  const [err, setErr] = useState("");

  // KYC form
  const [type, setType] = useState("personal");
  const [realName, setRealName] = useState("");
  const [companyName, setCompanyName] = useState("");
  const [idNo, setIdNo] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.getKYC().then(setKyc).catch(() => setKyc(null));
  }, []);

  async function submitKYC(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    setMsg("");
    setBusy(true);
    try {
      const k = await api.submitKYC({
        type,
        real_name: type === "personal" ? realName : undefined,
        company_name: type === "company" ? companyName : undefined,
        id_no: idNo || undefined,
        material_urls: [],
      });
      setKyc(k);
      setMsg("实名材料已提交。");
      await refresh();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function setRole(role: string) {
    setErr("");
    setMsg("");
    try {
      await api.updateRole(role);
      await refresh();
      setMsg(`角色已更新为 ${role}`);
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  if (!user) return null;

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <h1 className="text-2xl font-semibold">账户</h1>
      {msg && <Alert kind="success">{msg}</Alert>}
      {err && <Alert>{err}</Alert>}

      <Card>
        <div className="flex items-center justify-between">
          <div>
            <div className="text-sm text-neutral-500">账号</div>
            <div className="font-medium">{user.account}</div>
          </div>
          <div className="text-right">
            <div className="text-sm text-neutral-500">实名状态</div>
            <Badge>{user.kyc_status}</Badge>
          </div>
        </div>
        <div className="mt-4 border-t border-neutral-100 pt-4">
          <div className="mb-2 text-sm text-neutral-500">角色（买家 / 卖家 / 兼具）</div>
          <div className="flex gap-2">
            {["buyer", "seller", "both"].map((r) => (
              <Button key={r} variant={user.role === r ? "primary" : "secondary"} onClick={() => setRole(r)}>
                {r}
              </Button>
            ))}
          </div>
        </div>
      </Card>

      <Card>
        <h2 className="mb-1 text-lg font-semibold">实名认证</h2>
        <p className="mb-4 text-sm text-neutral-500">
          买卖数据需先通过实名认证（合规硬性要求）。身份证号经哈希存储，不留明文。
          {kyc && (
            <>
              {" "}当前提交状态：<Badge>{kyc.verify_status}</Badge>
            </>
          )}
        </p>
        <form onSubmit={submitKYC} className="space-y-4">
          <Field label="类型">
            <Select value={type} onChange={(e) => setType(e.target.value)}>
              <option value="personal">个人</option>
              <option value="company">企业</option>
            </Select>
          </Field>
          {type === "personal" ? (
            <>
              <Field label="真实姓名">
                <Input value={realName} onChange={(e) => setRealName(e.target.value)} required />
              </Field>
              <Field label="身份证号">
                <Input value={idNo} onChange={(e) => setIdNo(e.target.value)} required />
              </Field>
            </>
          ) : (
            <Field label="企业名称">
              <Input value={companyName} onChange={(e) => setCompanyName(e.target.value)} required />
            </Field>
          )}
          <Button type="submit" disabled={busy}>
            {busy ? "提交中…" : "提交实名"}
          </Button>
        </form>
      </Card>
    </div>
  );
}
