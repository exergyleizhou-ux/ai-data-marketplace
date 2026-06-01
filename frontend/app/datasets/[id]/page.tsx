"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { api, yuan, type Dataset, type Preview, type QualityCheck, type Review } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Alert, Badge, Button, Card, Empty, Spinner } from "@/components/ui";
import { QualityReport } from "@/components/QualityReport";
import { DatasheetView } from "@/components/Datasheet";

export default function DatasetDetailPage({ params }: { params: { id: string } }) {
  const { id } = params;
  const router = useRouter();
  const { user } = useAuth();
  const [ds, setDs] = useState<Dataset | null>(null);
  const [reviews, setReviews] = useState<Review[]>([]);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [previewErr, setPreviewErr] = useState("");
  const [quality, setQuality] = useState<QualityCheck[] | null>(null);
  const [err, setErr] = useState("");
  const [buying, setBuying] = useState(false);
  const [notFound, setNotFound] = useState(false);

  useEffect(() => {
    api.getDataset(id).then(setDs).catch(() => setNotFound(true));
    api.datasetReviews(id).then((r) => setReviews(r.items)).catch(() => {});
    api.datasetQuality(id).then((r) => setQuality(r.checks)).catch(() => setQuality([]));
  }, [id]);

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

  if (notFound) return <Empty>数据集不存在</Empty>;
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
          <h1 className="mt-2 text-2xl font-semibold">{ds.title}</h1>
          <p className="mt-2 whitespace-pre-wrap text-neutral-600">{ds.description || "（无描述）"}</p>
        </div>

        <Card>
          <div className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
            <Meta label="许可类型" value={ds.license_type} />
            <Meta label="样本数" value={String(ds.sample_count)} />
            <Meta label="大小" value={`${(ds.total_size_bytes / 1024).toFixed(1)} KB`} />
            <Meta label="来源签约" value={ds.source_signed_at ? "已签署" : "未签署"} />
          </div>
          {ds.source_declaration && (
            <div className="mt-4 border-t border-neutral-100 pt-4 text-sm text-neutral-600">
              <div className="font-medium text-neutral-700">来源声明</div>
              <div className="mt-1 grid gap-1 sm:grid-cols-2">
                <span>来源：{ds.source_declaration.source}</span>
                <span>采集方式：{ds.source_declaration.collection_method}</span>
                <span>许可范围：{ds.source_declaration.license_scope}</span>
                <span>含个人信息：{ds.source_declaration.contains_pii ? "是" : "否"}</span>
              </div>
            </div>
          )}
        </Card>

        <Card>
          <div className="mb-3 flex items-center justify-between">
            <h2 className="font-semibold">样本预览</h2>
            {!preview && (
              <Button variant="secondary" onClick={loadPreview} disabled={!user}>
                {user ? "加载预览" : "登录后预览"}
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
                已脱敏抽样 {preview.line_count} 行{preview.truncated ? "（仅展示部分）" : ""}。预览不代表完整数据。
              </p>
            </>
          )}
        </Card>

        <Card>
          <h2 className="mb-3 font-semibold">
            数据说明卡 <span className="font-normal text-neutral-400">/ Datasheet</span>
          </h2>
          <DatasheetView ds={ds.datasheet ?? {}} />
        </Card>

        <Card>
          <h2 className="mb-3 font-semibold">
            数据质量 <span className="font-normal text-neutral-400">/ Data Quality</span>
          </h2>
          {quality === null ? <Spinner /> : <QualityReport checks={quality} />}
          <div className="mt-4 border-t border-neutral-100 pt-3">
            <a
              href={api.croissantUrl(id)}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 text-xs text-neutral-500 underline-offset-2 hover:text-neutral-800 hover:underline"
            >
              🥐 机器可读元数据 / Croissant (JSON-LD)
            </a>
            <span className="ml-2 text-xs text-neutral-300">符合 MLCommons Croissant 1.0</span>
          </div>
        </Card>

        <Card>
          <h2 className="mb-3 font-semibold">买家评价（{reviews.length}）</h2>
          {reviews.length === 0 ? (
            <Empty>暂无评价</Empty>
          ) : (
            <ul className="space-y-3">
              {reviews.map((r) => (
                <li key={r.id} className="border-b border-neutral-100 pb-3 last:border-0">
                  <div className="flex items-center gap-2">
                    <span className="text-amber-500">{"★".repeat(r.score)}{"☆".repeat(5 - r.score)}</span>
                    {r.issue_flag && <Badge>有问题</Badge>}
                  </div>
                  {r.comment && <p className="mt-1 text-sm text-neutral-600">{r.comment}</p>}
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      <div className="lg:col-span-1">
        <Card className="sticky top-20">
          <div className="text-3xl font-semibold">{yuan(price)}</div>
          <div className="mt-1 text-sm text-neutral-500">一次性买断 · {ds.license_type}</div>
          {err && <div className="mt-3"><Alert>{err}</Alert></div>}
          <div className="mt-4 space-y-2">
            {ds.status !== "published" ? (
              <Button disabled className="w-full">未上架</Button>
            ) : isSeller ? (
              <Button disabled className="w-full">这是你的数据集</Button>
            ) : !user ? (
              <Button className="w-full" onClick={() => router.push("/login")}>
                登录后购买
              </Button>
            ) : user.kyc_status !== "verified" ? (
              <Button className="w-full" onClick={() => router.push("/account")}>
                需先实名认证
              </Button>
            ) : (
              <Button className="w-full" onClick={buy} disabled={buying}>
                {buying ? "下单中…" : "立即购买"}
              </Button>
            )}
            <p className="text-center text-xs text-neutral-400">下载前需签署数据使用许可协议</p>
          </div>
        </Card>
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
