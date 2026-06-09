"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import {
  api,
  yuan,
  type Dataset,
  type Preview,
  type QualityCheck,
  type Review,
  type VersionInfo,
  type Certificate,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useT } from "@/lib/i18n";
import { Alert, Badge, Button, Card, Empty, Spinner } from "@/components/ui";
import { QualityReport } from "@/components/QualityReport";
import { DatasheetView } from "@/components/Datasheet";
import { SchemaTable, hasSchema } from "@/components/SchemaTable";
import { ComputeBuyer } from "@/components/Compute";

export default function DatasetDetailPage({ params }: { params: { id: string } }) {
  const { id } = params;
  const router = useRouter();
  const { user } = useAuth();
  const { t } = useT();
  const [ds, setDs] = useState<Dataset | null>(null);
  const [reviews, setReviews] = useState<Review[]>([]);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [previewErr, setPreviewErr] = useState("");
  const [quality, setQuality] = useState<QualityCheck[] | null>(null);
  const [versions, setVersions] = useState<VersionInfo[]>([]);
  const [cert, setCert] = useState<Certificate | null>(null);
  const [err, setErr] = useState("");
  const [buying, setBuying] = useState(false);
  const [notFound, setNotFound] = useState(false);
  const [watching, setWatching] = useState(false);
  const [watchBusy, setWatchBusy] = useState(false);

  useEffect(() => {
    api.getDataset(id).then(setDs).catch(() => setNotFound(true));
    api.datasetReviews(id).then((r) => setReviews(r.items)).catch(() => {});
    api.datasetQuality(id).then((r) => setQuality(r.checks)).catch(() => setQuality([]));
    api.datasetVersions(id).then((r) => setVersions(r.versions)).catch(() => {});
    api.datasetCertificate(id).then(setCert).catch(() => {});
    api.listMyWatches().then((r) => setWatching(r.items.some((w) => w.dataset_id === id))).catch(() => {});
  }, [id]);

  async function toggleWatch() {
    setWatchBusy(true);
    try {
      if (watching) {
        await api.unwatchDataset(id);
        setWatching(false);
      } else {
        await api.watchDataset(id);
        setWatching(true);
      }
    } catch { /* silent */ }
    finally { setWatchBusy(false); }
  }

  async function loadPreview() {
    setPreviewErr("");
    try {
      setPreview(await api.preview(id));
    } catch (e) {
      setPreviewErr((e as Error).message);
    }
  }

  async function buy() {
    setErr("");
    setBuying(true);
    try {
      const order = await api.createOrder(id, ds!.license_type);
      router.push(`/orders/${order.id}`);
    } catch (e) {
      setErr((e as Error).message);
      setBuying(false);
    }
  }

  if (notFound) return <Empty>{t("数据集不存在", "Dataset not found")}</Empty>;
  if (!ds) return <Spinner />;

  const price = ds.final_price_cents ?? ds.suggested_price_cents;
  const isSeller = user?.id === ds.seller_id;

  return (
    <div className="grid gap-6 lg:grid-cols-3">
      <div className="space-y-6 lg:col-span-2">
        <div>
          <div className="flex items-center gap-2">
            <Badge>{ds.data_type}</Badge>
            <Badge>{ds.status}</Badge>
            {ds.domain && <span className="text-xs text-neutral-400">{ds.domain}</span>}
          </div>
          <h1 className="mt-2 flex items-center gap-2 text-2xl font-semibold">
            {ds.title}
            <button
              onClick={toggleWatch}
              disabled={watchBusy}
              className="text-xl"
              title={watching ? "取消收藏" : "收藏"}
            >
              {watching ? "⭐" : "☆"}
            </button>
          </h1>
          <p className="mt-2 whitespace-pre-wrap text-neutral-600">{ds.description || t("（无描述）", "(no description)")}</p>
        </div>

        <Card>
          <div className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
            <Meta label={t("许可类型", "License")} value={ds.license_type} />
            <Meta label={t("样本数", "Samples")} value={String(ds.sample_count)} />
            <Meta label={t("大小", "Size")} value={`${(ds.total_size_bytes / 1024).toFixed(1)} KB`} />
            <Meta label={t("来源签约", "Provenance")} value={ds.source_signed_at ? t("已签署", "Signed") : t("未签署", "Unsigned")} />
          </div>
          {ds.source_declaration && (
            <div className="mt-4 border-t border-neutral-100 pt-4 text-sm text-neutral-600">
              <div className="font-medium text-neutral-700">{t("来源声明", "Provenance declaration")}</div>
              <div className="mt-1 grid gap-1 sm:grid-cols-2">
                <span>{t("来源：", "Source: ")}{ds.source_declaration.source}</span>
                <span>{t("采集方式：", "Collection: ")}{ds.source_declaration.collection_method}</span>
                <span>{t("许可范围：", "License scope: ")}{ds.source_declaration.license_scope}</span>
                <span>{t("含个人信息：", "Contains PII: ")}{ds.source_declaration.contains_pii ? t("是", "Yes") : t("否", "No")}</span>
              </div>
            </div>
          )}
        </Card>

        <Card>
          <div className="mb-3 flex items-center justify-between">
            <h2 className="font-semibold">{t("样本预览", "Sample preview")}</h2>
            {!preview && (
              <Button variant="secondary" onClick={loadPreview} disabled={!user}>
                {user ? t("加载预览", "Load preview") : t("登录后预览", "Sign in to preview")}
              </Button>
            )}
          </div>
          {previewErr && <Alert>{previewErr}</Alert>}
          {preview && (
            <>
              <pre className="max-h-72 overflow-auto rounded-md bg-neutral-900 p-4 text-xs leading-relaxed text-neutral-100">
                {preview.lines.join("\n")}
              </pre>
              <p className="mt-2 text-xs text-neutral-400">
                {t(
                  `已脱敏抽样 ${preview.line_count} 行${preview.truncated ? "（仅展示部分）" : ""}。预览不代表完整数据。`,
                  `Masked sample of ${preview.line_count} lines${preview.truncated ? " (partial)" : ""}. The preview does not represent the full data.`,
                )}
              </p>
            </>
          )}
        </Card>

        <Card>
          <h2 className="mb-3 font-semibold">
            {t("数据说明卡", "Datasheet")} <span className="font-normal text-neutral-400">/ Datasheet</span>
          </h2>
          <DatasheetView ds={ds.datasheet ?? {}} />
        </Card>

        {quality && hasSchema(quality) && (
          <Card>
            <h2 className="mb-3 font-semibold">
              {t("数据结构", "Schema")} <span className="font-normal text-neutral-400">/ Schema</span>
            </h2>
            <SchemaTable checks={quality} />
          </Card>
        )}

        <Card>
          <h2 className="mb-3 font-semibold">
            {t("数据质量", "Data Quality")} <span className="font-normal text-neutral-400">/ Data Quality</span>
          </h2>
          {quality === null ? <Spinner /> : <QualityReport checks={quality} />}
          <div className="mt-4 border-t border-neutral-100 pt-3">
            <a
              href={api.croissantUrl(id)}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 text-xs text-neutral-500 underline-offset-2 hover:text-neutral-800 hover:underline"
            >
              {t("🥐 机器可读元数据", "🥐 Machine-readable metadata")} / Croissant (JSON-LD)
            </a>
            <span className="ml-2 text-xs text-neutral-300">{t("符合 MLCommons Croissant 1.0", "MLCommons Croissant 1.0")}</span>
          </div>
        </Card>

        <Card>
          <h2 className="mb-3 font-semibold">{t(`买家评价（${reviews.length}）`, `Reviews (${reviews.length})`)}</h2>
          {reviews.length === 0 ? (
            <Empty>{t("暂无评价", "No reviews yet")}</Empty>
          ) : (
            <ul className="space-y-3">
              {reviews.map((r) => (
                <li key={r.id} className="border-b border-neutral-100 pb-3 last:border-0">
                  <div className="flex items-center gap-2">
                    <span className="text-amber-500">{"★".repeat(r.score)}{"☆".repeat(5 - r.score)}</span>
                    {r.issue_flag && <Badge>{t("有问题", "Issue")}</Badge>}
                  </div>
                  {r.comment && <p className="mt-1 text-sm text-neutral-600">{r.comment}</p>}
                </li>
              ))}
            </ul>
          )}
        </Card>

        {cert?.status === "registered" && (
          <Card>
            <h2 className="mb-3 font-semibold">
              {t("数据存证凭证", "Integrity Certificate")} <span className="font-normal text-neutral-400">/ Integrity Certificate</span>
            </h2>
            <div className="space-y-2 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <span className="rounded-md bg-green-50 px-2 py-1 font-mono text-sm font-semibold text-green-800">
                  {cert.certificate_id}
                </span>
                <span className="text-xs text-neutral-400">{t("一数一码", "one-dataset-one-code")} · {cert.operator}</span>
              </div>
              <div className="grid gap-1 text-xs text-neutral-500 sm:grid-cols-2">
                <span className="truncate">SHA-256: <span className="font-mono">{cert.content_sha256}</span></span>
                {cert.registered_at && <span>{t("登记时间: ", "Registered: ")}{cert.registered_at.slice(0, 10)}</span>}
              </div>
              {cert.quality && Object.keys(cert.quality).length > 0 && (
                <div className="flex flex-wrap gap-1">
                  {Object.entries(cert.quality).map(([k, v]) => (
                    <span key={k} className="rounded-full bg-neutral-100 px-2 py-0.5 text-[11px] text-neutral-600">
                      {k}: {v}
                    </span>
                  ))}
                </div>
              )}
              <p className="border-t border-neutral-100 pt-2 text-[11px] leading-relaxed text-neutral-400">
                {cert.statement_zh}
              </p>
            </div>
          </Card>
        )}

        {versions.length > 0 && (
          <Card>
            <h2 className="mb-3 font-semibold">
              {t("版本历史", "Versions")} <span className="font-normal text-neutral-400">/ Versions</span>
            </h2>
            <ul className="space-y-2">
              {versions.map((v) => (
                <li key={v.version_no} className="flex items-baseline gap-3 text-sm">
                  <span className="font-medium text-neutral-700">v{v.version_no}</span>
                  <span className="text-xs text-neutral-400">{(v.created_at || "").slice(0, 10)}</span>
                  {v.changelog && <span className="text-neutral-600">{v.changelog}</span>}
                </li>
              ))}
            </ul>
          </Card>
        )}
      </div>

      <div className="lg:col-span-1">
        <Card className="sticky top-20">
          <div className="text-3xl font-semibold">{yuan(price)}</div>
          <div className="mt-1 text-sm text-neutral-500">{t("一次性买断", "One-time purchase")} · {ds.license_type}</div>
          {err && <div className="mt-3"><Alert>{err}</Alert></div>}
          <div className="mt-4 space-y-2">
            {ds.status !== "published" ? (
              <Button disabled className="w-full">{t("未上架", "Not listed")}</Button>
            ) : isSeller ? (
              <Button disabled className="w-full">{t("这是你的数据集", "This is your dataset")}</Button>
            ) : !user ? (
              <Button className="w-full" onClick={() => router.push("/login")}>
                {t("登录后购买", "Sign in to buy")}
              </Button>
            ) : user.kyc_status !== "verified" ? (
              <Button className="w-full" onClick={() => router.push("/account")}>
                {t("需先实名认证", "Real-name verification required")}
              </Button>
            ) : (
              <Button className="w-full" onClick={buy} disabled={buying}>
                {buying ? t("下单中…", "Placing order…") : t("立即购买", "Buy now")}
              </Button>
            )}
            <p className="text-center text-xs text-neutral-400">{t("下载前需签署数据使用许可协议", "You must sign the data-use license before downloading")}</p>
          </div>
        </Card>
        <div className="mt-4">
          <ComputeBuyer datasetId={id} sellerId={ds.seller_id} />
        </div>
      </div>
    </div>
  );
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-neutral-400">{label}</div>
      <div className="font-medium text-neutral-800">{value}</div>
    </div>
  );
}
