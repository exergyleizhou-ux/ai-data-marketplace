// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { GET } from "./route";

afterEach(() => { vi.unstubAllGlobals(); delete process.env.WORKBENCH_DATABASE_URL; delete process.env.LUMEN_PROVIDER_CONFIGURED; delete process.env.STORAGE_DRIVER; delete process.env.COMPUTE_RUNNER; });
describe("workbench status", () => {
  it("fails closed without the workbench session", async () => { expect((await GET(new NextRequest("https://oasis.test/api/workbench/status"))).status).toBe(401); });
  it("returns only sanitized bounded status fields", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("secret stack /Users/alice/key", { status: 503 })));
    const response = await GET(new NextRequest("https://oasis.test/api/workbench/status", { headers: { cookie: "oasis_workbench=x" } }));
    const body = await response.json();
    expect(body.status).toBe("down");
    expect(Object.keys(body.services)).toEqual(["backend", "code", "lab", "science_bridge", "workspace", "provider", "storage", "compute"]);
    for (const service of Object.values(body.services) as Record<string, string>[]) expect(Object.keys(service)).toEqual(["status", "reason_code", "next_action"]);
    expect(JSON.stringify(body)).not.toContain("secret stack");
    expect(response.headers.get("cache-control")).toBe("no-store");
  });
  it("reports a fully configured healthy stack", async () => {
    process.env.WORKBENCH_DATABASE_URL = "configured"; process.env.LUMEN_PROVIDER_CONFIGURED = "true"; process.env.STORAGE_DRIVER = "local"; process.env.COMPUTE_RUNNER = "docker";
    vi.stubGlobal("fetch", vi.fn(async () => new Response("{}", { status: 200 })));
    const body = await (await GET(new NextRequest("https://oasis.test/api/workbench/status", { headers: { cookie: "oasis_workbench=x" } }))).json();
    expect(body.status).toBe("ok");
  });
});
