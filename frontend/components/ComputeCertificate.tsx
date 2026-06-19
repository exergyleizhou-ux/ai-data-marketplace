"use client";

import { useEffect, useState } from "react";
import { useT } from "@/lib/i18n";
import { Badge } from "@/components/ui";
import { CertStatement } from "@/components/CertStatement";

type CertData = Record<string, unknown>;
const s = (v: unknown): string => (typeof v === "string" ? v : v == null ? "" : String(v));

// A copyable certificate field. Clicking copies the full value to the clipboard.
function CertField({ label, value, mono, wrap }: { label: string; value: string; mono?: boolean; wrap?: boolean }) {
  const { t } = useT();
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 1200);
    } catch {
      /* clipboard unavailable — ignore */
    }
  }
  if (!value) return null;
  return (
    <div className="flex items-start justify-between gap-3 py-1">
      <span className="shrink-0 text-xs text-muted">{label}</span>
      <button
        type="button"
        onClick={copy}
        title={`${t("点击复制", "click to copy")} · ${value}`}
        className={`text-right text-xs text-ink/80 transition hover:text-forest-700 ${mono ? "font-mono" : ""} ${
          wrap ? "break-all" : "max-w-[62%] truncate"
        }`}
      >
        {copied ? t("已复制 ✓", "copied ✓") : value}
      </button>
    </div>
  );
}

// ComputeCertificateCard renders the full provenance & integrity certificate for a
// released compute-to-data result as a shareable credential: the VO-<id>, the
// binding of the output fingerprint to the audited (pinned-digest) algorithm and
// the source dataset, the issuer, and the bilingual statement.
export function ComputeCertificateCard({ cert }: { cert: CertData }) {
  const { t } = useT();
  const certId = s(cert["certificate_id"]);
  const algo = (cert["algorithm"] as CertData) ?? {};
  const integrity = (cert["integrity"] as CertData) ?? {};
  const verifiable = Boolean(integrity["verifiable"]);
  const hashAlgo = s(integrity["algorithm"]) || "SHA-256";
  const outputSha = s(cert["output_sha256"]);
  const outBytes = s(cert["output_bytes"]);

  return (
    <div className="overflow-hidden rounded-2xl border border-rule bg-white shadow-sm">
      <div className="border-b border-rule bg-paper px-5 py-4">
        <p className="font-mono text-kicker uppercase tracking-widest text-forest-700">
          {t("计算结果存证 · 可用不可见", "Compute-to-data certificate")}
        </p>
        <p className="mt-1 break-all font-mono text-lg font-semibold text-ink">{certId || "—"}</p>
        <div className="mt-2 flex flex-wrap gap-1.5">
          <Badge>{s(cert["status"]) || "registered"}</Badge>
          {verifiable && (
            <span className="inline-block rounded-full bg-emerald-50 px-2.5 py-0.5 text-xs font-medium text-emerald-700">
              {t("可验证", "Verifiable")}
            </span>
          )}
          <span className="inline-block rounded-full bg-neutral-100 px-2.5 py-0.5 text-xs font-medium text-neutral-600">
            {hashAlgo}
          </span>
        </div>
      </div>

      <div className="space-y-3 px-5 py-4">
        <p className="text-xs leading-relaxed text-muted">
          {t(
            "本凭证把计算结果的内容指纹绑定到产出它的已审核算法(镜像 digest 钉死)与源数据集——结果可用,原始数据不可见。",
            "This certificate binds the result's content fingerprint to the audited algorithm that produced it (pinned image digest) and the source dataset — the result is usable, the raw data stays unseen.",
          )}
        </p>

        <div className="rounded-xl border border-rule bg-paper/50 p-3">
          <CertField
            label={t("输出指纹", "Output fingerprint")}
            value={outputSha ? `${hashAlgo.toLowerCase()}:${outputSha}` : ""}
            mono
            wrap
          />
          <div className="my-2 flex items-center gap-2 text-[10px] uppercase tracking-wider text-muted">
            <span className="h-px flex-1 bg-rule" />
            {t("由…产出", "produced by")}
            <span className="h-px flex-1 bg-rule" />
          </div>
          <CertField label={t("已审核算法", "Audited algorithm")} value={s(algo["name"])} />
          <CertField label={t("镜像 digest", "Image digest")} value={s(algo["image_digest"])} mono wrap />
          <CertField label={t("源数据集", "Source dataset")} value={s(cert["dataset_id"])} mono />
        </div>

        <div>
          <CertField label={t("出具方", "Issuer")} value={s(cert["operator"])} />
          <CertField label={t("登记时间", "Registered")} value={s(cert["registered_at"]).slice(0, 19)} mono />
          {outBytes && <CertField label={t("输出大小", "Output size")} value={`${outBytes} B`} mono />}
        </div>

        <div className="border-t border-rule pt-3">
          <CertStatement zh={s(cert["statement_zh"])} en={s(cert["statement_en"])} />
        </div>

        <div className="flex flex-wrap items-center justify-between gap-2 pt-1">
          <a href={`/verify?cert=${encodeURIComponent(certId)}`} className="text-xs font-medium text-forest-700 hover:underline">
            {t("公开验证 →", "Verify publicly →")}
          </a>
          <span className="text-[10px] text-muted">
            {t("可对下载结果重算 SHA-256 与本凭证比对", "Re-hash the downloaded result and compare")}
          </span>
        </div>
      </div>
    </div>
  );
}

// ComputeCertificateModal overlays the certificate card. Backdrop click, the
// Close button, or Escape all dismiss it.
export function ComputeCertificateModal({ cert, onClose }: { cert: CertData; onClose: () => void }) {
  const { t } = useT();
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div
      role="dialog"
      aria-modal="true"
      onClick={onClose}
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-ink/40 p-4 sm:items-center"
    >
      <div onClick={(e) => e.stopPropagation()} className="w-full max-w-md">
        <ComputeCertificateCard cert={cert} />
        <div className="mt-2 text-center">
          <button
            type="button"
            onClick={onClose}
            className="rounded-full bg-white/90 px-4 py-1.5 text-xs text-ink/70 transition hover:bg-white"
          >
            {t("关闭", "Close")}
          </button>
        </div>
      </div>
    </div>
  );
}
