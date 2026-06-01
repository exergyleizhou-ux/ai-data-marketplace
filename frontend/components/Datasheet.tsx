"use client";

import { useState } from "react";
import { type Datasheet } from "@/lib/api";
import { Button, Field, Textarea, Input } from "@/components/ui";

/** Datasheet — structured dataset documentation (Gebru et al. 2021 / HF dataset
 *  cards). Shared field metadata drives both the buyer view and the seller editor. */

type FieldKey = Exclude<keyof Datasheet, "languages">;

const FIELDS: { key: FieldKey; zh: string; en: string; ph: string }[] = [
  { key: "intended_uses", zh: "用途", en: "Intended uses", ph: "这份数据适合用于哪些任务/模型训练" },
  { key: "out_of_scope_uses", zh: "不适用场景", en: "Out-of-scope uses", ph: "不应被用于哪些用途" },
  { key: "composition", zh: "数据构成", en: "Composition", ph: "实例代表什么、字段/列、规模、是否含子集" },
  { key: "collection_process", zh: "采集过程", en: "Collection process", ph: "如何/何时/何地采集，来源" },
  { key: "preprocessing", zh: "预处理", en: "Preprocessing / labeling", ph: "清洗、去重、脱敏、标注等处理" },
  { key: "limitations", zh: "已知局限", en: "Limitations", ph: "已知缺口、偏差、代表性问题（如实披露）" },
  { key: "ethical_considerations", zh: "伦理考量", en: "Ethical considerations", ph: "敏感内容、授权同意、潜在风险" },
  { key: "update_policy", zh: "更新维护", en: "Maintenance", ph: "是否更新、频率、联系人" },
];

export function DatasheetView({ ds }: { ds: Datasheet }) {
  const present = FIELDS.filter((f) => (ds[f.key] || "").trim() !== "");
  const langs = ds.languages?.filter(Boolean) ?? [];
  if (present.length === 0 && langs.length === 0) {
    return (
      <p className="text-sm text-neutral-400">
        卖家尚未填写数据说明卡。<span className="text-neutral-300">No datasheet provided yet.</span>
      </p>
    );
  }
  return (
    <dl className="space-y-4">
      {langs.length > 0 && (
        <div>
          <dt className="text-sm font-medium text-neutral-700">
            语言 <span className="font-normal text-neutral-400">/ Languages</span>
          </dt>
          <dd className="mt-1 flex flex-wrap gap-1">
            {langs.map((l) => (
              <span key={l} className="rounded-full bg-neutral-100 px-2 py-0.5 text-xs text-neutral-600">
                {l}
              </span>
            ))}
          </dd>
        </div>
      )}
      {present.map((f) => (
        <div key={f.key}>
          <dt className="text-sm font-medium text-neutral-700">
            {f.zh} <span className="font-normal text-neutral-400">/ {f.en}</span>
          </dt>
          <dd className="mt-1 whitespace-pre-wrap text-sm text-neutral-600">{ds[f.key]}</dd>
        </div>
      ))}
    </dl>
  );
}

export function DatasheetEditor({
  initial,
  onSave,
}: {
  initial?: Datasheet;
  onSave: (ds: Datasheet) => Promise<void>;
}) {
  const [v, setV] = useState<Record<string, string>>(() => {
    const init: Record<string, string> = {};
    for (const f of FIELDS) init[f.key] = initial?.[f.key] ?? "";
    return init;
  });
  const [langs, setLangs] = useState((initial?.languages ?? []).join(", "));
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [done, setDone] = useState(false);

  async function save() {
    setErr("");
    setBusy(true);
    try {
      const ds: Datasheet = {};
      for (const f of FIELDS) {
        const val = v[f.key].trim();
        if (val) ds[f.key] = val;
      }
      const langList = langs.split(/[,，\s]+/).map((s) => s.trim()).filter(Boolean);
      if (langList.length) ds.languages = langList;
      await onSave(ds);
      setDone(true);
      setTimeout(() => setDone(false), 2000);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-3">
      <p className="text-xs text-neutral-500">
        数据说明卡（可选，随时可改）。参考 Datasheets for Datasets / Hugging Face dataset cards。填得越完整，买家越信任。
      </p>
      {err && <p className="text-sm text-rose-600">{err}</p>}
      <Field label="语言 / Languages（逗号分隔）">
        <Input value={langs} onChange={(e) => setLangs(e.target.value)} placeholder="zh, en" />
      </Field>
      {FIELDS.map((f) => (
        <Field key={f.key} label={`${f.zh} / ${f.en}`}>
          <Textarea
            rows={2}
            value={v[f.key]}
            onChange={(e) => setV((s) => ({ ...s, [f.key]: e.target.value }))}
            placeholder={f.ph}
          />
        </Field>
      ))}
      <div className="flex items-center gap-2">
        <Button onClick={save} disabled={busy}>
          {busy ? "保存中…" : "保存说明卡"}
        </Button>
        {done && <span className="text-sm text-emerald-600">已保存 ✓</span>}
      </div>
    </div>
  );
}
