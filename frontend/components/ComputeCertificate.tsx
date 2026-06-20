"use client";

import { useEffect, useRef, useState } from "react";
import { useT } from "@/lib/i18n";
import { CertStatement } from "@/components/CertStatement";
import { Seal } from "@/components/Seal";

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
    <div className="flex flex-col gap-0.5 py-1.5 sm:flex-row sm:items-start sm:gap-3">
      <span className="shrink-0 text-xs text-muted sm:w-28">{label}</span>
      <button
        type="button"
        onClick={copy}
        title={`${t("点击复制", "click to copy")} · ${value}`}
        aria-label={`${label}: ${value} — ${t("点击复制", "click to copy")}`}
        className={`min-w-0 rounded text-left text-xs text-ink/80 transition hover:text-gold-700 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ink ${
          mono ? "font-mono" : ""
        } ${wrap ? "break-all" : "truncate"}`}
      >
        {copied ? t("已复制 ✓", "copied ✓") : value}
      </button>
    </div>
  );
}

// EmbedButton copies an HTML snippet that embeds the live verify badge linking
// back to the public verification page — the shareable "verified by Oasis" loop.
// The badge is served by the backend (GET /verify/:cert_id/badge.svg) and renders
// green when the cert is registered.
function EmbedButton({ certId }: { certId: string }) {
  const { t } = useT();
  const [copied, setCopied] = useState(false);
  if (!certId) return null;
  async function copy() {
    const origin = typeof window !== "undefined" ? window.location.origin : "";
    const api = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080/api/v1";
    const id = encodeURIComponent(certId);
    const snippet =
      `<a href="${origin}/verify?cert=${id}" target="_blank" rel="noreferrer">` +
      `<img src="${api}/verify/${id}/badge.svg" alt="Oasis C2D verified — ${certId}" height="20"></a>`;
    try {
      await navigator.clipboard.writeText(snippet);
      setCopied(true);
      setTimeout(() => setCopied(false), 1400);
    } catch {
      /* clipboard unavailable — ignore */
    }
  }
  return (
    <button
      type="button"
      onClick={copy}
      title={t("复制可嵌入的验证徽章代码", "copy embeddable verify-badge code")}
      className="inline-flex items-center gap-1 rounded text-xs font-medium text-forest-700 transition hover:text-gold-700 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ink"
    >
      {copied ? t("已复制嵌入代码 ✓", "embed code copied ✓") : t("嵌入徽章 ⧉", "Embed badge ⧉")}
    </button>
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
    <div className="elev overflow-hidden rounded-2xl border border-rule bg-white">
      {/* gold foil top edge — the seal accent */}
      <div className="h-1 bg-gradient-to-r from-gold-700 via-gold to-gold-700" />
      <div className={`relative border-b border-rule bg-paper px-6 py-5 ${verifiable ? "sheen" : ""}`}>
        {verifiable && (
          <div
            className="pointer-events-none absolute -top-1 right-3 sm:right-4"
            style={{ background: "radial-gradient(closest-side, rgba(180,83,9,0.12), transparent)" }}
          >
            <Seal size={72} label={t(`已验证封缄 · 证书 ${certId}`, `verified seal · certificate ${certId}`)} />
          </div>
        )}
        <div className={verifiable ? "pr-16 sm:pr-20" : ""}>
          <p className="font-mono text-kicker uppercase tracking-widest text-forest-700">
            {t("计算结果存证 · 可用不可见", "Compute-to-data certificate")}
          </p>
          <p className="mt-1 break-all font-mono text-lg font-semibold text-ink sm:text-xl">{certId || "—"}</p>
          <div className="mt-2.5 flex flex-wrap items-center gap-x-3 gap-y-1.5">
            {verifiable && (
              <span className="inline-flex items-center gap-1.5 rounded-full border border-gold-100 bg-gold-50 px-2.5 py-0.5 text-xs font-medium text-gold-700">
                <span className="inline-block h-2 w-2 rounded-full bg-gold" aria-hidden />
                {t("可验证", "Verifiable")}
              </span>
            )}
            <span className="font-mono text-[10px] uppercase tracking-wider text-muted">{s(cert["status"]) || "registered"}</span>
            <span className="font-mono text-[10px] uppercase tracking-wider text-muted">{hashAlgo}</span>
          </div>
        </div>
      </div>

      <div className="space-y-3 px-6 py-5">
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
          <div className="flex items-center gap-3">
            <a href={`/verify?cert=${encodeURIComponent(certId)}`} className="text-xs font-medium text-forest-700 hover:underline">
              {t("公开验证 →", "Verify publicly →")}
            </a>
            <EmbedButton certId={certId} />
          </div>
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
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null;
    ref.current?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      prev?.focus?.();
    };
  }, [onClose]);

  return (
    <div
      onClick={onClose}
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-ink/40 p-4 sm:items-center"
    >
      <div
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-label={t(`计算结果存证 ${s(cert["certificate_id"])}`, `Compute-to-data certificate ${s(cert["certificate_id"])}`)}
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-md outline-none"
      >
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
