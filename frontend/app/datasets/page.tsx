"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, yuan, type Dataset, type ComputeSignal } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Badge, Button, Card, Empty, Input, Select, SkeletonGrid } from "@/components/ui";
import { QualityBadge } from "@/components/QualityReport";
import { Reveal } from "@/components/Reveal";

const DATA_TYPES = ["", "text", "code", "structured"];

// ComputeBadge surfaces the platform's flagship capability in the catalog: this
// dataset supports verifiable compute-to-data, at a trust level, with N results
// already produced — a discoverability + confidence cue.
function ComputeBadge({ sig }: { sig: ComputeSignal }) {
  const { t } = useT();
  const extra = sig.allow_federated ? " · " + t("联邦", "federated") : "";
  return (
    <span className="inline-flex items-center gap-1 rounded-full border border-forest-200 bg-forest-50 px-2 py-0.5 text-[11px] font-medium text-forest-700">
      <span aria-hidden className="inline-block h-1.5 w-1.5 rounded-full bg-forest-600" />
      {t("可信计算", "Compute-to-data")} · {sig.trust_level}
      {sig.jobs_run > 0 ? " · " + t(`${sig.jobs_run} 次`, `${sig.jobs_run} runs`) : ""}
      {extra}
    </span>
  );
}

export default function DatasetsPage() {
  const { t } = useT();
  const [items, setItems] = useState<Dataset[] | null>(null);
  const [signals, setSignals] = useState<Record<string, ComputeSignal>>({});
  const [q, setQ] = useState("");
  const [dataType, setDataType] = useState("");
  const [sort, setSort] = useState("newest");
  const [c2dOnly, setC2dOnly] = useState(false);

  const load = useCallback(async () => {
    setItems(null);
    const res = await api.listDatasets({ q, data_type: dataType, sort, limit: 50 });
    setItems(res.items);
    // Best-effort enrichment: badge which datasets support verifiable compute.
    const ids = res.items.map((d) => d.id);
    if (ids.length > 0) {
      try {
        const sig = await api.computeOfferSignals(ids);
        setSignals(sig.signals ?? {});
      } catch {
        setSignals({});
      }
    } else {
      setSignals({});
    }
  }, [q, dataType, sort]);

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const visible = items === null ? null : c2dOnly ? items.filter((d) => signals[d.id]?.enabled) : items;

  return (
    <div className="space-y-6 pt-2">
      <div>
        <p className="font-mono text-kicker uppercase text-muted">{t("目录", "Catalog")}</p>
        <h1 className="mt-3 font-display text-display-sm leading-tight tracking-tight">{t("数据市场", "Data Marketplace")}</h1>
      </div>

      <form
        onSubmit={(e) => {
          e.preventDefault();
          void load();
        }}
        className="flex flex-wrap items-end gap-3"
      >
        <div className="min-w-[14rem] flex-1">
          <Input placeholder={t("搜索标题 / 描述…", "Search title / description…")} value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <Select value={dataType} onChange={(e) => setDataType(e.target.value)} className="w-36">
          {DATA_TYPES.map((dt) => (
            <option key={dt} value={dt}>
              {dt === "" ? t("全部类型", "All types") : dt}
            </option>
          ))}
        </Select>
        <Select value={sort} onChange={(e) => setSort(e.target.value)} className="w-36">
          <option value="newest">{t("最新", "Newest")}</option>
          <option value="quality">{t("质量优先", "Quality first")}</option>
          <option value="price_asc">{t("价格从低", "Price ↑")}</option>
          <option value="price_desc">{t("价格从高", "Price ↓")}</option>
        </Select>
        <label className="flex cursor-pointer items-center gap-1.5 rounded-lg border border-rule bg-white px-3 py-2 text-sm text-ink/80 transition hover:border-forest-300">
          <input type="checkbox" checked={c2dOnly} onChange={(e) => setC2dOnly(e.target.checked)} className="accent-forest-600" />
          {t("仅看可信计算", "Compute-to-data only")}
        </label>
        <Button type="submit">{t("筛选", "Filter")}</Button>
      </form>

      {visible === null ? (
        <SkeletonGrid count={9} />
      ) : visible.length === 0 ? (
        <Empty>{c2dOnly ? t("暂无支持可信计算的数据集", "No compute-to-data datasets yet") : t("暂无符合条件的数据集", "No datasets match your filters")}</Empty>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {visible.map((d, i) => (
            <Reveal key={d.id} delay={(i % 3) * 60}>
              <Link href={`/datasets/${d.id}`} className="block h-full rounded-2xl">
                <Card className="lift flex h-full flex-col">
                  <div className="flex items-start justify-between gap-2">
                    <h3 className="font-display text-lg leading-snug text-ink">{d.title}</h3>
                    <Badge>{d.data_type}</Badge>
                  </div>
                  <p className="mt-2 line-clamp-2 min-h-[2.5rem] text-sm text-ink/60">
                    {d.description || t("(无描述)", "(no description)")}
                  </p>
                  <div className="mt-2 flex min-h-[1.25rem] flex-wrap items-center gap-1.5">
                    <QualityBadge band={d.authenticity_band} verified={d.quality_verified} />
                    {signals[d.id]?.enabled && <ComputeBadge sig={signals[d.id]} />}
                  </div>
                  <div className="mt-auto flex items-center justify-between pt-3">
                    <span className="font-mono text-lg font-semibold text-ink">{yuan(d.final_price_cents ?? d.suggested_price_cents)}</span>
                    <span className="text-xs text-muted">{t(`${d.sample_count} 条样本`, `${d.sample_count} samples`)}</span>
                  </div>
                </Card>
              </Link>
            </Reveal>
          ))}
        </div>
      )}
    </div>
  );
}
