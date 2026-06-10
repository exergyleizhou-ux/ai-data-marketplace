import { test, expect } from "@playwright/test";
import { randomUUID } from "node:crypto";

// Real auth journey through the browser: register → authenticated nav → logout
// → login. Hits the live backend, mints a real JWT, persists it, and re-reads
// it after a reload.
test("register, persist session, logout, and log back in", async ({ page }) => {
  const account = `e2e-auth-${randomUUID().slice(0, 8)}@e2e.local`;
  const password = "password123";

  // --- register via the UI ---
  // The Field component wraps <span>label</span> + input in one <label>, which
  // getByLabel doesn't reliably associate — target the controls by role/type
  // instead (the form has a single text input + one password input).
  await page.goto("/register");
  // Target inputs by their accessible names (account="账号", password="密码 …").
  await page.getByRole("textbox", { name: "账号" }).fill(account);
  await page.getByRole("textbox", { name: "密码" }).fill(password);
  await page.getByRole("checkbox").check();
  await page.getByRole("button", { name: "注册" }).click();

  // Lands on the account page, authenticated: the nav shows the sign-out button
  // (unique to the authed nav) and the account identifier.
  await page.waitForURL("**/account");
  await expect(page.getByRole("button", { name: "退出" })).toBeVisible();
  await expect(page.getByText(account).first()).toBeVisible();

  // --- session survives a reload (token persisted in localStorage) ---
  await page.reload();
  await expect(page.getByRole("button", { name: "退出" })).toBeVisible();

  // --- logout returns to the anonymous nav ---
  // exact:true so the nav's "登录" link doesn't also match a "去登录" prompt link.
  await page.getByRole("button", { name: "退出" }).click();
  await expect(page.getByRole("link", { name: "登录", exact: true })).toBeVisible();

  // --- log back in via the UI ---
  await page.goto("/login");
  await page.getByRole("textbox", { name: "账号" }).fill(account);
  await page.getByRole("textbox", { name: "密码" }).fill(password);
  await page.getByRole("button", { name: "登录" }).click();

  await page.waitForURL("**/datasets");
  // The login page sets tokens + navigates but doesn't push into AuthProvider
  // state (unlike register), so the nav only reflects the session after a
  // reload — which also confirms the persisted token yields a valid session.
  await page.reload();
  await expect(page.getByRole("button", { name: "退出" })).toBeVisible();
});
