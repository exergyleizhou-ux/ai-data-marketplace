"use client";

import { useState } from "react";
import type { QualityCheck } from "@/lib/api";

/** Bilingual buyer-facing quality report. Renders the persisted quality_check
 *  rows: the authenticity score + band, the de-identification proof, and the
 *  remaining checks — framed as signals, never verdicts. */

const BAND: Record<string, { zh: string; en: string; cls: string }> = {
  clean: { zh: "纯净绿洲", en: "Clean", cls: "border-green-200 bg-green-50 text-green-800" },
  review: { zh: "建议复核", en: "Review", cls: "border-amber-200 bg-amber-50 text-amber-800" },
  suspect: { zh: "显著异常", en: "Suspect", cls: "border-rose-200 bg-rose-50 text-rose-800" },
};

const RESULT: Record<string, { zh: string; cls: string }> = {
  pass: { zh: "通过", cls: "text-green-700" },
  warn: { zh: "提示", cls: "text-amber-700" },
  fail: { zh: "未通过", cls: "text-rose-700" },
};

const SEV: Record<string, string> = {
  high: "bg-rose-500",
  medium: "bg-amber-500",
  low: "bg-yellow-400",
  info: "bg-neutral-300",
};

const CHECK_LABEL: Record<string, [string, string]> = {
  format: ["格式校验", "Format"],
  stats: ["基础统计", "Statistics"],
  dedup: ["去重检测", "De-duplication"],
  pii: ["个人信息扫描", "PII scan"],
};

const num = (v: unknown): number | undefined => (typeof v === "number" ? v : undefined);
const str = (v: unknown): string => (typeof v === "string" ? v : "");
const arr = (v: unknown): Record<string, unknown>[] =>
  Array.isArray(v) ? (v as Record<string, unknown>[]) : [];

export function QualityReport({ checks }: { checks: QualityCheck[] }) {
  if (!checks || checks.length === 0) {
    return (
      <p className="text-sm text-neutral-400">
        质检尚未完成。<span className="text-neutral-300">Quality screening not yet available.</span>
      </p>
    );
  }
  const byType: Record<string, QualityCheck> = {};
  for (const c of checks) byType[c.type] = c;

  return (
    <div className="space-y-5">
      <div className="rounded-md border border-neutral-200 bg-neutral-50 px-4 py-3 text-xs leading-relaxed text-neutral-500">
        以下为统计信号，非质量结论。每条发现均附「无辜解释」与方法引用，仅供您独立判断。
        <br />
        These are statistical signals, not verdicts — each finding lists innocent explanations and a
        method reference for your own judgement.
      </div>

      {byType.authenticity && <AuthenticityCard check={byType.authenticity} />}
      {byType.pii_redaction && <RedactionRow check={byType.pii_redaction} />}

      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {["format", "stats", "dedup", "pii"]
          .filter((t) => byType[t])
          .map((t) => (
            <CheckChip key={t} check={byType[t]} />
          ))}
      </div>
    </div>
  );
}

function AuthenticityCard({ check }: { check: QualityCheck }) {
  const r = check.report || {};
  const applicable = r.applicable !== false;
  const score = num(r.score);
  const band = str(r.band) || "clean";
  const findings = arr(r.findings);
  const b = BAND[band] ?? BAND.clean;

  if (!applicable) {
    return (
      <div className="rounded-lg border border-neutral-200 p-4">
        <Title zh="数据真实性筛查" en="Data authenticity" />
        <p className="mt-1 text-sm text-neutral-500">
          该数据集非表格格式，跳过统计真实性筛查。
          <span className="text-neutral-400"> Non-tabular — statistical screening skipped.</span>
        </p>
      </div>
    );
  }

  return (
    <div className={`rounded-lg border p-4 ${b.cls}`}>
      <div className="flex items-center justify-between">
        <Title zh="数据真实性分" en="Authenticity score" />
        <div className="flex items-center gap-2">
          {score !== undefined && <span className="text-2xl font-semibold tabular-nums">{score}</span>}
          <span className="rounded-full border bg-white/60 px-2.5 py-0.5 text-xs font-medium">
            {b.zh} · {b.en}
          </span>
        </div>
      </div>
      {findings.length > 0 ? (
        <ul className="mt-3 space-y-2">
          {findings
            .filter((f) => f.significant)
            .map((f, i) => (
              <Finding key={i} f={f} />
            ))}
          {findings.filter((f) => f.significant).length === 0 && (
            <li className="text-sm text-green-700/80">未发现显著统计异常。No significant anomalies.</li>
          )}
        </ul>
      ) : (
        <p className="mt-2 text-sm text-green-700/80">未发现显著统计异常。No significant anomalies.</p>
      )}
    </div>
  );
}

