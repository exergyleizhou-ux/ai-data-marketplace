// @vitest-environment node

import { describe, expect, it } from "vitest";
import nextConfig from "../next.config.mjs";

describe("nextConfig rewrites", () => {
  it("proxies the Lumen Lab surface and API through the frontend server", async () => {
    expect(nextConfig.rewrites).toBeTypeOf("function");
    const rewrites = await nextConfig.rewrites!();
    expect(rewrites).toEqual(
      expect.arrayContaining([
        { source: "/lumen-lab", destination: "http://127.0.0.1:18992/" },
        { source: "/lumen-lab/:path*", destination: "http://127.0.0.1:18992/:path*" },
        { source: "/api/lab/:path*", destination: "http://127.0.0.1:18992/api/lab/:path*" },
      ]),
    );
  });

  it("allows only same-origin workbench framing and denies active external content", async () => {
    expect(nextConfig.headers).toBeTypeOf("function");
    const rules = await nextConfig.headers!();
    const headers = Object.fromEntries(rules[0]!.headers.map(({ key, value }) => [key, value]));
    expect(headers["X-Frame-Options"]).toBe("SAMEORIGIN");
    expect(headers["Content-Security-Policy"]).toContain("frame-ancestors 'self'");
    expect(headers["Content-Security-Policy"]).toContain("object-src 'none'");
    expect(headers["Content-Security-Policy"]).toContain("connect-src 'self'");
  });
});
