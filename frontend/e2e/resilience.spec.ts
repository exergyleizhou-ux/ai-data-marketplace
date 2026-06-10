import { test, expect } from "@playwright/test";

// Verifies the App Router not-found boundary renders for unmatched routes, and
// that the security headers from next.config are actually served.
test("unknown routes render the not-found boundary", async ({ page }) => {
  const res = await page.goto("/definitely-not-a-real-route-" + Date.now());
  expect(res?.status()).toBe(404);
  // Heading role avoids matching the description text, which also contains
  // the substring "页面不存在".
  await expect(page.getByRole("heading", { name: /页面不存在|Page not found/ })).toBeVisible();
  await expect(page.getByRole("link", { name: /返回首页|Go home/ })).toBeVisible();
});

test("security headers are served on app responses", async ({ page }) => {
  const res = await page.goto("/");
  const headers = res!.headers();
  expect(headers["x-frame-options"]).toBe("DENY");
  expect(headers["x-content-type-options"]).toBe("nosniff");
  expect(headers["strict-transport-security"]).toContain("max-age=");
  expect(headers["referrer-policy"]).toBe("strict-origin-when-cross-origin");
  expect(headers["permissions-policy"]).toContain("camera=()");
});
