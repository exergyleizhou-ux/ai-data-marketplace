"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Alert, Button, Input, PageHeader } from "@/components/ui";
import { CertStatement } from "@/components/CertStatement";
import { Reveal } from "@/components/Reveal";
import { Seal } from "@/components/Seal";

export default function VerifyPage() {
  const { t } = useT();
  const [certId, setCertId] = useState("");
  const [result, setResult] = useState<{
    cert_id: string; resource_type: string; resource_id: string;
    registered_at: string; status: string; verifiable: boolean;
    statement_zh: string; statement_en: string;
  } | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  const doLookup = useCallback(async (id: string) => {
    if (!id) return;
    setErr("");
    setBusy(true);
    setResult(null);
    try {
      setResult(await api.verifyCertificate(id));
    } catch (ex) {
      setErr((ex as Error).message);
    } finally {
      setBusy(false);
    }
  }, []);

  function lookup(e: React.FormEvent) {
    e.preventDefault();
    void doLookup(certId.trim());
  }

  // Deep-link: /verify?cert=VO-... pre-fills and verifies automatically (e.g. the
  // "Verify publicly" link on a compute-result certificate card).
  useEffect(() => {
    const c = new URLSearchParams(window.location.search).get("cert");
    if (c) {
      setCertId(c);
      void doLookup(c.trim());
    }
  }, [doLookup]);

  return (
    <div className="max-w-2xl space-y-8 pb-16">
      <PageHeader
        kicker={t("公开验证 · 无需登录", "Public verification · no login")}
        title={t("存证验真", "Verify a certificate")}
        subtitle={t(
          "输入存证编号(如 VO-XXXXXXXXXXXX),独立核验一份数据或计算结果的来源——它确认输出确由声明的算法、对声明的数据产生。",
          "Enter a certificate ID (e.g. VO-XXXXXXXXXXXX) to independently check the provenance of a dataset or computation — it confirms the output came from the stated algorithm over the stated data.",
        )}
      />
      <form onSubmit={lookup} className="flex flex-col gap-2 sm:flex-row">
        <Input
          value={certId}
          onChange={(e) => setCertId(e.target.value)}
          placeholder="VO-XXXXXXXXXXXX"
          className="flex-1 font-mono text-base"
          aria-label={t("存证编号", "Certificate ID")}
        />
        <Button type="submit" disabled={busy || !certId.trim()}>
          {busy ? t("核验中…", "Verifying…") : t("验真", "Verify")}
        </Button>
      </form>
      {busy && (
        <div className="space-y-3 rounded-2xl border border-rule bg-white p-6">
          <div className="skeleton h-5 w-44" />
          <div className="skeleton h-4 w-full" />
          <div className="skeleton h-4 w-2/3" />
        </div>
      )}
      {err && (
        <Alert>
          {err}{" "}
          <button type="button" onClick={() => doLookup(certId.trim())} className="font-medium underline hover:text-ink">
            {t("重试", "retry")}
          </button>
        </Alert>
      )}
      {result && !busy && (
        <Reveal>
          <div className="elev overflow-hidden rounded-2xl border border-rule bg-white" aria-live="polite">
            <div className="h-1 bg-gradient-to-r from-gold-700 via-gold to-gold-700" />
            <div className={`relative border-b border-rule bg-paper px-6 py-5 ${result.verifiable ? "sheen" : ""}`}>
              {result.verifiable && (
                <div
                  className="pointer-events-none absolute -top-1 right-4"
                  style={{ background: "radial-gradient(closest-side, rgba(180,83,9,0.12), transparent)" }}
                >
                  <Seal size={68} label={t(`已验证封缄 · 证书 ${result.cert_id}`, `verified seal · certificate ${result.cert_id}`)} />
                </div>
              )}
              <div className={result.verifiable ? "pr-16 sm:pr-20" : ""}>
                <p className="font-mono text-kicker uppercase tracking-widest text-forest-700">{t("验真结果", "Verification result")}</p>
                <p className="mt-1 break-all font-mono text-lg font-semibold text-ink sm:text-xl">{result.cert_id}</p>
                <div className="mt-2.5 flex flex-wrap items-center gap-x-3 gap-y-1.5">
                  {result.verifiable && (
                    <span className="inline-flex items-center gap-1.5 rounded-full border border-gold-100 bg-gold-50 px-2.5 py-0.5 text-xs font-medium text-gold-700">
                      <span className="inline-block h-2 w-2 rounded-full bg-gold" aria-hidden />
                      {t("可验证", "Verifiable")}
                    </span>
                  )}
                  <span className="font-mono text-[10px] uppercase tracking-wider text-muted">
                    {result.status === "registered" ? t("已登记", "registered") : result.status}
                  </span>
                </div>
              </div>
            </div>
            <div className="space-y-2.5 px-6 py-5 text-sm">
              <div className="flex justify-between gap-4">
                <span className="text-muted">{t("资源类型", "Resource type")}</span>
                <span className="font-mono text-xs">{result.resource_type}</span>
              </div>
              {result.resource_id && (
                <div className="flex justify-between gap-4">
                  <span className="text-muted">{t("资源ID", "Resource ID")}</span>
                  <span className="truncate font-mono text-xs">{result.resource_id}</span>
                </div>
              )}
              <div className="flex justify-between gap-4">
                <span className="text-muted">{t("登记时间", "Registered at")}</span>
                <span className="font-mono text-xs">{result.registered_at?.slice(0, 19)}</span>
              </div>
              <div className="border-t border-rule pt-3">
                <CertStatement zh={result.statement_zh} en={result.statement_en} />
              </div>
            </div>
          </div>
        </Reveal>
      )}
      <p className="font-mono text-[10px] uppercase tracking-widest text-muted">
        {t(
          "绿洲 · 可信科研数据 · 一码存证 · 溯源可信",
          "Verdant Oasis · verified research data · provenance by certificate",
        )}
      </p>
    </div>
  );
}
