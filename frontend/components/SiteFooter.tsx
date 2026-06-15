"use client";

import Link from "next/link";
import { BRAND } from "@/lib/brand";
import { useT } from "@/lib/i18n";

export function SiteFooter() {
  const { t } = useT();
  return (
    <footer className="mx-auto max-w-6xl px-4 py-8 text-sm text-neutral-500">
      <p className="font-medium text-neutral-700">{BRAND.name}</p>
      <p className="mt-1 italic">{BRAND.sloganEn}</p>
      <p>{BRAND.sloganZh}</p>
      <p className="mt-3">
        <Link href="/terms" className="hover:underline">
          {t("用户服务协议", "Terms of Service")}
        </Link>
        <span className="mx-2">·</span>
        <Link href="/privacy" className="hover:underline">
          {t("隐私政策", "Privacy Policy")}
        </Link>
        <span className="mx-2">·</span>
        <Link href="/verify" className="hover:underline">
          {t("存证验真", "Verify certificate")}
        </Link>
      </p>
    </footer>
  );
}
