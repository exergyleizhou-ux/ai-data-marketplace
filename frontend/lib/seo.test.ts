import { describe, expect, it } from "vitest";
import { BRAND } from "./brand";
import { siteMetadata } from "./seo";

// The strategy is locked to a "verified research data marketplace" (可信科研数据市场).
// The hero already says this; these tests pin the brand config + the page metadata
// (browser tab, SEO, and the OpenGraph/Twitter card a founder's shared link renders)
// to the same positioning, and guard against the old "AI 训练数据交易市场" copy.
const OLD_POSITIONING = ["训练数据交易", "训练数据流通"];

describe("BRAND positioning", () => {
  it("tagline names the verified research-data marketplace, not the old training-data market", () => {
    expect(BRAND.tagline).toContain("可信");
    expect(BRAND.tagline).toContain("科研");
    for (const old of OLD_POSITIONING) expect(BRAND.tagline).not.toContain(old);
  });

  it("description leads with verification/provenance, not training-data trading", () => {
    expect(BRAND.description).toMatch(/验证|溯源|完整性|可信/);
    for (const old of OLD_POSITIONING) expect(BRAND.description).not.toContain(old);
  });

  it("carries an English positioning line for link shares in EN contexts", () => {
    expect(BRAND.taglineEn.toLowerCase()).toContain("verified research data");
  });

  it("keeps the oasis philosophy free of the old training-data category", () => {
    for (const old of OLD_POSITIONING) expect(BRAND.philosophy).not.toContain(old);
  });
});

describe("siteMetadata", () => {
  it("builds the title from the brand and new tagline", () => {
    expect(siteMetadata.title).toBe(`${BRAND.name} — ${BRAND.tagline}`);
  });

  it("sets metadataBase to the stable public domain so OG image/url resolve", () => {
    expect(String(siteMetadata.metadataBase)).toContain("demo.oasisdata2026.xyz");
  });

  it("ships an OpenGraph card so the shared link previews the right pitch", () => {
    const og = siteMetadata.openGraph as Record<string, unknown>;
    expect(og).toBeTruthy();
    expect(og.type).toBe("website");
    expect(String(og.title)).toContain(BRAND.tagline);
    expect(String(og.description)).toMatch(/验证|溯源|verified/i);
    expect(String(og.url)).toContain("demo.oasisdata2026.xyz");
  });

  it("ships a Twitter summary card with matching copy", () => {
    const tw = siteMetadata.twitter as Record<string, unknown>;
    expect(tw).toBeTruthy();
    expect(tw.card).toBe("summary_large_image");
    expect(String(tw.title)).toContain(BRAND.tagline);
  });

  it("declares research/verification SEO keywords", () => {
    const kw = (siteMetadata.keywords as string[]).join(" ").toLowerCase();
    expect(kw).toContain("可信数据市场");
    expect(kw).toContain("科研数据");
    expect(kw).toContain("verified research data");
  });
});
