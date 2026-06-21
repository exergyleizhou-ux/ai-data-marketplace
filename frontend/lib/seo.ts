import type { Metadata } from "next";
import { BRAND } from "./brand";

// Single source for page-level SEO + social-share metadata. Kept out of layout.tsx
// so it stays free of next/font + React runtime deps and is unit-testable.
//
// metadataBase is the stable public demo domain — it's the link a founder shares in
// cold outreach, so the OpenGraph/Twitter card and canonical url must resolve there
// and must pitch the *verified research data* positioning (not the old training-data
// market copy the hero already moved away from).
const SITE_URL = "https://demo.oasisdata2026.xyz";

const title = `${BRAND.name} — ${BRAND.tagline}`;
const description = `${BRAND.description} ${BRAND.descriptionEn}`;

export const siteMetadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title,
  description,
  keywords: [
    "可信数据市场",
    "科研数据",
    "数据验证",
    "完整性体检",
    "溯源证书",
    "可用不可见",
    "verified research data",
    "data provenance",
    "dataset integrity",
  ],
  openGraph: {
    type: "website",
    siteName: BRAND.nameEn,
    url: SITE_URL,
    title,
    description,
    locale: "zh_CN",
    alternateLocale: ["en_US"],
  },
  twitter: {
    card: "summary_large_image",
    title,
    description,
  },
};
