import { test, expect } from "@playwright/test";
import { randomUUID } from "node:crypto";
import { apiRegister, seedVerifiedKyc, seedPublishedDataset } from "./helpers";

// Full-stack purchase journey through the browser. Setup (seller, buyer, a
// published dataset) is seeded against the REAL backend the same way the Go E2E
// suite does — registration via the API, dataset published via cmd/devsql —
// then the BUYER drives browse → buy → pay entirely through the real UI.
//
// The download/delivery step needs a real staged file (the Go delivery module
// tests cover that); a SQL-seeded dataset has no object, so we assert the order
// reaches the paid/escrow state, which is the cross-module contract the UI owns.
test("buyer browses a seeded dataset and completes a sandbox purchase", async ({ page }) => {
  const seller = await apiRegister("e2e-seller");
  const buyer = await apiRegister("e2e-buyer");
  seedVerifiedKyc(seller.id);
  seedVerifiedKyc(buyer.id);

  const datasetId = randomUUID();
  const title = `E2E 浏览器数据集 ${datasetId.slice(0, 6)}`;
  seedPublishedDataset({ id: datasetId, sellerId: seller.id, title });

  // Log the buyer in by injecting their real tokens before the app boots.
  await page.addInitScript(
    ([a, r]) => {
      localStorage.setItem("adm_access", a);
      localStorage.setItem("adm_refresh", r);
    },
    [buyer.access, buyer.refresh],
  );

  // The seeded, published dataset is visible in the real catalog.
  await page.goto("/datasets");
  const card = page.getByText(title);
  await expect(card).toBeVisible();
  await card.click();

  // Detail page → place the order.
  await page.waitForURL("**/datasets/**");
  await page.getByRole("button", { name: "立即购买" }).click();

  // Order page → pay through the sandbox (mock) channel.
  await page.waitForURL("**/orders/**");
  await page.getByRole("button", { name: "去支付" }).click();
  await page.getByRole("button", { name: /模拟支付成功/ }).click();

  // Escrow confirmation + the order has advanced to a paid state (the
  // "sign license & get download link" action only appears once paid).
  await expect(page.getByText(/支付成功/)).toBeVisible();
  await expect(page.getByRole("button", { name: /签署许可/ })).toBeVisible();
});
