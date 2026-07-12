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
});
