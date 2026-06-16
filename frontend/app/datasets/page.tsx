"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, yuan, type Dataset } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Badge, Button, Card, Empty, Input, Select, Spinner } from "@/components/ui";
import { QualityBadge } from "@/components/QualityReport";

const DATA_TYPES = ["", "text", "code", "structured"];

export default function DatasetsPage() {
  const { t } = useT();
  const [items, setItems] = useState<Dataset[] | null>(null);
  const [q, setQ] = useState("");
  const [dataType, setDataType] = useState("");
  const [sort, setSort] = useState("newest");

  const load = useCallback(async () => {
    setItems(null);
    const res = await api.listDatasets({ q, data_type: dataType, sort, limit: 50 });
    setItems(res.items);
  }, [q, dataType, sort]);

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

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
        <Button type="submit">{t("筛选", "Filter")}</Button>
      </form>

      {items === null ? (
        <Spinner />
      ) : items.length === 0 ? (
        <Empty>{t("暂无符合条件的数据集", "No datasets match your filters")}</Empty>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {items.map((d) => (
            <Link key={d.id} href={`/datasets/${d.id}`}>
              <Card className="h-full transition hover:shadow-md">
                <div className="flex items-start justify-between gap-2">
                  <h3 className="font-semibold leading-snug">{d.title}</h3>
                  <Badge>{d.data_type}</Badge>
                </div>
                <p className="mt-2 line-clamp-2 min-h-[2.5rem] text-sm text-neutral-500">
                  {d.description || t("（无描述）", "(no description)")}
                </p>
                <div className="mt-2 min-h-[1.25rem]">
                  <QualityBadge band={d.authenticity_band} verified={d.quality_verified} />
                </div>
                <div className="mt-2 flex items-center justify-between">
                  <span className="text-lg font-semibold">{yuan(d.final_price_cents ?? d.suggested_price_cents)}</span>
                  <span className="text-xs text-neutral-400">{t(`${d.sample_count} 条样本`, `${d.sample_count} samples`)}</span>
                </div>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
