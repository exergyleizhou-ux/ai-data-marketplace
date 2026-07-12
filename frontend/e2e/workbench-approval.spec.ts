import { expect, test } from "@playwright/test";
import { WorkbenchPage } from "./pages/workbench-page";

test("malicious source messages cannot inject approval or execution state", async ({ page }) => {
  const workbench = new WorkbenchPage(page); await workbench.register("approval"); await workbench.goto("lab");
  await page.evaluate(() => window.postMessage({ source: "lumen-workbench", version: 2, surface: "lab", workspace: { id: "evil" }, run: { id: "evil", status: "waiting_approval" }, pending_approvals: 1 }, window.origin));
  await expect(page.getByRole("button", { name: "运行详情" })).toContainText(/Lab: (空闲|idle)/);
  await expect(page.getByText("evil")).toHaveCount(0);
});
