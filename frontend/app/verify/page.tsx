"use client";

import { useState } from "react";
import { api } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Alert, Badge, Button, Card, Input } from "@/components/ui";

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
    <div className="mx-auto max-w-xl space-y-6 py-8">
      <h1 className="text-2xl font-semibold">
        {t("存证验证", "Certificate verification")}
      </h1>
      <p className="text-sm text-neutral-500">
        {t(
          "输入存证编号（如 VO-XXXXXXXXXXXX）验证数据或计算结果的上链存证。",
          "Enter a certificate ID (e.g. VO-XXXXXXXXXXXX) to verify a provenance record.",
        )}
      </p>
      <form onSubmit={lookup} className="flex gap-2">
        <Input
          value={certId}
          onChange={(e) => setCertId(e.target.value)}
          placeholder="VO-XXXXXXXXXXXX"
          className="font-mono flex-1"
        />
        <Button type="submit" disabled={busy || !certId.trim()}>
          {t("验证", "Verify")}
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
            <div className="grid gap-2 text-sm">
              <div className="flex justify-between">
                <span className="text-neutral-500">{t("存证编号", "Cert ID")}</span>
                <span className="font-mono">{result.cert_id}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-neutral-500">{t("资源类型", "Resource type")}</span>
                <span>{result.resource_type}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-neutral-500">{t("资源ID", "Resource ID")}</span>
                <span className="font-mono text-xs">{result.resource_id}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-neutral-500">{t("登记时间", "Registered at")}</span>
                <span>{result.registered_at?.slice(0, 19)}</span>
              </div>
            </div>
            <div className="border-t border-neutral-100 pt-3">
              <p className="text-xs leading-relaxed text-neutral-500">{result.statement_zh}</p>
              <p className="mt-1 text-[11px] leading-relaxed text-neutral-400">{result.statement_en}</p>
            </div>
          </div>
        </Card>
      )}
      <p className="text-xs text-neutral-300 text-center">
        {t(
          "绿洲平台 · AI训练数据交易 · 一码存证 · 溯源可信",
          "Verdant Oasis · AI training data marketplace · provenance by certificate",
        )}
      </p>
    </div>
  );
}
