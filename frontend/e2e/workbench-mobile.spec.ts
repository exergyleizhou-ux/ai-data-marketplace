import { expect, test } from "@playwright/test";
import { WorkbenchPage } from "./pages/workbench-page";

test("390px viewport exposes runtime status and cancellation without overflow", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 }); const workbench = new WorkbenchPage(page); await workbench.register("mobile"); await workbench.goto("coding");
  const active = await workbench.startCode("E2E_CANCEL mobile controlled long command");
  await page.getByRole("button", { name: "运行详情" }).click();
  await expect(page.getByRole("dialog")).toBeVisible(); await page.getByRole("button", { name: /取消 Run/ }).click(); await expect.poll(async () => (await (await page.request.get(`/api/lumen/code/v1/runs/${active.runID}`)).json()).run.status).toBe("canceled"); await workbench.requestCodeRefresh(active.runID); await expect(page.getByRole("button", { name: "运行详情", exact: true })).toContainText("canceled");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});
