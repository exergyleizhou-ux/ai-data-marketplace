import { expect, test } from "@playwright/test";
import { WorkbenchPage } from "./pages/workbench-page";
import { existsSync } from "node:fs";
import { join } from "node:path";

test("real approval rejection has no side effect and malicious messages cannot inject state", async ({ page }) => {
  const workbench = new WorkbenchPage(page); await workbench.register("approval"); await workbench.goto("lab");
  await page.evaluate(() => window.postMessage({ source: "lumen-workbench", version: 2, surface: "lab", workspace: { id: "evil" }, run: { id: "evil", status: "waiting_approval" }, pending_approvals: 1 }, window.origin));
  await expect(page.getByRole("button", { name: "运行详情" })).toContainText(/Lab: (空闲|idle)/);
  await expect(page.getByText("evil")).toHaveCount(0);
  await workbench.goto("coding"); const owner = await workbench.owner(); await workbench.startCode("E2E_APPROVAL create the controlled marker", "default");
  await page.getByRole("button", { name: "运行详情" }).click(); const dialog = page.getByRole("dialog"); await expect(dialog).toContainText(/1 (个待审批|pending approval)/); const runID = (await dialog.textContent())?.match(/run_[a-f0-9]+/)?.[0]; expect(runID).toBeTruthy(); await page.getByRole("button", { name: /拒绝|Reject/ }).click();
  await expect.poll(async () => (await (await page.request.get(`/api/lumen/code/v1/runs/${runID}/approvals`)).json()).approvals[0]?.decision).toBe("rejected"); expect(existsSync(join(owner.root, "rejected-side-effect.txt"))).toBe(false);
});
