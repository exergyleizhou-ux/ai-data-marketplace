// Security headers for the Next-served responses (SSR pages, static assets).
// The Go backend sets its own headers on /api responses via its security
// middleware; this is the front-door equivalent for everything Next serves.
const securityHeaders = [
  { key: "Strict-Transport-Security", value: "max-age=63072000; includeSubDomains; preload" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  // Workbench surfaces are deliberately same-origin framed. DENY would break
  // that contract; SAMEORIGIN plus frame-ancestors 'self' rejects every
  // cross-origin embedding attempt.
  { key: "X-Frame-Options", value: "SAMEORIGIN" },
  { key: "Content-Security-Policy", value: `default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'self'; frame-src 'self'; form-action 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'${process.env.E2E_ALLOW_HTTP === "1" ? "" : "; upgrade-insecure-requests"}` },
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
  async rewrites() {
    // Dev convenience: proxy Lumen services through the Next dev server.
    // Production routes /lumen/* and /lumen-science/* at nginx/Caddy instead.
    const lumenServe = process.env.LUMEN_SERVE_URL || "http://127.0.0.1:8787";
    const lumenScience = process.env.LUMEN_SCIENCE_URL || "http://127.0.0.1:18990";
    const lumenLab = process.env.LUMEN_LAB_URL || "http://127.0.0.1:18992";
    const backend = process.env.BACKEND_API_BASE_URL || "http://127.0.0.1:8080/api/v1";
    return [
      { source: "/api/v1/:path*", destination: `${backend}/:path*` },
      { source: "/lumen", destination: `${lumenServe}/` },
      { source: "/lumen/:path*", destination: `${lumenServe}/:path*` },
      { source: "/lumen-science", destination: `${lumenScience}/` },
      { source: "/lumen-science/:path*", destination: `${lumenScience}/:path*` },
      { source: "/lumen-lab", destination: `${lumenLab}/` },
      { source: "/lumen-lab/:path*", destination: `${lumenLab}/:path*` },
      { source: "/api/lab/:path*", destination: `${lumenLab}/api/lab/:path*` },
    ];
  },
};

export default nextConfig;
