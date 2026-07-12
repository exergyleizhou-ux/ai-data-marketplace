import { expect, test } from "@playwright/test";
import { WorkbenchPage } from "./pages/workbench-page";

test("independent A and B contexts receive distinct tenant sessions", async ({ browser }) => {
  const a = await browser.newContext({ locale: "zh-CN" }); const b = await browser.newContext({ locale: "zh-CN" });
  try {
    const pa = new WorkbenchPage(await a.newPage()); const pb = new WorkbenchPage(await b.newPage());
    await pa.register("tenant-a"); await pb.register("tenant-b"); await pa.goto(); await pb.goto();
    const ac = await pa.sessionCookie(); const bc = await pb.sessionCookie();
    expect(ac?.value).toBeTruthy(); expect(bc?.value).toBeTruthy(); expect(ac?.value).not.toBe(bc?.value);
    const foreign = await pa.page.request.get("/api/v1/workbench/runs/not-owned?workspace_id=00000000-0000-0000-0000-000000000000");
    expect(foreign.status()).toBe(404);
  } finally { await a.close(); await b.close(); }
});
