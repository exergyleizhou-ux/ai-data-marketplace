"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  api,
  yuan,
  type ComputeAlgorithm,
  type ComputeEntitlement,
  type ComputeJob,
  type ComputeOffer,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useT } from "@/lib/i18n";
import { Alert, Badge, Button, Card, Field, Input, Select } from "@/components/ui";

const TERMINAL = new Set(["released", "failed", "rejected", "canceled"]);

// ---------------------------------------------------------------------------
// Buyer: purchase a compute entitlement, run a whitelisted algorithm in the
// sandbox, get the OUTPUT (model/metrics) — never the raw data.
// ---------------------------------------------------------------------------
export function ComputeBuyer({ datasetId, sellerId }: { datasetId: string; sellerId: string }) {
  const { user } = useAuth();
  const { t } = useT();
  const router = useRouter();
  const [offer, setOffer] = useState<ComputeOffer | null | "none">(null);
  const [algos, setAlgos] = useState<ComputeAlgorithm[]>([]);
  const [selected, setSelected] = useState("");
  const [ent, setEnt] = useState<ComputeEntitlement | null>(null);
  const [jobs, setJobs] = useState<ComputeJob[]>([]);
  const [busy, setBusy] = useState("");
  const [err, setErr] = useState("");

  const isSeller = !!user && user.id === sellerId;

  const refreshJobs = useCallback(async () => {
    if (!user) return;
    try {
      const all = await api.listMyComputeJobs();
      setJobs(all.items.filter((j) => j.dataset_id === datasetId));
    } catch {
      /* ignore */
    }
  }, [user, datasetId]);

  // The buyer's active entitlement for this dataset (server is the source of
  // truth — granted when the compute order is paid).
  const refreshEnt = useCallback(async () => {
    if (!user) return;
    try {
      const all = await api.listMyComputeEntitlements();
      const active = all.items.find(
        (e) => e.dataset_id === datasetId && e.status === "active" && e.jobs_used < e.jobs_quota,
      );
      setEnt(active ?? null);
    } catch {
      /* ignore */
    }
  }, [user, datasetId]);

  // Load the offer + (if enabled) the algorithm list. Restore any entitlement.
  useEffect(() => {
    let alive = true;
    api
      .getComputeOffer(datasetId)
      .then((o) => {
        if (!alive) return;
        if (!o.enabled) {
          setOffer("none");
          return;
        }
        setOffer(o);
        api.listComputeAlgorithms(datasetId).then((r) => {
          if (!alive) return;
          setAlgos(r.items);
          if (r.items[0]) setSelected(r.items[0].id);
        }).catch(() => {});
      })
      .catch(() => alive && setOffer("none"));
    return () => {
      alive = false;
    };
  }, [datasetId]);

  useEffect(() => {
    if (user) {
      void refreshJobs();
      void refreshEnt();
    }
  }, [user, refreshJobs, refreshEnt]);

  // Poll while any job is still in flight.
  const hasPending = jobs.some((j) => !TERMINAL.has(j.status));
  useEffect(() => {
    if (!hasPending) return;
    const t = setInterval(() => void refreshJobs(), 1500);
    return () => clearInterval(t);
  }, [hasPending, refreshJobs]);

  // Real purchase: create a compute order and go to the payment page. After
  // paying, the entitlement is granted server-side and appears via refreshEnt.
  async function purchase() {
    setErr("");
    setBusy("purchase");
    try {
      const { order_id } = await api.createComputeOrder(datasetId);
      router.push(`/orders/${order_id}`);
    } catch (e) {
      setErr((e as Error).message);
      setBusy("");
    }
  }

  async function submit() {
    if (!ent || !selected) return;
    setErr("");
    setBusy("submit");
    try {
      await api.submitComputeJob({ dataset_id: datasetId, entitlement_id: ent.id, algorithm_id: selected });
      await refreshJobs();
      await refreshEnt();
    } catch (e) {
      setErr((e as Error).message);
      await refreshEnt();
    } finally {
      setBusy("");
    }
  }

  async function download(jobId: string) {
    setErr("");
    try {
      await api.downloadComputeOutput(jobId);
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  if (offer === null || offer === "none") return null; // loading or not offered

  const remaining = ent ? Math.max(ent.jobs_quota - ent.jobs_used, 0) : 0;
  const trustText =
    offer.trust_level === "L1"
      ? t("L1 · 数据沙箱（买方不可见）", "L1 · Data sandbox (invisible to the buyer)")
      : offer.trust_level === "L2"
        ? t("L2 · 机密计算（连平台也不可见）", "L2 · Confidential computing (invisible to the platform too)")
        : offer.trust_level === "L3"
          ? t("L3 · 数据不出域", "L3 · Data never leaves its domain")
          : offer.trust_level;

  return (
    <Card className="border-emerald-200">
      <div className="flex items-center gap-2">
        <span className="text-lg font-semibold">{t("可用不可见 · 沙箱计算", "Available-but-Invisible · Sandbox Compute")}</span>
        <Badge>{offer.trust_level}</Badge>
      </div>
      <p className="mt-1 text-sm text-neutral-500">
        {t(
          "在平台沙箱内对本数据集运行经审核的算法，你获得计算结果（模型 / 指标），而不获得原始数据。",
          "Run a platform-reviewed algorithm against this dataset inside the sandbox. You receive the computation output (model / metrics) — never the raw data.",
        )}
      </p>
      <p className="mt-1 text-xs text-neutral-400">{trustText}</p>

      <div className="mt-3 text-2xl font-semibold">{yuan(offer.price_cents)}</div>
      <div className="text-xs text-neutral-400">
        {t("每份计算权益（含若干次作业额度）", "Per compute entitlement (includes several job credits)")}
      </div>

      {err && (
        <div className="mt-3">
          <Alert>{err}</Alert>
        </div>
      )}

      <div className="mt-4 space-y-3">
        {isSeller ? (
          <Alert kind="info">{t("这是你的数据集。", "This is your own dataset.")}</Alert>
        ) : !user ? (
          <Button className="w-full" onClick={() => router.push("/login")}>
            {t("登录后购买计算权益", "Sign in to buy a compute entitlement")}
          </Button>
        ) : user.kyc_status !== "verified" ? (
          <Button className="w-full" onClick={() => router.push("/account")}>
            {t("需先实名认证", "Real-name verification required")}
          </Button>
        ) : !ent ? (
          <Button className="w-full" onClick={purchase} disabled={busy === "purchase"}>
            {busy === "purchase"
              ? t("前往支付…", "Going to payment…")
              : t(`购买计算权益（${yuan(offer.price_cents)}）`, `Buy compute entitlement (${yuan(offer.price_cents)})`)}
          </Button>
        ) : (
          <div className="space-y-2 rounded-lg border border-neutral-200 p-3">
            <div className="text-xs text-neutral-500">
              {t(
                `已购计算权益 · 剩余约 ${remaining} / ${ent.jobs_quota} 次`,
                `Entitlement active · ~${remaining} / ${ent.jobs_quota} runs left`,
              )}
            </div>
            {algos.length === 0 ? (
              <p className="text-xs text-neutral-400">
                {t("该数据集暂无可用的已审核算法。", "No approved algorithms are available for this dataset yet.")}
              </p>
            ) : (
              <>
                <Field label={t("选择算法", "Choose an algorithm")}>
                  <Select value={selected} onChange={(e) => setSelected(e.target.value)}>
                    {algos.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.name} · {a.output_kind}
                        {a.trusted ? t(" · 可信", " · trusted") : ""}
                      </option>
                    ))}
                  </Select>
                </Field>
                <Button className="w-full" onClick={submit} disabled={busy === "submit" || remaining <= 0 || !selected}>
                  {busy === "submit"
                    ? t("提交中…", "Submitting…")
                    : remaining <= 0
                      ? t("额度已用尽（请重新购买）", "No credits left (buy again)")
                      : t("提交计算作业", "Submit compute job")}
                </Button>
              </>
            )}
          </div>
        )}
      </div>

      {jobs.length > 0 && (
        <div className="mt-4 border-t border-neutral-100 pt-3">
          <div className="mb-2 text-sm font-medium text-neutral-700">{t("我的计算作业", "My compute jobs")}</div>
          <ul className="space-y-2">
            {jobs.map((j) => (
              <li key={j.id} className="flex items-center justify-between gap-2 text-sm">
                <div className="min-w-0">
                  <span className="font-mono text-xs text-neutral-400">{j.id.slice(0, 8)}</span>{" "}
                  <Badge>{j.status}</Badge>
                  {j.error && <span className="ml-1 text-xs text-red-500">{j.error}</span>}
                </div>
                {j.status === "released" ? (
                  <Button variant="secondary" onClick={() => void download(j.id)}>
                    {t("下载输出", "Download output")}
                  </Button>
                ) : TERMINAL.has(j.status) ? (
                  <span className="text-xs text-neutral-400">—</span>
                ) : j.status === "output_reviewing" ? (
                  <span className="text-xs text-amber-600">{t("运营审核中…", "Under review…")}</span>
                ) : (
                  <span className="text-xs text-neutral-400">{t("运行中…", "Running…")}</span>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}

      <p className="mt-3 text-xs text-neutral-400">
        {t(
          "诚实标注：L1 为买方可用不可见——平台运营方仍可访问数据。需「连平台也不可见」请选 L2（机密计算 / TEE，规划中）。",
          "Honest note: L1 is buyer-invisible — the platform operator can still access the data. For platform-invisible, choose L2 (confidential computing / TEE, planned).",
        )}
      </p>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Seller: enable & configure the sandbox-sale offer for a dataset.
// ---------------------------------------------------------------------------
export function ComputeOfferEditor({ datasetId }: { datasetId: string }) {
  const { t } = useT();
  const [loaded, setLoaded] = useState(false);
  const [enabled, setEnabled] = useState(false);
  const [priceYuan, setPriceYuan] = useState("10.00");
  const [maxOutputMiB, setMaxOutputMiB] = useState("10");
  const [reviewOutput, setReviewOutput] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [saved, setSaved] = useState(false);
  const initialized = useRef(false);

  useEffect(() => {
    if (initialized.current) return;
    initialized.current = true;
    api
      .getComputeOffer(datasetId)
      .then((o) => {
        setEnabled(o.enabled);
        if (o.price_cents) setPriceYuan((o.price_cents / 100).toFixed(2));
        if (o.max_output_bytes) setMaxOutputMiB(String(Math.round(o.max_output_bytes / (1 << 20))));
        setReviewOutput(o.review_output);
      })
      .catch(() => {
        /* no offer yet — keep defaults */
      })
      .finally(() => setLoaded(true));
  }, [datasetId]);

  async function save() {
    setErr("");
    setSaved(false);
    setBusy(true);
    try {
      await api.putComputeOffer(datasetId, {
        enabled,
        trust_level: "L1",
        price_cents: Math.round(parseFloat(priceYuan || "0") * 100),
        max_output_bytes: Math.max(1, Math.round(parseFloat(maxOutputMiB || "10"))) * (1 << 20),
        review_output: reviewOutput,
      });
      setSaved(true);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  if (!loaded)
    return <p className="text-xs text-neutral-400">{t("加载沙箱售卖配置…", "Loading sandbox-sale settings…")}</p>;

  return (
    <div className="space-y-3">
      <p className="text-sm text-neutral-600">
        {t(
          "开启「可用不可见」售卖：买方在沙箱内对本数据集运行经平台审核的算法，只取走计算结果，不获得原始数据（L1 信任级别）。",
          "Enable available-but-invisible sale: buyers run platform-reviewed algorithms against this dataset in the sandbox and take away only the result, not the raw data (L1 trust level).",
        )}
      </p>
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
        {t("开启沙箱计算售卖", "Enable sandbox-compute sale")}
      </label>
      <div className="grid grid-cols-2 gap-3">
        <Field label={t("计算权益单价（元）", "Price per entitlement (CNY)")}>
          <Input value={priceYuan} onChange={(e) => setPriceYuan(e.target.value)} inputMode="decimal" />
        </Field>
        <Field label={t("输出上限（MiB）", "Output cap (MiB)")} hint={t("防止把整库塞进输出", "Stops dumping the whole dataset into the output")}>
          <Input value={maxOutputMiB} onChange={(e) => setMaxOutputMiB(e.target.value)} inputMode="numeric" />
        </Field>
      </div>
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={reviewOutput} onChange={(e) => setReviewOutput(e.target.checked)} />
        {t("放行前需运营人工复核输出（高敏感数据建议开启）", "Require ops human review of output before release (recommended for sensitive data)")}
      </label>
      {err && <Alert>{err}</Alert>}
      {saved && <Alert kind="success">{t("已保存沙箱售卖配置。", "Sandbox-sale settings saved.")}</Alert>}
      <Button onClick={save} disabled={busy}>
        {busy ? t("保存中…", "Saving…") : t("保存配置", "Save settings")}
      </Button>
    </div>
  );
}
