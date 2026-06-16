"use client";

// 404 page. Rendered inside the root layout (providers available) for unmatched
// routes and any explicit notFound() call.

import Link from "next/link";
import { useT } from "@/lib/i18n";

export default function NotFound() {
  const { t } = useT();
  return (
    <div className="py-24 sm:py-32">
      <p className="font-mono text-kicker uppercase text-muted">{t("错误 · 404", "Error · 404")}</p>
      <h1 className="mt-4 font-display text-display-md leading-[1.02] tracking-tight sm:text-display-lg">
        {t("页面不存在。", "Nothing here.")}
      </h1>
      <p className="mt-5 max-w-md text-base leading-relaxed text-ink/70">
        {t("你访问的页面不存在或已被移除。", "The page you're looking for doesn't exist or has been moved.")}
      </p>
      <Link
        href="/"
        className="mt-8 inline-flex items-center gap-2 rounded-full bg-ink px-5 py-2.5 text-sm font-medium text-paper hover:bg-ink/85"
      >
        {t("返回首页", "Back home")}
        <span aria-hidden>→</span>
      </Link>
    </div>
  );
}
