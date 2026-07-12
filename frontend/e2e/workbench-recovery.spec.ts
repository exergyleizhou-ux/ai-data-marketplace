import { expect, test } from "@playwright/test";
import { WorkbenchPage } from "./pages/workbench-page";

test("authenticated runtime survives reload and can reconnect", async ({ page }) => {
  const workbench = new WorkbenchPage(page); await workbench.register("recovery"); await workbench.goto("coding");
  const before = (await workbench.sessionCookie())?.value; await page.reload();
  await expect(page.locator("iframe")).toBeVisible(); expect((await workbench.sessionCookie())?.value).toBeTruthy(); expect(before).toBeTruthy();
});

test("cancel action is available for an active run", async ({ page }) => {
  const workbench = new WorkbenchPage(page); await workbench.register("cancel"); await workbench.goto("coding");
  await workbench.postSnapshot("code", { id: "cancelable", status: "running" });
  await page.getByRole("button", { name: "运行详情" }).click(); await expect(page.getByRole("button", { name: /取消 Run/ })).toBeEnabled();
});
