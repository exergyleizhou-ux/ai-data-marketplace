import { expect, test } from "@playwright/test";
import { WorkbenchPage } from "./pages/workbench-page";

test("anonymous users never load a runtime iframe", async ({ page }) => {
  await page.goto("/workspace?tab=lab");
  await expect(page.locator("iframe")).toHaveCount(0);
  await expect(page.getByText("请登录或重试")).toBeVisible();
});

test("login establishes an opaque HttpOnly Workbench session", async ({ page }) => {
  const workbench = new WorkbenchPage(page); await workbench.register("auth"); await workbench.goto();
  const cookie = await workbench.sessionCookie();
  expect(cookie).toMatchObject({ httpOnly: true, sameSite: "Strict", path: "/" });
  expect(await page.evaluate(() => document.cookie)).not.toContain("oasis_workbench");
});
