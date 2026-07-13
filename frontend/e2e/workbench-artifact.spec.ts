import { expect, test } from "@playwright/test";
import { WorkbenchPage } from "./pages/workbench-page";

test("artifact endpoints fail closed for unknown and foreign objects", async ({ page }) => {
  const workbench = new WorkbenchPage(page); await workbench.register("artifact"); await workbench.goto();
  const response = await page.request.get("/api/v1/workbench/artifacts/foreign/download?workspace_id=00000000-0000-0000-0000-000000000000");
  expect(response.status()).toBe(404);
});
