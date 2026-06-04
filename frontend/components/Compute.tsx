"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  api,
  yuan,
  type ComputeAlgorithm,
  type ComputeAttestation,
  type ComputeEntitlement,
  type ComputeJob,
  type ComputeOffer,
  type FederatedJob,
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
                  {offer.trust_level === "L2" && j.status === "released" && <AttestationChip jobId={j.id} />}
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
  const [allowFederated, setAllowFederated] = useState(false);
  const [allowPSI, setAllowPSI] = useState(false);
  const [trustLevel, setTrustLevel] = useState("L1");
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
        setAllowFederated(!!o.allow_federated);
        setAllowPSI(!!o.allow_psi);
        if (o.trust_level) setTrustLevel(o.trust_level);
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
        trust_level: trustLevel,
        price_cents: Math.round(parseFloat(priceYuan || "0") * 100),
        max_output_bytes: Math.max(1, Math.round(parseFloat(maxOutputMiB || "10"))) * (1 << 20),
        review_output: reviewOutput,
        allow_federated: allowFederated,
        allow_psi: allowPSI,
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
      <Field
        label={t("信任级别", "Trust level")}
        hint={
          trustLevel === "L2"
            ? t("L2 需平台以 TEE 运行时部署（COMPUTE_RUNNER=tee）", "L2 requires the platform deployed with a TEE runtime (COMPUTE_RUNNER=tee)")
            : t("L1：买方不可见、平台运营方仍可见", "L1: invisible to the buyer; the platform operator can still see the data")
        }
      >
        <Select value={trustLevel} onChange={(e) => setTrustLevel(e.target.value)}>
          <option value="L1">{t("L1 · 数据沙箱（买方不可见）", "L1 · Data sandbox (buyer-invisible)")}</option>
          <option value="L2">{t("L2 · 机密计算 / TEE（连平台也不可见）", "L2 · Confidential computing / TEE (platform-invisible)")}</option>
        </Select>
      </Field>
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
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={allowFederated} onChange={(e) => setAllowFederated(e.target.checked)} />
        {t(
          "允许联邦学习（L3 · 数据不出域）：本数据集可与其他方联合训练，只贡献模型参数，原始数据不出本沙箱",
          "Allow federated learning (L3 · data-stays-home): this dataset can co-train with others, contributing only model params — raw data never leaves its sandbox",
        )}
      </label>
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={allowPSI} onChange={(e) => setAllowPSI(e.target.checked)} />
        {t(
          "允许隐私求交（L3 · PSI）：本数据集可与其他方求交集（如共同名单）。注意：这与联邦学习是不同的授权——求交会暴露记录是否在交集中。",
          "Allow private set intersection (L3 · PSI): this dataset can be intersected with others (e.g. a shared list). Note: this is a separate consent from federated learning — PSI reveals whether records are in the overlap.",
        )}
      </label>
      {err && <Alert>{err}</Alert>}
      {saved && <Alert kind="success">{t("已保存沙箱售卖配置。", "Sandbox-sale settings saved.")}</Alert>}
      <Button onClick={save} disabled={busy}>
        {busy ? t("保存中…", "Saving…") : t("保存配置", "Save settings")}
      </Button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Buyer: federated (L3) compute across N datasets the buyer holds entitlements
// on. Each dataset trains locally in its own sandbox; only the FedAvg joint
// model is returned. Raw data never leaves a sandbox.
// ---------------------------------------------------------------------------
const FED_PAGE_SIZE = 10;

