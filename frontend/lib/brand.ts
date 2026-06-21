// Single source of truth for the product brand. Display/marketing copy only —
// code identifiers (Go module path, npm package name, S3 bucket, env keys) keep
// their original slug intentionally; see docs/Verdant-Oasis-品牌统一替换确认清单.md.
export const BRAND = {
  name: "Verdant Oasis（绿洲）",
  nameEn: "Verdant Oasis",
  nameZh: "绿洲",
  // Descriptive subtitle kept alongside the brand for context/SEO. Positioning is
  // locked to a "verified research-data marketplace" (see docs/战略-可信数据市场-定位与冷启动.md).
  tagline: "可信科研数据市场",
  taglineEn: "The marketplace for verified research data",
  sloganEn: "In the data desert, we build a pure oasis.",
  sloganZh: "在数据荒漠中，我们筑起一片纯净绿洲。",
  description: "每一份数据都经完整性体检 + 可溯源认证;卖方先验证再上架,买方放心用于训练与研究。科研先行。",
  descriptionEn:
    "Every dataset is integrity-screened and provenance-certified before listing — verified data you can trust for training and research. Research-first.",
  // The "pure oasis" philosophy, shown on legal pages and the landing hero.
  philosophy:
    "Verdant Oasis（绿洲）致力于在杂乱无序的「数据荒漠」中，以质量可信、来源合规、资金安全为根基，为可信数据的流通筑起一片干净、可追溯、可信赖的绿洲。",
} as const;
