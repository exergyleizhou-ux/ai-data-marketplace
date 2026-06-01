import Link from "next/link";
import { type ReactNode } from "react";
import { BRAND as BASE } from "@/lib/brand";

/**
 * Legal-page brand: the shared brand SoT (lib/brand.ts) plus the registered-
 * entity and contact fields the agreements need. `name` uses the plain Latin
 * form so the bilingual legal header reads "Verdant Oasis · 绿洲".
 */
export const BRAND = {
  ...BASE,
  name: BASE.nameEn,
  entity: "杭州科农绿洲生物科技有限公司",
  entityEn: "Hangzhou Kenong Oasis Biotechnology Co., Ltd.",
  uscc: "91330185MAE1A5P27M",
  addressZh: "浙江省杭州市临安区青山湖街道崇文路2088号8幢210室",
  addressEn:
    "Room 210, Building 8, No. 2088 Chongwen Road, Qingshanhu Street, Lin'an District, Hangzhou, Zhejiang Province, China",
  philosophyEn:
    "We build a pure oasis in the data desert — where clean, high-quality AI training data converges without pollution.",
  philosophyZh:
    "我们在数据荒漠中筑起一片纯净绿洲——让高质量的 AI 训练数据在此汇聚，而非相互污染。",
  effectiveDate: "2026-06-01",
  updatedDate: "2026-06-01",
  contactEmail: "legal@verdantoasis.cn",
  dpoEmail: "privacy@verdantoasis.cn",
  /** Fund custodian — placeholder until the actual licensed payment institution is confirmed. */
  custodianZh: "资金存管机构【待实际支付机构名称确定后填写】",
  custodianEn:
    "the fund-custody institution [name to be filled in once the actual payment institution is confirmed]",
} as const;

/** Green "纯净绿洲" header block shown at the top of /terms and /privacy. */
export function LegalHeader({ titleZh, titleEn }: { titleZh: string; titleEn: string }) {
  return (
    <header className="space-y-5">
      <div className="rounded-2xl border border-green-200 bg-gradient-to-br from-green-50 to-emerald-50 p-7">
        <p className="text-sm font-medium tracking-tight text-green-800">
          {BRAND.name} · {BRAND.nameZh}
        </p>
        <p className="mt-3 text-lg font-semibold leading-snug text-green-900">{BRAND.sloganEn}</p>
        <p className="text-lg font-semibold leading-snug text-green-900">{BRAND.sloganZh}</p>
        <p className="mt-4 max-w-2xl text-sm leading-relaxed text-green-800/90">{BRAND.philosophyEn}</p>
        <p className="max-w-2xl text-sm leading-relaxed text-green-800/90">{BRAND.philosophyZh}</p>
      </div>

      <div className="space-y-1">
        <h1 className="text-3xl font-semibold tracking-tight">
          {titleZh} <span className="text-neutral-400">/ {titleEn}</span>
        </h1>
        <p className="text-sm text-neutral-500">
          运营主体 Operator：{BRAND.entity}（{BRAND.entityEn}）
        </p>
        <p className="text-sm text-neutral-500">
          统一社会信用代码 USCC：{BRAND.uscc}
        </p>
        <p className="text-sm text-neutral-500">
          注册地址 Registered address：{BRAND.addressZh}
        </p>
        <p className="text-sm text-neutral-500">
          生效日期 Effective：{BRAND.effectiveDate} · 更新日期 Updated：{BRAND.updatedDate}
        </p>
      </div>

      <div className="rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-xs leading-relaxed text-amber-800">
        草稿提示：本文本为待律师审核的草稿版本，部分条款（如管辖法院、存管机构名称、跨境安排）需结合最终商业与合规安排定稿。
        <br />
        Draft notice: This text is a draft pending legal review. Certain clauses (competent court, custodian
        name, cross-border arrangements) must be finalized against the actual commercial and compliance setup.
      </div>
    </header>
  );
}

/** A numbered bilingual section. */
export function LegalSection({
  n,
  zh,
  en,
  children,
}: {
  n: number;
  zh: string;
  en: string;
  children: ReactNode;
}) {
  return (
    <section className="space-y-3">
      <h2 className="text-xl font-semibold tracking-tight">
        {n}. {zh} <span className="text-neutral-400">/ {en}</span>
      </h2>
      <div className="space-y-3 text-sm leading-relaxed text-neutral-700">{children}</div>
    </section>
  );
}

/** A bilingual paragraph: Chinese on top, English (muted) below. */
export function P({ zh, en }: { zh: string; en: string }) {
  return (
    <p>
      <span className="block text-neutral-800">{zh}</span>
      <span className="mt-1 block text-neutral-500">{en}</span>
    </p>
  );
}

/** A bilingual list. Each item is [zh, en]. */
export function List({ items }: { items: [string, string][] }) {
  return (
    <ul className="list-disc space-y-2 pl-5">
      {items.map(([zh, en], i) => (
        <li key={i}>
          <span className="block text-neutral-800">{zh}</span>
          <span className="mt-0.5 block text-neutral-500">{en}</span>
        </li>
      ))}
    </ul>
  );
}

/** Footer cross-links between the two legal docs. */
export function LegalFooterNav({ current }: { current: "terms" | "privacy" }) {
  return (
    <nav className="flex gap-4 border-t border-neutral-200 pt-6 text-sm">
      {current !== "terms" && (
        <Link href="/terms" className="font-medium text-neutral-900 underline-offset-2 hover:underline">
          用户服务协议 / Terms of Service
        </Link>
      )}
      {current !== "privacy" && (
        <Link href="/privacy" className="font-medium text-neutral-900 underline-offset-2 hover:underline">
          隐私政策 / Privacy Policy
        </Link>
      )}
    </nav>
  );
}
