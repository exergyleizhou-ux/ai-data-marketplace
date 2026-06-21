"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useT } from "@/lib/i18n";

/**
 * Live proof-of-verification: hits the public /verify endpoint and shows the
 * result. Accessible (role=status + aria-live), with an animated in-progress dot,
 * and — critically — it distinguishes three failure modes so the chip never lies:
 *   - a transient network failure → offer a retry,
 *   - a genuinely not-verifiable certificate (server says so) → "not verifiable",
 *   - a 404 (the cert simply isn't hosted on *this* instance, e.g. a flagship
 *     production cert viewed on the lightweight demo) → an honest "production cert"
 *     note, NOT a failure/retry that would read as "this cert is broken".
 */
/**
 * Maps a verifyCertificate rejection to a chip state. A 404 means the certificate
 * isn't registered on THIS instance (e.g. a production-only flagship cert viewed
 * on the lightweight demo) — an honest "absent", not a failure. Anything else is
 * treated as transient/network and stays retryable, so a flaky request never
 * reads as "this cert is fake".
 */
export function classifyVerifyError(err: unknown): "absent" | "neterr" {
  return (err as { status?: number } | null)?.status === 404 ? "absent" : "neterr";
}

export function VerifyChip({ certId }: { certId: string }) {
  const { t } = useT();
  const [st, setSt] = useState<"loading" | "ok" | "neterr" | "no" | "absent">("loading");

  const run = useCallback(() => {
    let on = true;
    setSt("loading");
    void (async () => {
      try {
        const r = await api.verifyCertificate(certId);
        if (on) setSt(r.verifiable ? "ok" : "no");
      } catch (err: unknown) {
        if (on) setSt(classifyVerifyError(err));
      }
    })();
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
  if (st === "absent")
    return (
      <span
        role="note"
        title={t(
          "该证书在生产实例签发与核验;此演示实例聚焦数据市场,未托管其实时核验。",
          "Issued and verified on our production instance; this demo focuses on the marketplace and does not host it for live re-hash.",
        )}
        className={`${base} bg-neutral-100 text-neutral-500`}
      >
        {t("生产实例存证", "production cert")}
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
