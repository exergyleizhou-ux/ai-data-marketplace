"use client";

import { useT } from "@/lib/i18n";

// CertStatement renders a bilingual certificate provenance statement, leading
// with the statement in the active UI language. Previously the verify and
// dataset pages always showed the Chinese statement first (and the dataset page
// never showed the English one), so English users saw Chinese on a
// trust-critical surface.
export function CertStatement({
  zh,
  en,
  className = "text-xs leading-relaxed text-ink/70",
  showSecondary = true,
}: {
  zh?: string;
  en?: string;
  className?: string;
  showSecondary?: boolean;
}) {
  const { lang } = useT();
  const primary = lang === "en" ? en : zh;
  const secondary = lang === "en" ? zh : en;
  return (
    <>
      <p className={className}>{primary}</p>
      {showSecondary && secondary ? (
        <p className="mt-1 text-[11px] leading-relaxed text-muted">{secondary}</p>
      ) : null}
    </>
  );
}
