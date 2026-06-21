// Regenerate the OpenGraph/Twitter link-preview card: app/opengraph-image.png
// (1200x630). Run:  node scripts/gen-og-image.mjs
//
// The card is the visual a founder's shared link renders in email / WeChat / X /
// Slack, so it carries the locked "verified research data" positioning + a real,
// re-hashable cert as an honest proof point. Rendered with sharp (already a dep);
// CJK stays in a sans family so glyphs are guaranteed across render hosts.
import sharp from "sharp";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const __dirname = dirname(fileURLToPath(import.meta.url));
const OUT = join(__dirname, "..", "app", "opengraph-image.png");

const PAPER = "#fafaf7";
const INK = "#18181b";
const FOREST = "#065f46";
const MUTED = "#6f6f69";
const RULE = "#e6e4dd";
const TINT = "#eef4f0";

const SANS = "'PingFang SC','Hiragino Sans GB','Microsoft YaHei','Helvetica Neue',Helvetica,Arial,sans-serif";
const SERIF = "Georgia,'Times New Roman',serif";
const MONO = "'SF Mono',Menlo,'DejaVu Sans Mono','Courier New',monospace";

const svg = `<svg width="1200" height="630" viewBox="0 0 1200 630" xmlns="http://www.w3.org/2000/svg">
  <rect width="1200" height="630" fill="${PAPER}"/>
  <rect x="0" y="0" width="14" height="630" fill="${FOREST}"/>

  <!-- verified seal, top-right -->
  <circle cx="1066" cy="128" r="66" fill="${TINT}" stroke="${FOREST}" stroke-width="3"/>
  <text x="1066" y="118" font-family="${SANS}" font-size="46" fill="${FOREST}" text-anchor="middle">✓</text>
  <text x="1066" y="156" font-family="${SANS}" font-size="17" letter-spacing="1" fill="${FOREST}" text-anchor="middle">已验证</text>

  <!-- kicker -->
  <text x="80" y="120" font-family="${MONO}" font-size="23" letter-spacing="3" fill="${FOREST}">THE VERIFIED RESEARCH-DATA MARKETPLACE · 科研先行</text>

  <!-- headline -->
  <text x="78" y="240" font-family="${SANS}" font-size="82" font-weight="600" fill="${INK}">每一份数据，都经过验证。</text>
  <text x="80" y="320" font-family="${SERIF}" font-style="italic" font-size="58" fill="${INK}" opacity="0.78">Every dataset, verified.</text>

  <!-- subline -->
  <text x="80" y="392" font-family="${SANS}" font-size="29" fill="${MUTED}">完整性体检 + 可溯源认证 — integrity-screened &amp; provenance-certified</text>

  <!-- divider -->
  <rect x="80" y="468" width="1040" height="1.5" fill="${RULE}"/>

  <!-- footer: brand + domain -->
  <text x="80" y="528" font-family="${SERIF}" font-size="36" fill="${INK}">Verdant Oasis（绿洲）</text>
  <text x="1120" y="528" font-family="${MONO}" font-size="26" fill="${FOREST}" text-anchor="end">demo.oasisdata2026.xyz</text>

  <!-- honest proof point: a real, re-hashable certificate -->
  <text x="80" y="572" font-family="${MONO}" font-size="20" fill="${MUTED}">证书 CERT VO-3D77D6E1E44C · sha256 9b0eec98… · 任何人可重算验证</text>
</svg>`;

const png = await sharp(Buffer.from(svg)).png().toBuffer();
await sharp(png).toFile(OUT);
const meta = await sharp(OUT).metadata();
console.log(`wrote ${OUT}  ${meta.width}x${meta.height}  ${(png.length / 1024).toFixed(1)}KB`);
