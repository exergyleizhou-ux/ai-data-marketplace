"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { api, yuan, type Dataset } from "@/lib/api";
import { Protected } from "@/components/Protected";
import { Alert, Badge, Button, Card, Empty, Field, Input, Select, Spinner, Textarea } from "@/components/ui";

export default function SellPage() {
  return (
    <Protected requireKYC>
      <SellInner />
    </Protected>
  );
}

function SellInner() {
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
      <h1 className="text-2xl font-semibold">卖家工作台</h1>
      {err && <Alert>{err}</Alert>}
      <CreateForm onCreated={reload} />

      <section className="space-y-3">
        <h2 className="text-lg font-semibold">我的数据集</h2>
        {items === null ? (
          <Spinner />
        ) : items.length === 0 ? (
          <Empty>还没有数据集，先在上方创建一个</Empty>
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
      setErr("请勾选来源合法性承诺");
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
      <Button onClick={() => setOpen(true)}>+ 创建数据集</Button>
    );

  return (
    <Card>
      <h2 className="mb-4 text-lg font-semibold">创建数据集</h2>
      <form onSubmit={submit} className="space-y-4">
        {err && <Alert>{err}</Alert>}
        <Field label="标题">
          <Input value={title} onChange={(e) => setTitle(e.target.value)} required />
        </Field>
        <Field label="描述">
          <Textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-3">
          <Field label="数据类型">
            <Select value={dataType} onChange={(e) => setDataType(e.target.value)}>
              <option value="text">text</option>
              <option value="code">code</option>
              <option value="structured">structured</option>
            </Select>
          </Field>
          <Field label="许可类型">
            <Select value={licenseType} onChange={(e) => setLicenseType(e.target.value)}>
              <option value="commercial">commercial</option>
              <option value="research">research</option>
              <option value="train_only">train_only</option>
            </Select>
          </Field>
          <Field label="建议价（元）">
            <Input type="number" step="0.01" min="0" value={price} onChange={(e) => setPrice(e.target.value)} />
          </Field>
        </div>
        <Field label="领域标签（可选）">
          <Input value={domain} onChange={(e) => setDomain(e.target.value)} placeholder="如 medical / finance / general" />
        </Field>
        <div className="rounded-lg border border-neutral-200 p-4">
          <div className="mb-3 text-sm font-medium text-neutral-700">来源合法性声明</div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="数据来源">
              <Input value={source} onChange={(e) => setSource(e.target.value)} required placeholder="如 自有采集 / 授权获得" />
            </Field>
            <Field label="采集方式">
              <Input value={collection} onChange={(e) => setCollection(e.target.value)} required placeholder="如 公开网页 / 内部生产" />
            </Field>
          </div>
          <label className="mt-3 flex items-center gap-2 text-sm">
            <input type="checkbox" checked={containsPII} onChange={(e) => setContainsPII(e.target.checked)} />
            数据包含个人信息（如实声明；未声明却检出 PII 会被退回）
          </label>
          <label className="mt-2 flex items-start gap-2 text-sm">
            <input type="checkbox" checked={commitment} onChange={(e) => setCommitment(e.target.checked)} className="mt-1" />
            我承诺数据来源合法、拥有授权，并承担相应法律责任。
          </label>
        </div>
        <div className="flex gap-2">
          <Button type="submit" disabled={busy}>
            {busy ? "创建中…" : "创建草稿"}
          </Button>
          <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
            取消
          </Button>
        </div>
      </form>
    </Card>
  );
}

const PART_SIZE = 4 << 20; // 4 MiB

function DatasetRow({ d, onChange }: { d: Dataset; onChange: () => void }) {
  const fileRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState("");
  const [err, setErr] = useState("");

  async function sign() {
    setErr("");
    setBusy("签约中…");
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
        setBusy(`上传中 ${i + 1}/${parts}…`);
        const chunk = file.slice(i * PART_SIZE, (i + 1) * PART_SIZE);
        await api.uploadPart(d.id, init.upload_id, i + 1, chunk);
      }
      setBusy("质检中…");
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
            <span>{d.sample_count} 样本</span>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {!d.source_signed_at && (
            <Button variant="secondary" onClick={sign} disabled={!!busy}>
              签署来源承诺
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
                {busy || "上传数据文件"}
              </Button>
            </>
          )}
        </div>
      </div>
      {err && <div className="mt-2"><Alert>{err}</Alert></div>}
      {d.status === "reviewing" && (
        <p className="mt-2 text-xs text-neutral-500">已过质检，等待运营审核上架。</p>
      )}
      {d.status === "draft" && d.source_signed_at && (
        <p className="mt-2 text-xs text-neutral-500">已签约，请上传数据文件以进入质检。</p>
      )}
    </Card>
  );
}
