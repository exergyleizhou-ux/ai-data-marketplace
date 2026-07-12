import { expect, test } from "@playwright/test";
import { WorkbenchPage } from "./pages/workbench-page";

test("Code hosted surface is real and reports controlled run recovery state", async ({ page }) => {
  const workbench = new WorkbenchPage(page); await workbench.register("code"); await workbench.goto("coding");
  const health = await page.request.get("/api/lumen/code/"); expect(health.ok()).toBeTruthy();
  await workbench.postSnapshot("code", { id: "fixture-fail-fix-pass", status: "succeeded", terminal: true });
  await expect(page.getByRole("button", { name: "运行详情" })).toContainText("succeeded");
});
