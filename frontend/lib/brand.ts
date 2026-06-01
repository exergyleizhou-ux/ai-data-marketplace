// Single source of truth for the product brand. Display/marketing copy only —
// code identifiers (Go module path, npm package name, S3 bucket, env keys) keep
// their original slug intentionally; see docs/Verdant-Oasis-品牌统一替换确认清单.md.
export const BRAND = {
  name: "Verdant Oasis（绿洲）",
  nameEn: "Verdant Oasis",
  nameZh: "绿洲",
  // Descriptive subtitle kept alongside the brand for context/SEO.
  tagline: "AI 训练数据交易市场",
  sloganEn: "In the data desert, we build a pure oasis.",
  sloganZh: "在数据荒漠中，我们筑起一片纯净绿洲。",
  description: "高信任、可追溯、合规的 AI 训练数据流通平台。",
  // The "pure oasis" philosophy, shown on legal pages and the landing hero.
  philosophy:
    "Verdant Oasis（绿洲）致力于在杂乱无序的「数据荒漠」中，以质量可信、来源合规、资金安全为根基，为 AI 训练数据的流通筑起一片干净、可追溯、可信赖的绿洲。",
} as const;