export function FederatedComputePanel() {
  const { user } = useAuth();
  const { t } = useT();
  const [ents, setEnts] = useState<ComputeEntitlement[]>([]);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [algos, setAlgos] = useState<ComputeAlgorithm[]>([]);
  const [algo, setAlgo] = useState("");
  const [minParticipants, setMinParticipants] = useState("");
  const [feds, setFeds] = useState<FederatedJob[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [dsNames, setDsNames] = useState<Map<string, string>>(new Map());
  const [expandedFed, setExpandedFed] = useState<string | null>(null);
  const [subJobs, setSubJobs] = useState<ComputeJob[]>([]);
  const [loadingSubs, setLoadingSubs] = useState(false);

  const dsName = useCallback((id: string) => dsNames.get(id) || id.slice(0, 8), [dsNames]);

  const refreshFeds = useCallback(async () => {
    if (!user) return;
    try {
      const r = await api.listMyFederatedJobs(FED_PAGE_SIZE, 0);
      // PSI jobs share the federated endpoints but have their own panel.
      setFeds(r.items.filter((f) => f.mode !== "psi"));
      setHasMore(r.items.length >= FED_PAGE_SIZE);
    } catch {
      /* ignore */
    }
  }, [user]);

  // Resolve dataset UUIDs → human-readable titles.
  const resolveNames = useCallback(async (ids: string[]) => {
    const unique = [...new Set(ids)];
    const results = await Promise.allSettled(unique.map((id) => api.getDataset(id)));
    setDsNames((prev) => {
      const next = new Map(prev);
      results.forEach((r, i) => {
        if (r.status === "fulfilled" && r.value.title) next.set(unique[i], r.value.title);
      });
      return next;
    });
  }, []);

  useEffect(() => {
    if (!user) return;
    api
      .listMyComputeEntitlements()
      .then((r) => {
        const active = r.items.filter((e) => e.status === "active" && e.jobs_used < e.jobs_quota);
        setEnts(active);
        void resolveNames(active.map((e) => e.dataset_id));
      })
      .catch(() => {});
    void refreshFeds();
  }, [user, refreshFeds, resolveNames]);

  // Resolve names for federated job dataset_ids when feds change.
  useEffect(() => {
    const ids = feds.flatMap((f) => f.dataset_ids);
    if (ids.length > 0) void resolveNames(ids);
  }, [feds, resolveNames]);

  useEffect(() => {
    const first = [...picked][0];
    if (!first) {
      setAlgos([]);
      return;
    }
    api
      .listComputeAlgorithms(first)
      .then((r) => {
        const fed = r.items.filter((a) => a.runtime === "fed-logreg");
        const list = fed.length ? fed : r.items;
        setAlgos(list);
        setAlgo((prev) => prev || list[0]?.id || "");
      })
      .catch(() => setAlgos([]));
  }, [picked]);

  const hasPending = feds.some((f) => !TERMINAL.has(f.status));
  useEffect(() => {
    if (!hasPending) return;
    const iv = setInterval(() => void refreshFeds(), 1800);
    return () => clearInterval(iv);
  }, [hasPending, refreshFeds]);

  if (!user) return null;

  function toggle(dsId: string) {
    setPicked((prev) => {
      const next = new Set(prev);
      if (next.has(dsId)) next.delete(dsId);
      else next.add(dsId);
      return next;
    });
  }

  async function submit() {
    setErr("");
    setBusy(true);
    try {
      const ds = [...picked];
      const min = parseInt(minParticipants || "0", 10);
      await api.submitFederatedJob({
        algorithm_id: algo,
        dataset_ids: ds,
        min_participants: Number.isFinite(min) && min > 0 ? min : undefined,
      });
      setPicked(new Set());
      setMinParticipants("");
      await refreshFeds();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function loadMore() {
    setLoadingMore(true);
    try {
      const r = await api.listMyFederatedJobs(FED_PAGE_SIZE, feds.length);
      setFeds((prev) => [...prev, ...r.items]);
      setHasMore(r.items.length >= FED_PAGE_SIZE);
    } catch {
      /* ignore */
    } finally {
      setLoadingMore(false);
    }
  }

  async function toggleExpand(fedId: string) {
    if (expandedFed === fedId) {
      setExpandedFed(null);
      setSubJobs([]);
      return;
    }
    setExpandedFed(fedId);
    setSubJobs([]);
    setLoadingSubs(true);
    try {
      const r = await api.getFederatedJob(fedId);
      setSubJobs(r.sub_jobs);
    } catch {
      /* ignore */
    } finally {
      setLoadingSubs(false);
    }
  }

  const canSubmit = picked.size >= 2 && !!algo && !busy;

  return (
    <Card>
      <div className="flex items-center gap-2">
        <h2 className="text-lg font-semibold">{t("联邦计算 · 数据不出域", "Federated compute · data-stays-home")}</h2>
        <Badge>L3</Badge>
      </div>
      <p className="mt-1 text-sm text-neutral-500">
        {t(
          "选择你已购计算权益的 ≥2 个数据集联合训练：每个数据集只在自己的沙箱内本地训练，平台仅聚合模型参数（FedAvg），原始数据互不可见、不出域。",
          "Pick ≥2 datasets you hold entitlements on to co-train: each trains locally in its own sandbox; the platform only aggregates model params (FedAvg). Raw data stays in each domain, invisible to others.",
        )}
      </p>

      {err && (
        <div className="mt-3">
          <Alert>{err}</Alert>
        </div>
      )}

      {ents.length < 2 ? (
        <div className="mt-3">
          <Alert kind="info">
            {t(
              "需要至少 2 个「已购计算权益」的数据集才能发起联邦作业。请先在数据集页购买计算权益（卖家需开启「允许联邦学习」）。",
              "You need active compute entitlements on at least 2 datasets. Buy compute on dataset pages first (the seller must enable “Allow federated learning”).",
            )}
          </Alert>
        </div>
      ) : (
        <div className="mt-4 space-y-3 rounded-lg border border-neutral-200 p-3">
          <div className="text-xs font-medium text-neutral-600">{t("参与数据集", "Participating datasets")}</div>
          <ul className="space-y-1">
            {ents.map((e) => (
              <li key={e.dataset_id}>
                <label className="flex items-center gap-2 text-sm">
                  <input type="checkbox" checked={picked.has(e.dataset_id)} onChange={() => toggle(e.dataset_id)} />
                  <span className="text-sm text-neutral-700">{dsName(e.dataset_id)}</span>
                  <span className="text-xs text-neutral-400">
                    {t(`剩余 ${e.jobs_quota - e.jobs_used} 次`, `${e.jobs_quota - e.jobs_used} runs left`)}
                  </span>
                </label>
              </li>
            ))}
          </ul>
          <div className="grid grid-cols-2 gap-3">
            <Field label={t("联邦算法", "Federated algorithm")}>
              <Select value={algo} onChange={(e) => setAlgo(e.target.value)}>
                {algos.length === 0 && <option value="">{t("（选数据集后加载）", "(pick a dataset)")}</option>}
                {algos.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name} · {a.runtime}
                    {a.trusted ? t(" · 可信", " · trusted") : ""}
                  </option>
                ))}
              </Select>
            </Field>
            <Field
              label={t("最少参与方", "Min participants")}
              hint={t("留空=全部；可容忍掉队", "blank = all; tolerates dropouts")}
            >
              <Input
                value={minParticipants}
                onChange={(e) => setMinParticipants(e.target.value)}
                inputMode="numeric"
                placeholder={String(picked.size || "")}
              />
            </Field>
          </div>
          <Button className="w-full" onClick={submit} disabled={!canSubmit}>
            {busy
              ? t("提交中…", "Submitting…")
              : picked.size < 2
                ? t("至少选择 2 个数据集", "Select at least 2 datasets")
                : t(`发起联邦作业（${picked.size} 方）`, `Start federated job (${picked.size} parties)`)}
          </Button>
        </div>
      )}

      {feds.length > 0 && (
        <div className="mt-4 border-t border-neutral-100 pt-3">
          <div className="mb-2 text-sm font-medium text-neutral-700">{t("我的联邦作业", "My federated jobs")}</div>
          <ul className="space-y-2">
            {feds.map((f) => (
              <li key={f.id}>
                <div
                  className="flex cursor-pointer items-center justify-between gap-2 rounded-md p-1.5 text-sm hover:bg-neutral-50"
                  onClick={() => void toggleExpand(f.id)}
                >
                  <div className="min-w-0">
                    <span className="font-mono text-xs text-neutral-400">{f.id.slice(0, 8)}</span>{" "}
                    <Badge>{f.status}</Badge>{" "}
                    <span className="text-xs text-neutral-400">
                      {t(`${f.dataset_ids.length} 方 · 最少 ${f.min_participants}`, `${f.dataset_ids.length} parties · min ${f.min_participants}`)}
                      {f.dp_epsilon ? t(` · DP ε=${f.dp_epsilon}`, ` · DP ε=${f.dp_epsilon}`) : ""}
                    </span>
                    {f.status === "failed" && f.failure_code && (
                      <span className="ml-1 text-xs text-red-500">{f.failure_code}</span>
                    )}
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    {f.status === "released" ? (
                      <Button
                        variant="secondary"
                        onClick={(e) => {
                          e.stopPropagation();
                          void api.downloadFederatedOutput(f.id).catch((err) => setErr((err as Error).message));
                        }}
                      >
                        {t("下载联合模型", "Download joint model")}
                      </Button>
                    ) : TERMINAL.has(f.status) ? (
                      <span className="text-xs text-neutral-400">{"—"}</span>
                    ) : (
                      <span className="text-xs text-neutral-400">{t("运行中…", "Running…")}</span>
                    )}
                    <span className="text-xs text-neutral-300">{expandedFed === f.id ? "▲" : "▼"}</span>
                  </div>
                </div>
                {expandedFed === f.id && (
                  <div className="ml-4 mt-1 space-y-1 border-l-2 border-neutral-100 pl-3">
                    {loadingSubs ? (
                      <p className="text-xs text-neutral-400">{t("加载子作业…", "Loading sub-jobs…")}</p>
                    ) : subJobs.length === 0 ? (
                      <p className="text-xs text-neutral-400">{t("暂无子作业", "No sub-jobs")}</p>
                    ) : (
                      subJobs.map((sj) => (
                        <div key={sj.id} className="flex items-center gap-2 text-xs">
                          <span className="text-neutral-700">{dsName(sj.dataset_id)}</span>
                          <Badge>{sj.status}</Badge>
                          {sj.created_at && <span className="text-neutral-400">{sj.created_at.slice(0, 16)}</span>}
                          {sj.error && <span className="text-red-500">{sj.error}</span>}
                        </div>
                      ))
                    )}
                  </div>
                )}
              </li>
            ))}
          </ul>
          {hasMore && (
            <Button variant="secondary" className="mt-2 w-full" onClick={() => void loadMore()} disabled={loadingMore}>
              {loadingMore ? t("加载中…", "Loading…") : t("加载更多", "Load more")}
            </Button>
          )}
        </div>
      )}

      <p className="mt-3 text-xs text-neutral-400">
        {t(
          "诚实标注：当前为中心化 FedAvg——平台聚合时可见各方模型参数（非原始数据）。安全聚合（掩码求和）与真 TEE 在规划中；联合模型仍可能经参数泄漏，可叠加差分隐私（DP）。",
          "Honest note: this is central FedAvg — the platform sees each party's model params (not raw data) during aggregation. Secure aggregation and real TEE are planned; the joint model can still leak via params, so differential privacy (DP) can be layered on.",
        )}
      </p>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Buyer: private set intersection (PSI, Direction D) across N datasets the buyer
// holds entitlements on. Each dataset contributes its set inside its own sandbox;
// only the intersection is returned. A PSI job uses the federated endpoints with
// a psi-extract algorithm (mode "psi").
// ---------------------------------------------------------------------------
type PSIResultData = { intersection: string[]; cardinality: number; participants: number };

export function PSIComputePanel() {
  const { user } = useAuth();
  const { t } = useT();
  const [ents, setEnts] = useState<ComputeEntitlement[]>([]);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [algos, setAlgos] = useState<ComputeAlgorithm[]>([]);
  const [algo, setAlgo] = useState("");
  const [jobs, setJobs] = useState<FederatedJob[]>([]);
  const [results, setResults] = useState<Map<string, PSIResultData>>(new Map());
  const [dsNames, setDsNames] = useState<Map<string, string>>(new Map());
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const dsName = useCallback((id: string) => dsNames.get(id) || id.slice(0, 8), [dsNames]);

  const refreshJobs = useCallback(async () => {
    if (!user) return;
    try {
      const r = await api.listMyFederatedJobs(50, 0);
      setJobs(r.items.filter((f) => f.mode === "psi"));
    } catch {
      /* ignore */
    }
  }, [user]);

  const resolveNames = useCallback(async (ids: string[]) => {
    const unique = [...new Set(ids)];
    const settled = await Promise.allSettled(unique.map((id) => api.getDataset(id)));
    setDsNames((prev) => {
      const next = new Map(prev);
      settled.forEach((r, i) => {
        if (r.status === "fulfilled" && r.value.title) next.set(unique[i], r.value.title);
      });
      return next;
    });
  }, []);

  useEffect(() => {
    if (!user) return;
    api
      .listMyComputeEntitlements()
      .then((r) => {
        const active = r.items.filter((e) => e.status === "active" && e.jobs_used < e.jobs_quota);
        setEnts(active);
        void resolveNames(active.map((e) => e.dataset_id));
      })
      .catch(() => {});
    void refreshJobs();
  }, [user, refreshJobs, resolveNames]);

  // PSI algorithms (runtime psi-extract) come from any picked dataset.
  useEffect(() => {
    const first = [...picked][0];
    if (!first) {
      setAlgos([]);
      return;
    }
    api
      .listComputeAlgorithms(first)
      .then((r) => {
        const psi = r.items.filter((a) => a.runtime === "psi-extract");
        setAlgos(psi);
        setAlgo((prev) => prev || psi[0]?.id || "");
      })
      .catch(() => setAlgos([]));
  }, [picked]);

  const hasPending = jobs.some((j) => !TERMINAL.has(j.status));
  useEffect(() => {
    if (!hasPending) return;
    const iv = setInterval(() => void refreshJobs(), 1800);
    return () => clearInterval(iv);
  }, [hasPending, refreshJobs]);

  if (!user) return null;

  function toggle(dsId: string) {
    setPicked((prev) => {
      const next = new Set(prev);
      if (next.has(dsId)) next.delete(dsId);
      else next.add(dsId);
      return next;
    });
  }

  async function submit() {
    setErr("");
    setBusy(true);
    try {
      await api.submitFederatedJob({ algorithm_id: algo, dataset_ids: [...picked] });
      setPicked(new Set());
      await refreshJobs();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function viewResult(id: string) {
    setErr("");
    try {
      const r = await api.getFederatedOutputJSON<PSIResultData>(id);
      setResults((prev) => new Map(prev).set(id, r));
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  const canSubmit = picked.size >= 2 && !!algo && !busy;

  return (
    <Card>
      <div className="flex items-center gap-2">
        <h2 className="text-lg font-semibold">{t("隐私求交 · PSI", "Private set intersection · PSI")}</h2>
        <Badge>L3</Badge>
      </div>
      <p className="mt-1 text-sm text-neutral-500">
        {t(
          "选择你已购计算权益的 ≥2 个数据集，计算它们的交集（如共同名单）：每个数据集只在自己的沙箱内贡献集合，你只获得交集，不获得任何一方的非交集成员。",
          "Pick ≥2 datasets you hold entitlements on to compute their intersection (e.g. a shared list): each contributes its set inside its own sandbox; you get only the overlap, never any party's non-matching members.",
        )}
      </p>

      {err && (
        <div className="mt-3">
          <Alert>{err}</Alert>
        </div>
      )}

      {ents.length < 2 ? (
        <div className="mt-3">
          <Alert kind="info">
            {t(
              "需要至少 2 个「已购计算权益」的数据集才能发起求交作业。",
              "You need active compute entitlements on at least 2 datasets to run a PSI job.",
            )}
          </Alert>
        </div>
      ) : (
        <div className="mt-4 space-y-3 rounded-lg border border-neutral-200 p-3">
          <div className="text-xs font-medium text-neutral-600">{t("参与数据集", "Participating datasets")}</div>
          <ul className="space-y-1">
            {ents.map((e) => (
              <li key={e.dataset_id}>
                <label className="flex items-center gap-2 text-sm">
                  <input type="checkbox" checked={picked.has(e.dataset_id)} onChange={() => toggle(e.dataset_id)} />
                  <span className="text-sm text-neutral-700">{dsName(e.dataset_id)}</span>
                  <span className="text-xs text-neutral-400">
                    {t(`剩余 ${e.jobs_quota - e.jobs_used} 次`, `${e.jobs_quota - e.jobs_used} runs left`)}
                  </span>
                </label>
              </li>
            ))}
          </ul>
          <Field label={t("求交算法", "PSI algorithm")}>
            <Select value={algo} onChange={(e) => setAlgo(e.target.value)}>
              {algos.length === 0 && <option value="">{t("（选数据集后加载）", "(pick a dataset)")}</option>}
              {algos.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name} · {a.runtime}
                  {a.trusted ? t(" · 可信", " · trusted") : ""}
                </option>
              ))}
            </Select>
          </Field>
          <Button className="w-full" onClick={submit} disabled={!canSubmit}>
            {busy
              ? t("提交中…", "Submitting…")
              : picked.size < 2
                ? t("至少选择 2 个数据集", "Select at least 2 datasets")
                : t(`发起求交（${picked.size} 方）`, `Run PSI (${picked.size} parties)`)}
          </Button>
        </div>
      )}

      {jobs.length > 0 && (
        <div className="mt-4 border-t border-neutral-100 pt-3">
          <div className="mb-2 text-sm font-medium text-neutral-700">{t("我的求交作业", "My PSI jobs")}</div>
          <ul className="space-y-2">
            {jobs.map((j) => (
              <li key={j.id} className="text-sm">
                <div className="flex items-center justify-between gap-2">
                  <div className="min-w-0">
                    <span className="font-mono text-xs text-neutral-400">{j.id.slice(0, 8)}</span>{" "}
                    <Badge>{j.status}</Badge>{" "}
                    <span className="text-xs text-neutral-400">
                      {t(`${j.dataset_ids.length} 方`, `${j.dataset_ids.length} parties`)}
                    </span>
                    {j.status === "failed" && j.failure_code && (
                      <span className="ml-1 text-xs text-red-500">{j.failure_code}</span>
                    )}
                  </div>
                  {j.status === "released" ? (
                    <Button variant="secondary" onClick={() => void viewResult(j.id)}>
                      {t("查看交集", "View intersection")}
                    </Button>
                  ) : TERMINAL.has(j.status) ? (
                    <span className="text-xs text-neutral-400">{"—"}</span>
                  ) : (
                    <span className="text-xs text-neutral-400">{t("运行中…", "Running…")}</span>
                  )}
                </div>
                {results.has(j.id) && (
                  <div className="ml-4 mt-1 rounded-md border border-neutral-100 bg-neutral-50 p-2 text-xs">
                    <div className="font-medium text-neutral-600">
                      {t(`交集大小：${results.get(j.id)!.cardinality}`, `Intersection size: ${results.get(j.id)!.cardinality}`)}
                    </div>
                    {results.get(j.id)!.intersection.length > 0 && (
                      <div className="mt-1 break-words font-mono text-neutral-500">
                        {results.get(j.id)!.intersection.join(", ")}
                      </div>
                    )}
                  </div>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}

      <p className="mt-3 text-xs text-neutral-400">
        {t(
          "诚实标注：当前为 Mock 编排——平台在求交时可见各方集合（非密码学私密）。真隐私求交（Secretflow/SPU，平台只做编排、不持明文）在规划中。",
          "Honest note: this is a mock orchestrator — the platform sees each party's set during intersection (not cryptographically private). Real PSI (Secretflow/SPU, platform orchestrates without holding plaintext) is planned.",
        )}
      </p>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// L2 remote-attestation chip: shows whether the platform-verified attestation
// for a released confidential-compute job checks out (design P3).
// ---------------------------------------------------------------------------
function AttestationChip({ jobId }: { jobId: string }) {
  const { t } = useT();
  const [att, setAtt] = useState<ComputeAttestation | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let alive = true;
    api
      .getComputeAttestation(jobId)
      .then((a) => alive && setAtt(a))
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, [jobId]);

  if (failed || !att) return null;
  const ok = att.verified;
  return (
    <span
      title={`${t("度量值", "measurement")}: ${att.measurement} · ${t("证明者", "signer")}: ${att.signer}`}
      className={`ml-1 rounded-full px-2 py-0.5 text-[11px] ${
        ok ? "bg-emerald-50 text-emerald-700" : "bg-red-50 text-red-700"
      }`}
    >
      {ok ? t("🔒 机密计算·已验证", "🔒 Confidential · attested") : t("⚠ 证明未通过", "⚠ attestation failed")}
    </span>
  );
}
