import { expect, test } from "@playwright/test";
import { WorkbenchPage } from "./pages/workbench-page";

test("Lab hosted surface exposes report/provenance/artifact contract", async ({ page }) => {
  const workbench = new WorkbenchPage(page); await workbench.register("lab"); await workbench.goto("lab");
  const health = await page.request.get("/api/lumen/lab/api/lab/health"); expect(health.ok()).toBeTruthy();
  await workbench.postSnapshot("lab", { id: "lab-report-provenance-evidence", status: "succeeded", terminal: true });
  await expect(page.getByRole("button", { name: "运行详情" })).toContainText("succeeded");
});
