"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
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
import { useBackoffPoll } from "@/lib/usePoll";
import { Alert, Badge, Button, Card, Field, Input, Select } from "@/components/ui";
import { ComputeCertificateModal } from "@/components/ComputeCertificate";
import { Reveal } from "@/components/Reveal";

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
  useBackoffPoll(hasPending, () => void refreshJobs());

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
                  <div className="flex items-center gap-2">
                    <JobCertificate jobId={j.id} />
                    <Button variant="secondary" onClick={() => void download(j.id)}>
                      {t("下载输出", "Download output")}
                    </Button>
                  </div>
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
  // Raw items fetched from the shared fed+psi endpoint so far. Pagination
  // offsets are over the RAW result set, not the psi-filtered `feds` list — so
  // load-more must advance by this count, never by feds.length.
  const fedRawCountRef = useRef(0);
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
      fedRawCountRef.current = r.items.length;
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
  useBackoffPoll(hasPending, () => void refreshFeds());

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
      // Offset is over the RAW shared result set (fed + psi). Using feds.length
      // (psi-filtered, so smaller when psi jobs are interleaved) would request a
      // window overlapping rows already shown → duplicate rows / duplicate keys.
      const r = await api.listMyFederatedJobs(FED_PAGE_SIZE, fedRawCountRef.current);
      fedRawCountRef.current += r.items.length;
      setFeds((prev) => {
        // Drop psi (it has its own panel) and dedup by id as a belt-and-suspenders
        // guard against rows shifting between the refresh and this fetch.
        const seen = new Set(prev.map((f) => f.id));
        return [...prev, ...r.items.filter((f) => f.mode !== "psi" && !seen.has(f.id))];
      });
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
                  <div className="flex shrink-0 items-center gap-2" onClick={(e) => e.stopPropagation()}>
                    {f.status === "released" ? (
                      <>
                        <JobCertificate jobId={f.id} federated />
                        <Button
                          variant="secondary"
                          onClick={() => void api.downloadFederatedOutput(f.id).catch((err) => setErr((err as Error).message))}
                        >
                          {t("下载联合模型", "Download joint model")}
                        </Button>
                      </>
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
  useBackoffPoll(hasPending, () => void refreshJobs());

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
                    <div className="flex items-center gap-2">
                      <JobCertificate jobId={j.id} federated />
                      <Button variant="secondary" onClick={() => void viewResult(j.id)}>
                        {t("查看交集", "View intersection")}
                      </Button>
                    </div>
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
// Hub: the buyer's active compute entitlements across all datasets (算力权益).
// ---------------------------------------------------------------------------
function useDatasetNames() {
  const [names, setNames] = useState<Map<string, string>>(new Map());
  const resolve = useCallback(async (ids: string[]) => {
    const unique = [...new Set(ids)];
    const settled = await Promise.allSettled(unique.map((id) => api.getDataset(id)));
    setNames((prev) => {
      const next = new Map(prev);
      settled.forEach((r, i) => {
        if (r.status === "fulfilled" && r.value.title) next.set(unique[i], r.value.title);
      });
      return next;
    });
  }, []);
  const name = useCallback((id: string) => names.get(id) || id.slice(0, 8), [names]);
  return { name, resolve };
}

// ComputeFunnelCTA turns the buyer's empty state into a guided path into the
// signature sandbox-compute flow instead of a dead end.
function ComputeFunnelCTA({ message }: { message: string }) {
  const { t } = useT();
  return (
    <div className="rounded-lg border border-emerald-200 bg-emerald-50 p-5">
      <p className="text-sm text-emerald-900">{message}</p>
      <ol className="mt-3 space-y-1 text-xs text-emerald-800">
        <li>{t("1 · 在数据市场挑一个支持「沙箱计算」的数据集", "1 · Pick a compute-enabled dataset in the marketplace")}</li>
        <li>{t("2 · 在详情页购买计算权益(含若干次作业额度)", "2 · Buy a compute entitlement on its page (includes job credits)")}</li>
        <li>{t("3 · 选算法发起计算 / 联邦 / PSI,只取走结果、不碰原始数据", "3 · Pick an algorithm and run compute / federated / PSI — take only the result")}</li>
      </ol>
      <div className="mt-4 flex flex-wrap gap-2">
        <Link href="/datasets" className="rounded-md bg-emerald-700 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-800">
          {t("去数据市场", "Browse the marketplace")}
        </Link>
        <Link href="/trust" className="rounded-md border border-emerald-300 bg-white px-4 py-2 text-sm font-medium text-emerald-800 hover:bg-emerald-100">
          {t("它如何保护数据", "How it protects the data")}
        </Link>
      </div>
    </div>
  );
}

export function MyEntitlementsPanel() {
  const { user } = useAuth();
  const { t } = useT();
  const [ents, setEnts] = useState<ComputeEntitlement[] | null>(null);
  const { name, resolve } = useDatasetNames();

  useEffect(() => {
    if (!user) return;
    api
      .listMyComputeEntitlements()
      .then((r) => {
        const active = r.items.filter((e) => e.status === "active");
        setEnts(active);
        void resolve(active.map((e) => e.dataset_id));
      })
      .catch(() => setEnts([]));
  }, [user, resolve]);

  return (
    <Card>
      <h2 className="text-lg font-semibold">{t("我的算力权益", "My compute entitlements")}</h2>
      <p className="mt-1 text-sm text-neutral-500">
        {t(
          "你购买的每份计算权益含若干次作业额度。额度用尽后可在数据集页重新购买。",
          "Each entitlement you buy includes several job credits. Buy again on the dataset page when used up.",
        )}
      </p>
      {ents === null ? (
        <p className="mt-3 text-sm text-neutral-400">{t("加载中…", "Loading…")}</p>
      ) : ents.length === 0 ? (
        <ComputeFunnelCTA
          message={t(
            "你还没有任何计算权益。买一份就能在平台沙箱里对数据跑算法,只取走结果——原始数据拿不到、也不出域。",
            "You have no compute entitlements yet. Buy one to run algorithms against data inside the platform sandbox and take only the result — you never get the raw data.",
          )}
        />
      ) : (
        <ul className="mt-3 divide-y divide-neutral-100">
          {ents.map((e) => {
            const remaining = Math.max(e.jobs_quota - e.jobs_used, 0);
            return (
              <li key={e.id} className="flex items-center justify-between gap-2 py-2 text-sm">
                <Link href={`/datasets/${e.dataset_id}`} className="truncate font-medium text-neutral-800 hover:underline">
                  {name(e.dataset_id)}
                </Link>
                <span className="shrink-0 text-xs text-neutral-400">
                  {t(`剩余 ${remaining} / ${e.jobs_quota} 次`, `${remaining} / ${e.jobs_quota} runs left`)}
                </span>
              </li>
            );
          })}
        </ul>
      )}
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Hub: all of the buyer's regular sandbox-compute jobs across every dataset
// (计算作业). One place to track status, download outputs, and view provenance.
// ---------------------------------------------------------------------------
export function MyComputeJobsPanel() {
  const { user } = useAuth();
  const { t } = useT();
  const [jobs, setJobs] = useState<ComputeJob[] | null>(null);
  const [err, setErr] = useState("");
  const { name, resolve } = useDatasetNames();

  const refresh = useCallback(async () => {
    if (!user) return;
    try {
      const r = await api.listMyComputeJobs();
      setJobs(r.items);
      void resolve(r.items.map((j) => j.dataset_id));
    } catch {
      setJobs([]);
    }
  }, [user, resolve]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const hasPending = (jobs ?? []).some((j) => !TERMINAL.has(j.status));
  useBackoffPoll(hasPending, () => void refresh());

  async function download(jobId: string) {
    setErr("");
    try {
      await api.downloadComputeOutput(jobId);
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  return (
    <Card>
      <h2 className="font-display text-xl text-ink">{t("我的计算作业", "My compute jobs")}</h2>
      <p className="mt-1 text-sm text-ink/65">
        {t(
          "你在各数据集沙箱内发起的常规计算作业。完成后可下载结果(模型 / 指标)并查看存证。",
          "Regular sandbox-compute jobs you started across datasets. Download the result (model / metrics) and view its provenance once released.",
        )}
      </p>
      {err && (
        <div className="mt-3">
          <Alert>{err}</Alert>
        </div>
      )}
      {jobs === null ? (
        <div className="mt-4 space-y-2" aria-hidden>
          {[0, 1, 2, 3].map((i) => (
            <div key={i} className="skeleton h-9 w-full rounded-lg" />
          ))}
        </div>
      ) : jobs.length === 0 ? (
        <ComputeFunnelCTA
          message={t(
            "暂无计算作业。发起第一次「可用不可见」沙箱计算——你拿走模型 / 指标,数据方拿走收益,原始数据从不易手。",
            "No compute jobs yet. Run your first available-but-invisible sandbox computation — you take the model / metrics, the data owner earns, and the raw data never changes hands.",
          )}
        />
      ) : (
        <ul className="mt-4 divide-y divide-rule/70">
          {jobs.map((j, i) => (
            <Reveal as="li" key={j.id} delay={Math.min(i, 8) * 40} className="flex flex-wrap items-center justify-between gap-2 py-3 text-sm">
              <div className="min-w-0">
                <Link href={`/datasets/${j.dataset_id}`} className="rounded font-medium text-ink transition hover:text-forest-700 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ink">
                  {name(j.dataset_id)}
                </Link>{" "}
                <span className="font-mono text-xs text-muted">{j.id.slice(0, 8)}</span>{" "}
                <Badge>{j.status}</Badge>
                {j.error && <span className="ml-1 text-xs text-red-600">{j.error}</span>}
                {j.status === "released" && <AttestationChip jobId={j.id} />}
              </div>
              {j.status === "released" ? (
                <div className="flex shrink-0 items-center gap-2">
                  <JobCertificate jobId={j.id} />
                  <Button variant="secondary" onClick={() => void download(j.id)}>
                    {t("下载输出", "Download output")}
                  </Button>
                </div>
              ) : TERMINAL.has(j.status) ? (
                <span className="text-xs text-muted">—</span>
              ) : j.status === "output_reviewing" ? (
                <span className="text-xs text-amber-700">{t("运营审核中…", "Under review…")}</span>
              ) : (
                <span className="text-xs text-muted">{t("运行中…", "Running…")}</span>
              )}
            </Reveal>
          ))}
        </ul>
      )}
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Buyer/seller: submit a custom algorithm for ops review (算法申请). The request
// is forced pending + untrusted server-side and cannot run until ops approve it,
// so this safely grows algorithm supply without weakening the audit gate.
// ---------------------------------------------------------------------------
const ALGO_OUTPUT_KINDS = ["model", "metrics", "table", "aggregate"];

export function MyAlgorithmRequestsPanel() {
  const { user } = useAuth();
  const { t } = useT();
  const [mine, setMine] = useState<ComputeAlgorithm[] | null>(null);
  const [name, setName] = useState("");
  const [runtime, setRuntime] = useState("python-sklearn");
  const [image, setImage] = useState("");
  const [imageDigest, setImageDigest] = useState("");
  const [sourceRef, setSourceRef] = useState("");
  const [outputKind, setOutputKind] = useState("model");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [okMsg, setOkMsg] = useState("");

  const refresh = useCallback(async () => {
    if (!user) return;
    try {
      const r = await api.listMyAlgorithmRequests();
      setMine(r.items);
    } catch {
      setMine([]);
    }
  }, [user]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function submit() {
    setErr("");
    setOkMsg("");
    if (!name.trim() || !image.trim()) {
      setErr(t("请填写算法名称和镜像。", "Algorithm name and image are required."));
      return;
    }
    setBusy(true);
    try {
      const a = await api.requestAlgorithm({
        name: name.trim(),
        runtime: runtime.trim(),
        image: image.trim(),
        output_kind: outputKind,
        image_digest: imageDigest.trim() || undefined,
        source_ref: sourceRef.trim() || undefined,
      });
      setOkMsg(t(`已提交「${a.name}」,待运营审核。`, `Submitted “${a.name}” — pending ops review.`));
      setName("");
      setImage("");
      setImageDigest("");
      setSourceRef("");
      await refresh();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <h2 className="text-lg font-semibold">{t("申请自定义算法", "Submit a custom algorithm")}</h2>
      <p className="mt-1 text-sm text-neutral-500">
        {t(
          "提交一个容器化算法供平台审核。通过审核后,它会出现在允许该算法的数据集的可选列表里。诚实说明:提交即进入待审核,审核前不可运行——这是为防止未经审计的代码碰到数据。",
          "Submit a containerized algorithm for platform review. Once approved it appears in the choices for datasets that allow it. Honest note: a submission is pending and cannot run until reviewed — the audit gate keeps unvetted code away from data.",
        )}
      </p>
      <p className="mt-2 text-sm text-neutral-500">
        {t("不知道怎么写?用 ", "Not sure how to write it? Author it with ")}
        <Link href="/build" className="font-medium text-forest-700 underline underline-offset-2">
          {t("Lumen 按算法契约编写 →", "Lumen against the algorithm contract →")}
        </Link>
      </p>

      {err && (
        <div className="mt-3">
          <Alert>{err}</Alert>
        </div>
      )}
      {okMsg && (
        <div className="mt-3">
          <Alert kind="success">{okMsg}</Alert>
        </div>
      )}

      <div className="mt-4 grid gap-3 md:grid-cols-2">
        <Field label={t("算法名称", "Algorithm name")}>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("如:KMeans 聚类", "e.g. KMeans clustering")} />
        </Field>
        <Field label={t("运行时", "Runtime")}>
          <Input value={runtime} onChange={(e) => setRuntime(e.target.value)} placeholder="python-sklearn" />
        </Field>
        <Field label={t("容器镜像", "Container image")} hint={t("registry/repo:tag", "registry/repo:tag")}>
          <Input value={image} onChange={(e) => setImage(e.target.value)} placeholder="docker.io/you/algo:v1" />
        </Field>
        <Field label={t("输出类型", "Output kind")}>
          <Select value={outputKind} onChange={(e) => setOutputKind(e.target.value)}>
            {ALGO_OUTPUT_KINDS.map((k) => (
              <option key={k} value={k}>
                {k}
              </option>
            ))}
          </Select>
        </Field>
        <Field label={t("镜像 digest(可选,可信算法需要)", "Image digest (optional; required for trusted)")}>
          <Input value={imageDigest} onChange={(e) => setImageDigest(e.target.value)} placeholder="sha256:…" />
        </Field>
        <Field label={t("源码引用(可选)", "Source reference (optional)")}>
          <Input value={sourceRef} onChange={(e) => setSourceRef(e.target.value)} placeholder={t("如 git 提交 URL", "e.g. a git commit URL")} />
        </Field>
      </div>
      <Button className="mt-3" onClick={submit} disabled={busy}>
        {busy ? t("提交中…", "Submitting…") : t("提交审核", "Submit for review")}
      </Button>

      <div className="mt-5 border-t border-neutral-100 pt-4">
        <div className="mb-2 text-sm font-medium text-neutral-700">{t("我的算法申请", "My algorithm requests")}</div>
        {mine === null ? (
          <p className="text-sm text-neutral-400">{t("加载中…", "Loading…")}</p>
        ) : mine.length === 0 ? (
          <p className="text-sm text-neutral-400">{t("还没有提交过算法。", "No algorithm requests yet.")}</p>
        ) : (
          <ul className="space-y-2">
            {mine.map((a) => (
              <li key={a.id} className="flex items-center justify-between gap-2 text-sm">
                <span className="truncate font-medium text-neutral-800">
                  {a.name} <span className="text-xs text-neutral-400">· {a.runtime} · {a.output_kind}</span>
                </span>
                <span className="flex shrink-0 items-center gap-2">
                  <Badge>{a.status}</Badge>
                  {a.trusted && <Badge>{t("可信", "trusted")}</Badge>}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
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

// ---------------------------------------------------------------------------
// Buyer: provenance & integrity certificate (存证) for a released compute result.
// On click it fetches the platform-issued certificate and shows the VO-<id>; the
// cert binds the output SHA-256 to the audited algorithm (pinned image digest).
// ---------------------------------------------------------------------------
function JobCertificate({ jobId, federated }: { jobId: string; federated?: boolean }) {
  const { t } = useT();
  const [cert, setCert] = useState<Record<string, unknown> | null>(null);
  const [loading, setLoading] = useState(false);
  const [open, setOpen] = useState(false);

  async function load() {
    if (cert) {
      setOpen(true);
      return;
    }
    setLoading(true);
    try {
      const c = federated ? await api.getFederatedCertificate(jobId) : await api.getComputeJobCertificate(jobId);
      setCert(c);
      setOpen(true);
    } catch {
      /* leave the button in its idle state so the buyer can retry */
    } finally {
      setLoading(false);
    }
  }

  const certId = cert ? ((cert["certificate_id"] as string) || "") : "";

  return (
    <>
      {certId ? (
        <button
          type="button"
          onClick={() => setOpen(true)}
          title={t("查看计算结果存证", "View the result certificate")}
          className="rounded-full bg-emerald-50 px-2 py-0.5 font-mono text-[11px] text-emerald-700 transition hover:bg-emerald-100"
        >
          {certId}
        </button>
      ) : (
        <button
          type="button"
          onClick={() => void load()}
          disabled={loading}
          className="rounded-full border border-neutral-200 px-2 py-0.5 text-[11px] text-neutral-500 transition hover:bg-neutral-50 disabled:opacity-50"
        >
          {loading ? t("出具中…", "Issuing…") : t("存证凭证", "Certificate")}
        </button>
      )}
      {open && cert && <ComputeCertificateModal cert={cert} onClose={() => setOpen(false)} />}
    </>
  );
}
