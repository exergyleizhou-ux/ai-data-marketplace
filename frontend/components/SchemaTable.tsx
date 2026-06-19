"use client";

import type { QualityCheck } from "@/lib/api";
import { useT } from "@/lib/i18n";

type Col = {
  name: string;
  type: string;
  non_null: number;
  null: number;
  distinct: number;
  distinct_capped?: boolean;
  min?: number;
  max?: number;
  mean?: number;
  max_len?: number;
  samples?: string[];
};

const TYPE_CLS: Record<string, string> = {
  integer: "bg-sky-100 text-sky-700",
  number: "bg-indigo-100 text-indigo-700",
  boolean: "bg-amber-100 text-amber-700",
  string: "bg-neutral-100 text-neutral-600",
  empty: "bg-neutral-100 text-neutral-400",
};

/** Returns the applicable schema profile from the quality checks, if any. */
export function hasSchema(checks: QualityCheck[] | null): boolean {
  return !!checks?.some(
    (c) => c.type === "schema" && (c.report as { applicable?: boolean })?.applicable,
  );
}

type Alert = { column: string; code: string; message: string };

const ALERT_CLS: Record<string, string> = {
  empty: "border-rose-200 bg-rose-50 text-rose-700",
  high_null: "border-amber-200 bg-amber-50 text-amber-700",
  constant: "border-amber-200 bg-amber-50 text-amber-700",
  unique_key: "border-sky-200 bg-sky-50 text-sky-700",
  high_cardinality: "border-neutral-200 bg-neutral-50 text-neutral-600",
};

export function SchemaTable({ checks }: { checks: QualityCheck[] }) {
  const { t } = useT();
  const schema = checks.find((c) => c.type === "schema");
  const r = schema?.report as
    | {
        applicable?: boolean;
        row_count?: number;
        column_count?: number;
        columns?: Col[];
        alerts?: Alert[];
      }
    | undefined;
  if (!schema || !r?.applicable) {
    return (
      <p className="text-sm text-neutral-400">
        非表格数据，无结构概览。<span className="text-neutral-300">Not tabular — no schema.</span>
      </p>
    );
  }
  const cols = r.columns ?? [];
  const alerts = r.alerts ?? [];
  return (
    <div>
      <p className="mb-2 text-xs text-neutral-400">
        {r.row_count} 行 · {r.column_count} 列（按样本统计 / sampled）
      </p>
      {alerts.length > 0 && (
        <div className="mb-3">
          <p className="mb-1 text-xs font-medium text-neutral-500">
            数据健康提示 <span className="font-normal text-neutral-400">/ Data-health alerts</span>
          </p>
          <ul className="flex flex-wrap gap-1.5">
            {alerts.map((a, i) => (
              <li
                key={i}
                className={`rounded border px-2 py-0.5 text-[11px] ${ALERT_CLS[a.code] ?? ALERT_CLS.high_cardinality}`}
              >
                <span className="font-medium">{a.column}</span>: {a.message}
              </li>
            ))}
          </ul>
        </div>
      )}
      <div className="overflow-x-auto">
        <table className="w-full text-xs">
          <thead>
            <tr className="text-neutral-400">
              <th className="py-1 text-left font-medium">{t("字段", "Field")}</th>
              <th className="text-left font-medium">{t("类型", "Type")}</th>
              <th className="text-right font-medium">{t("非空", "Non-null")}</th>
              <th className="text-right font-medium">{t("缺失", "Missing")}</th>
              <th className="text-right font-medium">{t("不同值", "Distinct")}</th>
              <th className="pl-3 text-left font-medium">{t("范围 / 示例", "Range / example")}</th>
            </tr>
          </thead>
          <tbody>
            {cols.map((c) => (
              <tr key={c.name} className="border-t border-neutral-100">
                <td className="py-1 font-medium text-neutral-700">{c.name}</td>
                <td>
                  <span className={`rounded px-1.5 py-0.5 ${TYPE_CLS[c.type] ?? TYPE_CLS.string}`}>{c.type}</span>
                </td>
                <td className="text-right tabular-nums">{c.non_null}</td>
                <td className="text-right tabular-nums text-neutral-400">{c.null}</td>
                <td className="text-right tabular-nums">
                  {c.distinct}
                  {c.distinct_capped ? "+" : ""}
                </td>
                <td className="max-w-[16rem] truncate pl-3 text-neutral-500">{rangeOrSample(c)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function rangeOrSample(c: Col): string {
  if (c.min !== undefined && c.max !== undefined) {
    const mean = c.mean !== undefined ? ` (μ=${c.mean})` : "";
    return `${c.min} ~ ${c.max}${mean}`;
  }
  if (c.samples && c.samples.length > 0) {
    return c.samples.join("、");
  }
  return "—";
}
