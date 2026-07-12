import { expect, test } from "@playwright/test";
import { WorkbenchPage } from "./pages/workbench-page";

test("authenticated runtime reload recovers the same durable Run and events", async ({ page }) => {
  const workbench = new WorkbenchPage(page); await workbench.register("recovery"); await workbench.goto("coding");
  const completed = await workbench.runCode("E2E_RECOVERY complete this controlled request"); await workbench.requestCodeRefresh(completed.runID);
  await page.getByRole("button", { name: "运行详情" }).click(); const dialog = page.getByRole("dialog");
  const beforeText = await dialog.textContent(); const runID = beforeText?.match(/run_[a-f0-9]+/)?.[0]; expect(runID).toBeTruthy();
  const beforeEvents = await page.request.get(`/api/lumen/code/v1/runs/${runID}/events?after=0`); const before = await beforeEvents.json(); expect(before.events.length).toBeGreaterThan(0);
  await page.getByRole("button", { name: "关闭运行详情" }).click(); await page.reload(); await expect(page.locator("iframe")).toBeVisible(); await workbench.requestCodeRefresh(runID!);
  await expect(page.getByRole("button", { name: "运行详情" })).toContainText("succeeded"); await page.getByRole("button", { name: "运行详情" }).click(); await expect(page.getByRole("dialog")).toContainText(runID!);
  const after = await (await page.request.get(`/api/lumen/code/v1/runs/${runID}/events?after=0`)).json(); expect(after.events).toEqual(before.events);
});

test("cancel action terminates an actual active Code Run", async ({ page }) => {
  const workbench = new WorkbenchPage(page); await workbench.register("cancel"); await workbench.goto("coding");
  await workbench.startCode("E2E_CANCEL run a controlled long command"); await page.getByRole("button", { name: "运行详情" }).click(); const dialog = page.getByRole("dialog");
  const runID = (await dialog.textContent())?.match(/run_[a-f0-9]+/)?.[0]; expect(runID).toBeTruthy(); await page.getByRole("button", { name: /取消 Run/ }).click();
  await expect.poll(async () => (await (await page.request.get(`/api/lumen/code/v1/runs/${runID}`)).json()).run.status).toBe("canceled"); await workbench.requestCodeRefresh(runID!); await expect(page.getByRole("button", { name: "运行详情", exact: true })).toContainText("canceled");
});
