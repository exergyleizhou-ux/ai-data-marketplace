"use client";

import { useEffect, useState } from "react";
import { api, type KYC } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useT } from "@/lib/i18n";
import { Protected } from "@/components/Protected";
import { FederatedComputePanel } from "@/components/Compute";
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
  const { t } = useT();
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
      setMsg(t("实名材料已提交。", "Verification materials submitted."));
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
      setMsg(t(`角色已更新为 ${role}`, `Role updated to ${role}`));
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  if (!user) return null;

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <h1 className="text-2xl font-semibold">{t("账户", "Account")}</h1>
      {msg && <Alert kind="success">{msg}</Alert>}
      {err && <Alert>{err}</Alert>}

      <Card>
        <div className="flex items-center justify-between">
          <div>
            <div className="text-sm text-neutral-500">{t("账号", "Account")}</div>
            <div className="font-medium">{user.account}</div>
          </div>
          <div className="text-right">
            <div className="text-sm text-neutral-500">{t("实名状态", "Verification")}</div>
            <Badge>{user.kyc_status}</Badge>
          </div>
        </div>
        <div className="mt-4 border-t border-neutral-100 pt-4">
          <div className="mb-2 text-sm text-neutral-500">{t("角色（买家 / 卖家 / 兼具）", "Role (buyer / seller / both)")}</div>
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
        <h2 className="mb-1 text-lg font-semibold">{t("实名认证", "Real-name verification")}</h2>
        <p className="mb-4 text-sm text-neutral-500">
          {t(
            "买卖数据需先通过实名认证（合规硬性要求）。身份证号经哈希存储，不留明文。",
            "Buying or selling data requires real-name verification (a hard compliance requirement). ID numbers are stored hashed, never in plaintext.",
          )}
          {kyc && (
            <>
              {" "}
              {t("当前提交状态：", "Current status: ")}
              <Badge>{kyc.verify_status}</Badge>
            </>
          )}
        </p>
        <form onSubmit={submitKYC} className="space-y-4">
          <Field label={t("类型", "Type")}>
            <Select value={type} onChange={(e) => setType(e.target.value)}>
              <option value="personal">{t("个人", "Individual")}</option>
              <option value="company">{t("企业", "Company")}</option>
            </Select>
          </Field>
          {type === "personal" ? (
            <>
              <Field label={t("真实姓名", "Legal name")}>
                <Input value={realName} onChange={(e) => setRealName(e.target.value)} required />
              </Field>
              <Field label={t("身份证号", "ID number")}>
                <Input value={idNo} onChange={(e) => setIdNo(e.target.value)} required />
              </Field>
            </>
          ) : (
            <Field label={t("企业名称", "Company name")}>
              <Input value={companyName} onChange={(e) => setCompanyName(e.target.value)} required />
            </Field>
          )}
          <Button type="submit" disabled={busy}>
            {busy ? t("提交中…", "Submitting…") : t("提交实名", "Submit verification")}
          </Button>
        </form>
      </Card>

      <FederatedComputePanel />
    </div>
  );
}
