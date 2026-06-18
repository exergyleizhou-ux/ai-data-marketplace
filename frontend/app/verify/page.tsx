"use client";

import { useState } from "react";
import { api } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Alert, Badge, Button, Card, Input, PageHeader } from "@/components/ui";
import { CertStatement } from "@/components/CertStatement";

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

  async function lookup(e: React.FormEvent) {
    e.preventDefault();
    if (!certId.trim()) return;
    setErr("");
    setBusy(true);
    setResult(null);
    try {
      setResult(await api.verifyCertificate(certId.trim()));
    } catch (ex) {
      setErr((ex as Error).message);
    } finally {
      setBusy(false);
    }
  }

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
      {err && <Alert>{err}</Alert>}
      {result && (
        <Card>
          <div className="space-y-3">
            <div className="flex items-center gap-2">
              <Badge>{result.status === "registered" ? t("已登记", "Registered") : result.status}</Badge>
              {result.verifiable && <Badge>{t("可验证", "Verifiable")}</Badge>}
            </div>
            <div className="grid gap-2.5 text-sm">
              <div className="flex justify-between gap-4">
                <span className="text-muted">{t("存证编号", "Cert ID")}</span>
                <span className="font-mono text-forest-700">{result.cert_id}</span>
              </div>
              <div className="flex justify-between gap-4">
                <span className="text-muted">{t("资源类型", "Resource type")}</span>
                <span>{result.resource_type}</span>
              </div>
              <div className="flex justify-between gap-4">
                <span className="text-muted">{t("资源ID", "Resource ID")}</span>
                <span className="truncate font-mono text-xs">{result.resource_id}</span>
              </div>
              <div className="flex justify-between gap-4">
                <span className="text-muted">{t("登记时间", "Registered at")}</span>
                <span className="font-mono text-xs">{result.registered_at?.slice(0, 19)}</span>
              </div>
            </div>
            <div className="border-t border-rule pt-3">
              <CertStatement zh={result.statement_zh} en={result.statement_en} />
            </div>
          </div>
        </Card>
      )}
      <p className="font-mono text-[10px] uppercase tracking-widest text-muted">
        {t(
          "绿洲 · AI 训练数据 · 一码存证 · 溯源可信",
          "Verdant Oasis · AI training data · provenance by certificate",
        )}
      </p>
    </div>
  );
}
