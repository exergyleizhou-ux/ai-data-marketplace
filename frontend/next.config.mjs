// Security headers for the Next-served responses (SSR pages, static assets).
// The Go backend sets its own headers on /api responses via its security
// middleware; this is the front-door equivalent for everything Next serves.
const securityHeaders = [
  { key: "Strict-Transport-Security", value: "max-age=63072000; includeSubDomains; preload" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" },
];

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // Produces a minimal standalone server bundle for the Docker runtime stage.
  // Disabled for E2E (NEXT_OUTPUT_STANDALONE=0) so `next start` can serve the
  // real production build directly without assembling the standalone tree.
  output: process.env.NEXT_OUTPUT_STANDALONE === "0" ? undefined : "standalone",
  async headers() {
    return [{ source: "/:path*", headers: securityHeaders }];
  },
};

export default nextConfig;
