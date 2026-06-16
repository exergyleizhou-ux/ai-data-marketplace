"use client";

import Link from "next/link";
import { BRAND } from "@/lib/brand";
import { useT } from "@/lib/i18n";

export function SiteFooter() {
  const { t } = useT();
  return (
    <footer className="mt-24 border-t border-rule">
      <div className="mx-auto max-w-6xl px-4 py-12">
        <div className="flex flex-col gap-8 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="font-display text-2xl leading-none tracking-tight text-ink">{BRAND.nameEn}</p>
            <p className="mt-1 font-mono text-[10px] uppercase tracking-widest text-muted">{BRAND.nameZh}</p>
            <p className="mt-4 max-w-md text-sm italic text-ink/65">{BRAND.sloganEn}</p>
            <p className="text-sm text-ink/65">{BRAND.sloganZh}</p>
          </div>
          <nav className="flex flex-wrap gap-x-6 gap-y-2 text-sm text-ink/65">
            <Link href="/build" className="hover:text-ink">
              {t("用 Lumen 构建", "Build with Lumen")}
            </Link>
            <Link href="/trust" className="hover:text-ink">
              {t("可验证性 / 信任分级", "Trust & verifiability")}
            </Link>
            <Link href="/verify" className="hover:text-ink">
              {t("存证验真", "Verify certificate")}
            </Link>
            <Link href="/terms" className="hover:text-ink">
              {t("用户服务协议", "Terms")}
            </Link>
            <Link href="/privacy" className="hover:text-ink">
              {t("隐私政策", "Privacy")}
            </Link>
          </nav>
        </div>
        <p className="mt-8 border-t border-rule pt-6 font-mono text-[10px] uppercase tracking-widest text-muted">
          {t("数据基础设施 · 自 2026 · 全程留痕", "Data infrastructure · est. 2026 · fully audited")}
        </p>
      </div>
    </footer>
  );
}
