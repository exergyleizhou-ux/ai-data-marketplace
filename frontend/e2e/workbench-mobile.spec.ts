import { expect, test } from "@playwright/test";
import { WorkbenchPage } from "./pages/workbench-page";

test("390px viewport exposes runtime status and cancellation without overflow", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 }); const workbench = new WorkbenchPage(page); await workbench.register("mobile"); await workbench.goto("coding");
  await workbench.postSnapshot("code", { id: "mobile-run", status: "running" });
  await page.getByRole("button", { name: "运行详情" }).click();
  await expect(page.getByRole("dialog")).toBeVisible(); await expect(page.getByRole("button", { name: /取消 Run/ })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});
