"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { api, yuan, type Dataset } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Protected } from "@/components/Protected";
import { Alert, Badge, Button, Card, Empty, Field, Input, Select, Spinner, Textarea } from "@/components/ui";
import { DatasheetEditor } from "@/components/Datasheet";
import { ComputeOfferEditor } from "@/components/Compute";

export default function SellPage() {
  return (
    <Protected requireKYC>
      <SellInner />
    </Protected>
  );
}

function SellInner() {
  const { t } = useT();
  const [items, setItems] = useState<Dataset[] | null>(null);
  const [err, setErr] = useState("");

  const reload = useCallback(async () => {
    try {
      setItems((await api.myDatasets()).items);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-semibold">{t("卖家工作台", "Seller Workbench")}</h1>
      {err && <Alert>{err}</Alert>}
      <CreateForm onCreated={reload} />

      <section className="space-y-3">
        <h2 className="text-lg font-semibold">{t("我的数据集", "My datasets")}</h2>
        {items === null ? (
          <Spinner />
        ) : items.length === 0 ? (
          <Empty>{t("还没有数据集，先在上方创建一个", "No datasets yet — create one above")}</Empty>
        ) : (
          <div className="space-y-3">
            {items.map((d) => (
              <DatasetRow key={d.id} d={d} onChange={reload} />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function CreateForm({ onCreated }: { onCreated: () => void }) {
  const { t } = useT();
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [dataType, setDataType] = useState("text");
  const [licenseType, setLicenseType] = useState("commercial");
  const [domain, setDomain] = useState("");
  const [price, setPrice] = useState("100.00");
  const [source, setSource] = useState("");
  const [collection, setCollection] = useState("");
  const [containsPII, setContainsPII] = useState(false);
  const [commitment, setCommitment] = useState(false);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    if (!commitment) {
      setErr(t("请勾选来源合法性承诺", "Please check the provenance-legality commitment"));
      return;
    }
    setBusy(true);
    try {
      await api.createDataset({
        title,
        description,
        data_type: dataType,
        license_type: licenseType,
        domain: domain || undefined,
        suggested_price_cents: Math.round(parseFloat(price || "0") * 100),
        source_declaration: {
          source,
          collection_method: collection,
          contains_pii: containsPII,
          license_scope: licenseType,
          commitment,
        },
      });
      setTitle("");
      setDescription("");
      setSource("");
      setCollection("");
      setCommitment(false);
      setOpen(false);
      onCreated();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  if (!open)
    return (
      <Button onClick={() => setOpen(true)}>{t("+ 创建数据集", "+ Create dataset")}</Button>
    );

  return (
    <Card>
      <h2 className="mb-4 text-lg font-semibold">{t("创建数据集", "Create dataset")}</h2>
      <form onSubmit={submit} className="space-y-4">
        {err && <Alert>{err}</Alert>}
        <Field label={t("标题", "Title")}>
          <Input value={title} onChange={(e) => setTitle(e.target.value)} required />
        </Field>
        <Field label={t("描述", "Description")}>
          <Textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-3">
          <Field label={t("数据类型", "Data type")}>
            <Select value={dataType} onChange={(e) => setDataType(e.target.value)}>
              <option value="text">text</option>
              <option value="code">code</option>
              <option value="structured">structured</option>
            </Select>
          </Field>
          <Field label={t("许可类型", "License")}>
            <Select value={licenseType} onChange={(e) => setLicenseType(e.target.value)}>
              <option value="commercial">commercial</option>
              <option value="research">research</option>
              <option value="train_only">train_only</option>
            </Select>
          </Field>
          <Field label={t("建议价（元）", "Suggested price (CNY)")}>
            <Input type="number" step="0.01" min="0" value={price} onChange={(e) => setPrice(e.target.value)} />
          </Field>
        </div>
        <EarningsSimulator price={price} />
        <Field label={t("领域标签（可选）", "Domain tag (optional)")}>
          <Input value={domain} onChange={(e) => setDomain(e.target.value)} placeholder={t("如 medical / finance / general", "e.g. medical / finance / general")} />
        </Field>
        <div className="rounded-lg border border-neutral-200 p-4">
          <div className="mb-3 text-sm font-medium text-neutral-700">{t("来源合法性声明", "Provenance-legality declaration")}</div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t("数据来源", "Data source")}>
              <Input value={source} onChange={(e) => setSource(e.target.value)} required placeholder={t("如 自有采集 / 授权获得", "e.g. self-collected / licensed")} />
            </Field>
            <Field label={t("采集方式", "Collection method")}>
              <Input value={collection} onChange={(e) => setCollection(e.target.value)} required placeholder={t("如 公开网页 / 内部生产", "e.g. public web / internal")} />
            </Field>
          </div>
          <label className="mt-3 flex items-center gap-2 text-sm">
            <input type="checkbox" checked={containsPII} onChange={(e) => setContainsPII(e.target.checked)} />
            {t("数据包含个人信息（如实声明；未声明却检出 PII 会被退回）", "The data contains personal information (declare honestly; undeclared PII that is detected will be bounced)")}
          </label>
          <label className="mt-2 flex items-start gap-2 text-sm">
            <input type="checkbox" checked={commitment} onChange={(e) => setCommitment(e.target.checked)} className="mt-1" />
            {t("我承诺数据来源合法、拥有授权，并承担相应法律责任。", "I warrant the data is lawfully sourced and authorized, and accept the corresponding legal responsibility.")}
          </label>
        </div>
        <div className="flex gap-2">
          <Button type="submit" disabled={busy}>
            {busy ? t("创建中…", "Creating…") : t("创建草稿", "Create draft")}
          </Button>
          <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
            {t("取消", "Cancel")}
          </Button>
        </div>
      </form>
    </Card>
  );
}

const PART_SIZE = 4 << 20; // 4 MiB

function DatasetRow({ d, onChange }: { d: Dataset; onChange: () => void }) {
  const { t } = useT();
  const fileRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState("");
  const [err, setErr] = useState("");
  const [editingSheet, setEditingSheet] = useState(false);
  const [editingOffer, setEditingOffer] = useState(false);

  async function sign() {
    setErr("");
    setBusy(t("签约中…", "Signing…"));
    try {
      await api.signSource(d.id);
      onChange();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy("");
    }
  }

  async function upload(file: File) {
    setErr("");
    try {
      const init = await api.uploadInit(d.id, file.name);
      const parts = Math.max(1, Math.ceil(file.size / PART_SIZE));
      for (let i = 0; i < parts; i++) {
        setBusy(t(`上传中 ${i + 1}/${parts}…`, `Uploading ${i + 1}/${parts}…`));
        const chunk = file.slice(i * PART_SIZE, (i + 1) * PART_SIZE);
        await api.uploadPart(d.id, init.upload_id, i + 1, chunk);
      }
      setBusy(t("质检中…", "Running quality checks…"));
      await api.uploadComplete(d.id, init.upload_id);
      onChange();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy("");
    }
  }

  const canUpload = ["draft", "rejected", "uploading"].includes(d.status) && !!d.source_signed_at;

  return (
    <Card>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <Link href={`/datasets/${d.id}`} className="font-medium hover:underline">
            {d.title}
          </Link>
          <div className="mt-1 flex items-center gap-2 text-xs text-neutral-400">
            <Badge>{d.status}</Badge>
            <span>{d.data_type}</span>
            <span>{yuan(d.final_price_cents ?? d.suggested_price_cents)}</span>
            <span>{t(`${d.sample_count} 样本`, `${d.sample_count} samples`)}</span>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button variant="ghost" onClick={() => setEditingSheet((s) => !s)}>
            {editingSheet ? t("收起说明卡", "Hide datasheet") : t("数据说明卡", "Datasheet")}
          </Button>
          {d.status === "published" && (
            <Button variant="ghost" onClick={() => setEditingOffer((s) => !s)}>
              {editingOffer ? t("收起沙箱售卖", "Hide sandbox sale") : t("沙箱售卖", "Sandbox sale")}
            </Button>
          )}
          {!d.source_signed_at && (
            <Button variant="secondary" onClick={sign} disabled={!!busy}>
              {t("签署来源承诺", "Sign provenance")}
            </Button>
          )}
          {canUpload && (
            <>
              <input
                ref={fileRef}
                type="file"
                className="hidden"
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  if (f) void upload(f);
                }}
              />
              <Button onClick={() => fileRef.current?.click()} disabled={!!busy}>
                {busy || t("上传数据文件", "Upload data file")}
              </Button>
            </>
          )}
        </div>
      </div>
      {err && <div className="mt-2"><Alert>{err}</Alert></div>}
      {d.status === "reviewing" && (
        <p className="mt-2 text-xs text-neutral-500">{t("已过质检，等待运营审核上架。", "Passed quality checks; awaiting ops review to publish.")}</p>
      )}
      {d.status === "draft" && d.source_signed_at && d.sample_count > 0 && (
        <p className="mt-2 text-xs text-amber-600">
          {t(
            "质检发现问题，请查看数据集详情页的质量报告并按建议修正后重新上传。",
            "Quality checks found issues — see the dataset detail page for the report, then fix and re-upload.",
          )}
        </p>
      )}
      {d.status === "draft" && d.source_signed_at && d.sample_count === 0 && (
        <p className="mt-2 text-xs text-neutral-500">{t("已签约，请上传数据文件以进入质检。", "Provenance signed; upload a data file to start quality checks.")}</p>
      )}
      {d.status === "rejected" && (
        <p className="mt-2 text-xs text-rose-600">{t("数据集审核未通过，请修改后重新上传。", "Dataset review rejected; please revise and re-upload.")}</p>
      )}
      {editingSheet && (
        <div className="mt-4 border-t border-neutral-100 pt-4">
          <DatasheetEditor
            initial={d.datasheet}
            onSave={async (ds) => {
              await api.updateDatasheet(d.id, ds);
              onChange();
            }}
          />
        </div>
      )}
      {editingOffer && d.status === "published" && (
        <div className="mt-4 border-t border-neutral-100 pt-4">
          <ComputeOfferEditor datasetId={d.id} />
        </div>
      )}
    </Card>
  );
}

// PLATFORM_FEE_BPS mirrors backend order/model.go (10% = 1000 bps); seller nets 90%.
const PLATFORM_FEE_BPS = 1000;

// EarningsSimulator shows the seller their take-home at the pricing decision
// point: buyer-pays / platform fee / net, with floor rounding matching the backend.
function EarningsSimulator({ price }: { price: string }) {
  const { t } = useT();
  const cents = Math.round((parseFloat(price) || 0) * 100);
  if (cents <= 0) return null;
  const fee = Math.floor((cents * PLATFORM_FEE_BPS) / 10000);
  const net = cents - fee;
  return (
    <div className="rounded-lg border border-neutral-200 bg-neutral-50 p-3">
      <div className="text-sm font-medium text-neutral-700">{t("到手测算", "Take-home estimate")}</div>
      <div className="mt-2 grid grid-cols-3 gap-2 text-center text-sm">
        <div>
          <div className="text-xs text-neutral-400">{t("买家支付", "Buyer pays")}</div>
          <div className="font-semibold">{yuan(cents)}</div>
        </div>
        <div>
          <div className="text-xs text-neutral-400">{t("平台手续费 10%", "Platform fee 10%")}</div>
          <div className="font-semibold text-neutral-500">−{yuan(fee)}</div>
        </div>
        <div>
          <div className="text-xs text-neutral-400">{t("你到手", "You receive")}</div>
          <div className="font-semibold text-emerald-700">{yuan(net)}</div>
        </div>
      </div>
      <div className="mt-2 text-xs text-neutral-400">
        {t(`每卖出 10 份累计到手约 ${yuan(net * 10)}`, `~${yuan(net * 10)} for every 10 sales`)}
      </div>
    </div>
  );
}
