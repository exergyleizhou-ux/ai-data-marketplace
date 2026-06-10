"use client";

import { useCallback, useEffect, useState } from "react";
import { api, yuan, type KYC, type EarningsPoint, type EarningsByDataset, type Watch, type DataExportJob } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useT } from "@/lib/i18n";
import { Protected } from "@/components/Protected";
import { FederatedComputePanel, PSIComputePanel } from "@/components/Compute";
import Link from "next/link";
import { MiniChart } from "@/components/MiniChart";
import { Alert, Badge, Button, Card, Field, Input, Select, Spinner } from "@/components/ui";

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

      <TwoFactorCard />

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

      {user.role === "seller" || user.role === "both" ? <SellerAnalytics /> : null}

      <WatchlistCard />

      <NotificationPreferencesCard />

      <DataRightsCard />

      <FederatedComputePanel />
      <PSIComputePanel />
    </div>
  );
}

function SellerAnalytics() {
  const { t } = useT();
  const [tsDays, setTsDays] = useState(7);
  const [pts, setPts] = useState<EarningsPoint[] | null>(null);
  const [byDs, setByDs] = useState<EarningsByDataset[] | null>(null);
  const [err, setErr] = useState("");

  const load = useCallback(async () => {
    setErr("");
    try {
      const [tRes, dRes] = await Promise.all([
        api.sellerEarningsTimeseries(tsDays),
        api.sellerEarningsByDataset(),
      ]);
      setPts(tRes.points);
      setByDs(dRes.items);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, [tsDays]);
  useEffect(() => { void load(); }, [load]);

  if (pts === null && byDs === null) return <Spinner />;

  return (
    <Card>
      <h2 className="mb-3 font-semibold">{t("卖家收益分析", "Seller earnings analytics")} <span className="font-normal text-neutral-400">/ Analytics</span></h2>
      {err && <Alert>{err}</Alert>}
      <div className="flex items-center gap-2 mb-3">
        <span className="text-xs text-neutral-500">{t("时间范围", "Range")}:</span>
        {([7, 30, 90] as const).map((d) => (
          <button
            key={d}
            onClick={() => setTsDays(d)}
            className={`rounded px-2 py-0.5 text-xs ${tsDays === d ? "bg-neutral-900 text-white" : "border text-neutral-500"}`}
          >
            {d}{t("天", "d")}
          </button>
        ))}
      </div>
      {pts && pts.length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2 mb-4">
          <div>
            <div className="text-xs text-neutral-500 mb-1">{t("总收益", "Gross revenue")}</div>
            <MiniChart
              points={pts.map((p) => ({ date: p.date, value: p.gross_cents / 100 }))}
              color="#3b82f6"
              height={60}
              label="Gross revenue trend"
            />
            <div className="mt-1 text-xs text-neutral-400 text-right">
              {yuan(pts.reduce((s, p) => s + p.gross_cents, 0))}
            </div>
          </div>
          <div>
            <div className="text-xs text-neutral-500 mb-1">{t("已结算收益", "Settled revenue")}</div>
            <MiniChart
              points={pts.map((p) => ({ date: p.date, value: p.settled_cents / 100 }))}
              color="#22c55e"
              height={60}
              label="Settled revenue trend"
            />
            <div className="mt-1 text-xs text-neutral-400 text-right">
              {yuan(pts.reduce((s, p) => s + p.settled_cents, 0))}
            </div>
          </div>
        </div>
      )}
      {byDs && byDs.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="border-b border-neutral-200 text-left text-neutral-500">
              <tr>
                <th className="px-2 py-1 font-medium">{t("数据集", "Dataset")}</th>
                <th className="px-2 py-1 font-medium text-right">{t("总订单", "Orders")}</th>
                <th className="px-2 py-1 font-medium text-right">{t("已结算", "Settled")}</th>
                <th className="px-2 py-1 font-medium text-right">{t("总额", "Gross")}</th>
                <th className="px-2 py-1 font-medium text-right">{t("已结算额", "Settled amt")}</th>
                <th className="px-2 py-1 font-medium text-right">{t("最近订单", "Last order")}</th>
              </tr>
            </thead>
            <tbody>
              {byDs.map((d) => (
                <tr key={d.dataset_id} className="border-b border-neutral-100 last:border-0">
                  <td className="px-2 py-1.5 max-w-[160px] truncate" title={d.title || d.dataset_id}>
                    {d.title || d.dataset_id.slice(0, 8)}
                  </td>
                  <td className="px-2 py-1.5 text-right tabular-nums">{d.total_orders}</td>
                  <td className="px-2 py-1.5 text-right tabular-nums">{d.settled_orders}</td>
                  <td className="px-2 py-1.5 text-right tabular-nums">{yuan(d.gross_cents)}</td>
                  <td className="px-2 py-1.5 text-right tabular-nums">{yuan(d.settled_cents)}</td>
                  <td className="px-2 py-1.5 text-right text-xs text-neutral-400">{d.last_order_at?.slice(0, 10) || "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {byDs && byDs.length === 0 && (
        <p className="text-sm text-neutral-400">{t("暂无已售数据集", "No sold datasets yet")}</p>
      )}
    </Card>
  );
}

function WatchlistCard() {
  const { t } = useT();
  const [items, setItems] = useState<Watch[] | null>(null);

  useEffect(() => {
    api.listMyWatches().then((r) => setItems(r.items)).catch(() => setItems(null));
  }, []);

  if (items === null) return null;
  if (items.length === 0) return null;

  return (
    <Card>
      <h2 className="mb-2 font-semibold">
        {t("关注的数据集", "Watched datasets")} <span className="font-normal text-neutral-400">/ Watchlist</span>
      </h2>
      <div className="space-y-1">
        {items.map((w) => (
          <Link
            key={w.dataset_id}
            href={`/datasets/${w.dataset_id}`}
            className="flex items-center justify-between rounded px-2 py-1 text-sm hover:bg-neutral-50"
          >
            <span>{w.dataset_title || w.dataset_id.slice(0, 8)}</span>
            <span className="text-xs text-neutral-400">{w.created_at?.slice(0, 10)}</span>
          </Link>
        ))}
      </div>
    </Card>
  );
}

function DataRightsCard() {
  const { t } = useT();
  const [exportJob, setExportJob] = useState<DataExportJob | null>(null);
  const [expErr, setExpErr] = useState("");
  const [dReason, setDReason] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.getMyDataExport().then(setExportJob).catch(() => {});
  }, []);

  async function requestExport() {
    setBusy(true); setExpErr("");
    try { setExportJob(await api.requestDataExport()); }
    catch (e) { setExpErr((e as Error).message); }
    finally { setBusy(false); }
  }

  async function requestDeletion() {
    if (!dReason.trim()) return;
    setBusy(true); setExpErr("");
    try {
      await api.requestAccountDeletion(dReason.trim());
      setDReason("");
      alert("已提交注销申请，7 天冷静期内可撤销");
    } catch (e) { setExpErr((e as Error).message); }
    finally { setBusy(false); }
  }

  return (
    <Card>
      <h2 className="mb-3 font-semibold">
        {t("数据权利 (PIPL)", "Data rights (PIPL)")} <span className="font-normal text-neutral-400">/ PIPL Art. 45/47</span>
      </h2>
      {expErr && <Alert>{expErr}</Alert>}
      <div className="mb-3">
        <div className="text-sm font-medium">{t("下载我的数据", "Download my data")}</div>
        <p className="text-xs text-neutral-500 mb-1">{t("下载您所有个人数据 (PIPL 第45条)", "Download all your data (PIPL Article 45)")}</p>
        {exportJob ? (
          <div className="text-xs text-neutral-500">
            {t("状态", "Status")}: {exportJob.status}
            {exportJob.status === "ready" && (
              <Button variant="secondary" onClick={() => api.downloadMyDataExport()} className="ml-2">
                {t("下载", "Download")}
              </Button>
            )}
            {exportJob.error && <span className="text-red-500"> · {exportJob.error}</span>}
          </div>
        ) : (
          <Button variant="secondary" onClick={requestExport} disabled={busy}>
            {t("发起导出", "Request export")}
          </Button>
        )}
      </div>
      <div className="rounded-md border border-rose-200 p-3">
        <div className="text-sm font-medium text-rose-700">{t("注销账号", "Delete account")}</div>
        <p className="text-xs text-neutral-500 mb-2">{t("7 天冷静期后可撤销 (PIPL 第47条)", "7-day cooling-off (PIPL Art. 47)")}</p>
        <div className="flex gap-2">
          <input className="flex-1 rounded-md border border-neutral-300 px-2 py-1 text-sm"
            value={dReason} onChange={(e) => setDReason(e.target.value)}
            placeholder={t("注销原因 (可选)", "Reason (optional)") as string} />
          <Button variant="danger" onClick={requestDeletion} disabled={busy}>
            {t("提交注销", "Request")}
          </Button>
        </div>
      </div>
    </Card>
  );
}
function NotificationPreferencesCard() {
  const { t } = useT();
  const [prefs, setPrefs] = useState<Record<string, { kind: string; email_enabled: boolean; in_app_enabled: boolean }> | null>(null);
  const [busy, setBusy] = useState("");
  useEffect(() => { api.getNotificationPreferences().then((r) => setPrefs(r.items)).catch(() => setPrefs({})); }, []);
  const KINDS: Record<string, string> = {
    order_paid: t("订单已支付", "Order paid"), order_settled: t("订单已结算", "Order settled"),
    order_disputed: t("订单纠纷", "Order dispute"), quality_done: t("质检完成", "Quality done"),
    dataset_updated: t("数据集有更新", "Dataset updated"),
  };
  async function toggle(kind: string, email: boolean, inApp: boolean) { setBusy(kind); try { await api.updateNotificationPreference(kind, email, inApp); } finally { setBusy(""); } }
  if (!prefs) return null;
  return (<Card><h2 className="mb-3 font-semibold">{t("通知偏好", "Notification preferences")}</h2>
    <div className="space-y-1">{Object.entries(KINDS).map(([kind, label]) => {
      const p = prefs[kind]; const email = !p || p.email_enabled; const inApp = !p || p.in_app_enabled;
      return (<div key={kind} className="flex items-center justify-between border rounded px-3 py-2 text-sm">
        <span>{label}</span>
        <label className="text-xs">{t("邮件", "Email")}<input type="checkbox" checked={email} disabled={busy===kind} onChange={() => toggle(kind, !email, inApp)} /></label>
        <label className="ml-3 text-xs">{t("站内", "In-app")}<input type="checkbox" checked={inApp} disabled={busy===kind} onChange={() => toggle(kind, email, !inApp)} /></label>
      </div>);
    })}</div></Card>);
}

function TwoFactorCard() {
  const { user, refresh } = useAuth();
  const { t } = useT();
  const enrolled = user?.totp_enabled ?? false;
  const [step, setStep] = useState<"enroll" | "verify">("enroll");
  const [secret, setSecret] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [code, setCode] = useState("");
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState(false);

  async function enroll() {
    setErr(""); setMsg(""); setBusy(true);
    try {
      const r = await api.enroll2FA();
      setSecret(r.secret);
      setRecoveryCodes(r.recovery_codes);
      setStep("verify");
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(false); }
  }

  async function verifyEnroll() {
    setErr(""); setBusy(true);
    try {
      await api.verify2FAEnrollment(code);
      setMsg(t("2FA 已启用", "2FA enabled"));
      setStep("enroll");
      await refresh();
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(false); }
  }

  async function disable() {
    if (!window.confirm(t("确认禁用 2FA?", "Disable 2FA?"))) return;
    setErr(""); setBusy(true);
    try {
      await api.disable2FA(code);
      setMsg(t("2FA 已禁用", "2FA disabled"));
      await refresh();
    } catch (e) { setErr((e as Error).message); }
    finally { setBusy(false); }
  }

  return (
    <Card>
      <h2 className="mb-2 font-semibold">{t("两步验证 (2FA)", "Two-factor auth (2FA)")}</h2>
      {err && <Alert>{err}</Alert>}
      {msg && <Alert kind="success">{msg}</Alert>}

      {enrolled ? (
        <div className="space-y-2">
          <p className="text-sm text-green-700">{t("已启用", "Enabled")}</p>
          <Field label={t("TOTP 验证码", "TOTP code")}>
            <Input value={code} onChange={(e) => setCode(e.target.value)} placeholder="123456" />
          </Field>
          <Button variant="danger" disabled={busy} onClick={disable}>{t("禁用 2FA", "Disable 2FA")}</Button>
        </div>
      ) : step === "enroll" ? (
        <Button disabled={busy} onClick={enroll}>{t("启用 2FA", "Enable 2FA")}</Button>
      ) : (
        <div className="space-y-3">
          <p className="text-sm text-neutral-600">
            {t("请用 Authenticator App 扫描或手动输入：", "Scan with Authenticator app or enter manually:")}
          </p>
          <code className="block rounded bg-neutral-100 p-2 text-xs break-all">{secret}</code>
          {recoveryCodes.length > 0 && (
            <div className="rounded border border-amber-200 bg-amber-50 p-2">
              <div className="text-xs font-medium text-amber-800 mb-1">{t("备份恢复码（请保存）：", "Recovery codes (save these):")}</div>
              <pre className="text-xs text-amber-700">{recoveryCodes.join("\n")}</pre>
            </div>
          )}
          <Field label={t("TOTP 验证码", "TOTP code")}>
            <Input value={code} onChange={(e) => setCode(e.target.value)} placeholder="123456" />
          </Field>
          <div className="flex gap-2">
            <Button disabled={busy} onClick={verifyEnroll}>{t("验证启用", "Verify & Enable")}</Button>
            <Button variant="secondary" onClick={() => { setStep("enroll"); setErr(""); }}>{t("取消", "Cancel")}</Button>
          </div>
        </div>
      )}
    </Card>
  );
}