function Finding({ f }: { f: Record<string, unknown> }) {
  const [open, setOpen] = useState(false);
  const sev = str(f.severity) || "info";
  const innocent = (Array.isArray(f.innocent_explanations) ? f.innocent_explanations : []) as string[];
  const padj = num(f.p_value_adjusted);
  return (
    <li className="rounded-md bg-white/70 px-3 py-2 text-sm">
      <div className="flex items-center gap-2">
        <span className={`inline-block h-2 w-2 rounded-full ${SEV[sev] ?? SEV.info}`} />
        <span className="font-medium text-neutral-800">
          {str(f.detector_name) || str(f.detector)}
        </span>
        {str(f.column) && <span className="text-xs text-neutral-400">· {str(f.column)}</span>}
        {padj !== undefined && (
          <span className="ml-auto text-xs tabular-nums text-neutral-400">p≈{padj.toFixed(3)}</span>
        )}
      </div>
      {str(f.summary) && <p className="mt-1 text-neutral-600">{str(f.summary)}</p>}
      {innocent.length > 0 && (
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          className="mt-1 text-xs text-neutral-500 underline-offset-2 hover:underline"
        >
          {open ? "收起无辜解释" : `无辜解释 / innocent explanations (${innocent.length})`}
        </button>
      )}
      {open && (
        <ul className="mt-1 list-disc space-y-0.5 pl-5 text-xs text-neutral-500">
          {innocent.map((e, i) => (
            <li key={i}>{e}</li>
          ))}
        </ul>
      )}
      {str(f.reference) && (
        <p className="mt-1 text-xs text-neutral-400">方法 / method: {str(f.reference)}</p>
      )}
    </li>
  );
}

function RedactionRow({ check }: { check: QualityCheck }) {
  const r = check.report || {};
  const verified = r.verified === true;
  const detected = num(r.detected_total) ?? 0;
  const residual = num(r.residual_total) ?? 0;
  return (
    <div className="flex items-start gap-3 rounded-lg border border-neutral-200 p-4">
      <span className={`mt-0.5 text-lg ${verified ? "text-green-600" : "text-rose-600"}`}>
        {verified ? "✓" : "✕"}
      </span>
      <div className="text-sm">
        <Title zh="去标识化校验" en="De-identification proof" />
        <p className="mt-1 text-neutral-600">
          {detected === 0
            ? "未检出个人信息。No personal information detected."
            : verified
              ? `检出并脱敏 ${detected} 处个人信息，复扫零残留。Detected & redacted ${detected}; zero residual on re-scan.`
              : `脱敏后仍有 ${residual} 处残留，需卖方重新处理。${residual} residual after redaction.`}
        </p>
      </div>
    </div>
  );
}

function CheckChip({ check }: { check: QualityCheck }) {
  const label = CHECK_LABEL[check.type] ?? [check.type, check.type];
  const res = RESULT[check.result] ?? { zh: check.result, cls: "text-neutral-600" };
  const piiTotal = check.type === "pii" ? num(check.report?.total) : undefined;
  return (
    <div className="rounded-md border border-neutral-200 px-3 py-2 text-xs">
      <div className="text-neutral-500">{label[0]}</div>
      <div className="text-[10px] text-neutral-400">{label[1]}</div>
      <div className={`mt-1 font-medium ${res.cls}`}>
        {res.zh}
        {piiTotal !== undefined && piiTotal > 0 ? `（${piiTotal}）` : ""}
      </div>
    </div>
  );
}

function Title({ zh, en }: { zh: string; en: string }) {
  return (
    <h3 className="text-sm font-semibold text-neutral-800">
      {zh} <span className="font-normal text-neutral-400">/ {en}</span>
    </h3>
  );
}
