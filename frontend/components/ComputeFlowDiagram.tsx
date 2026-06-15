"use client";

import { useT } from "@/lib/i18n";

// ComputeFlowDiagram is the visual anchor for the signature "available-but-
// invisible" flow: data stays in the seller's domain, an audited algorithm runs
// next to it inside the platform sandbox, and only the result (with a
// certificate) leaves. Themed to the app palette (neutral + emerald); bilingual.
export function ComputeFlowDiagram() {
  const { t } = useT();
  return (
    // overflow-x-auto on the wrapper + fixed SVG width = real horizontal scroll on
    // mobile (labels stay legible). On desktop the inner div is wider than 720
    // anyway so it sits centered. The fade gradient on the right edge hints to
    // mobile users that more content is scrollable; it disappears when the SVG
    // fits in view because the wrapper's inner width then matches its container.
    <div className="relative">
    <div className="overflow-x-auto">
    <svg
      viewBox="0 0 720 250"
      width="720"
      height="250"
      role="img"
      aria-label={t(
        "可用不可见沙箱计算流程:数据留在卖家域,算法在数据旁运行,买家只取结果",
        "Available-but-invisible flow: data stays with the seller, the algorithm runs next to it, the buyer takes only the result",
      )}
      // No max-w-full: the wrapper above is overflow-x-auto, so we WANT the SVG
      // to keep its intrinsic 720px and let the wrapper scroll on narrow screens.
      // mx-auto centers it on desktop where the wrapper is wider than 720.
      className="mx-auto block"
    >
      <defs>
        <marker id="cfd-arrow" markerWidth="9" markerHeight="9" refX="7" refY="3" orient="auto">
          <path d="M0,0 L7,3 L0,6 Z" fill="#059669" />
        </marker>
      </defs>

      {/* Stage A: seller data */}
      <rect x="14" y="56" width="190" height="92" rx="10" fill="#ffffff" stroke="#e5e5e5" />
      <text x="109" y="90" textAnchor="middle" fontSize="15" fontWeight="500" fill="#171717">
        {t("数据留在卖家域", "Data stays with the seller")}
      </text>
      <text x="109" y="112" textAnchor="middle" fontSize="12" fill="#737373">
        {t("从不下载 · 不外传", "never downloaded or shipped")}
      </text>
      <text x="109" y="132" textAnchor="middle" fontSize="12" fill="#737373">
        🔒 {t("原始数据", "raw data")}
      </text>

      <line x1="206" y1="102" x2="252" y2="102" stroke="#059669" strokeWidth="1.5" markerEnd="url(#cfd-arrow)" />

      {/* Stage B: platform sandbox (dashed = boundary) */}
      <rect x="256" y="40" width="208" height="124" rx="10" fill="none" stroke="#059669" strokeWidth="1.5" strokeDasharray="5 4" />
      <text x="360" y="33" textAnchor="middle" fontSize="12" fill="#047857">
        {t("平台沙箱 · 数据不出域", "Platform sandbox · data stays home")}
      </text>
      <rect x="280" y="60" width="160" height="84" rx="8" fill="#ffffff" stroke="#e5e5e5" />
      <text x="360" y="92" textAnchor="middle" fontSize="15" fontWeight="500" fill="#171717">
        {t("算法跑在数据旁", "Algorithm runs by the data")}
      </text>
      <text x="360" y="114" textAnchor="middle" fontSize="12" fill="#737373">
        {t("经审核的代码", "audited code")}
      </text>
      <text x="360" y="132" textAnchor="middle" fontSize="12" fill="#737373">
        {t("差分隐私 · 输出闸门", "DP noise · output gate")}
      </text>

      <line x1="466" y1="102" x2="512" y2="102" stroke="#059669" strokeWidth="1.5" markerEnd="url(#cfd-arrow)" />

      {/* Stage C: buyer result */}
      <rect x="516" y="56" width="190" height="92" rx="10" fill="#ffffff" stroke="#e5e5e5" />
      <text x="611" y="90" textAnchor="middle" fontSize="15" fontWeight="500" fill="#171717">
        {t("买家只取结果", "Buyer takes only the result")}
      </text>
      <text x="611" y="112" textAnchor="middle" fontSize="12" fill="#737373">
        {t("模型 / 指标", "model / metrics")}
      </text>
      <text x="611" y="132" textAnchor="middle" fontSize="12" fill="#737373">
        📄 {t("存证凭证 · 可验真", "certificate · verifiable")}
      </text>

      {/* caption */}
      <text x="360" y="196" textAnchor="middle" fontSize="12.5" fill="#737373">
        {t(
          "原始数据从不离开沙箱 · 只有计算结果出域 · 输出绑定算法镜像 digest",
          "Raw data never leaves the sandbox · only the result exits · output bound to the algorithm image digest",
        )}
      </text>
      <text x="360" y="222" textAnchor="middle" fontSize="12.5" fill="#737373">
        {t(
          "L1 买方不可见 · L3 数据不出域(联邦 / PSI) · L2 连平台也不可见(TEE)",
          "L1 invisible to the buyer · L3 data-stays-home (federated / PSI) · L2 invisible to the platform too (TEE)",
        )}
      </text>
    </svg>
    </div>
    {/* Right-edge fade hint: visible only when content overflows, since on a
        wide enough viewport the gradient sits over white SVG that already ends. */}
    <div
      aria-hidden="true"
      className="pointer-events-none absolute right-0 top-0 h-full w-8 bg-gradient-to-l from-white sm:hidden"
    />
    </div>
  );
}
