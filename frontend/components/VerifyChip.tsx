"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useT } from "@/lib/i18n";

/**
 * Live proof-of-verification: hits the public /verify endpoint and shows the
 * result. Accessible (role=status + aria-live), with an animated in-progress dot,
 * and — critically — it distinguishes a transient network failure (offer a retry)
 * from a genuinely not-verifiable certificate, so a flaky request never reads as
 * "this cert is fake".
 */
export function VerifyChip({ certId }: { certId: string }) {
  const { t } = useT();
  const [st, setSt] = useState<"loading" | "ok" | "neterr" | "no">("loading");

  const run = useCallback(() => {
    let on = true;
    setSt("loading");
    api
      .verifyCertificate(certId)
      .then((r) => {
        if (on) setSt(r.verifiable ? "ok" : "no");
      })
      .catch(() => {
        if (on) setSt("neterr");
      });
    return () => {
      on = false;
    };
  }, [certId]);

  useEffect(() => run(), [run]);

  const base = "inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-medium";

  if (st === "loading")
    return (
      <span role="status" aria-live="polite" className={`${base} bg-neutral-100 text-ink/60`}>
        <span className="dot-pulse inline-block h-1.5 w-1.5 rounded-full bg-forest" aria-hidden />
        {t("核验中…", "checking…")}
      </span>
    );
  if (st === "ok")
    return (
      <span
        role="status"
        aria-live="polite"
        aria-label={t(`证书 ${certId} 已实时验证`, `certificate ${certId} verified live`)}
        className={`${base} chip-pop bg-forest-50 text-forest-700`}
      >
        {t("实时可验证 ✓", "live · verifiable ✓")}
      </span>
    );
  if (st === "no")
    return (
      <span role="status" className={`${base} bg-neutral-100 text-neutral-500`}>
        {t("未通过验证", "not verifiable")}
      </span>
    );
  return (
    <button
      type="button"
      onClick={run}
      className={`${base} bg-gold-50 text-gold-700 transition hover:bg-gold-100`}
      aria-label={t("验证失败,点击重试", "verification failed, click to retry")}
    >
      {t("验证失败 · 重试", "couldn't verify · retry")}
    </button>
  );
}
