import { defineConfig, devices } from "@playwright/test";

// Full-stack E2E: globalSetup stands up a real backend + Postgres; webServer
// builds and serves the real Next app (its default API base already points at
// the backend on :8080). Tests then drive the browser against the live stack.
const PORT = 3100;

export default defineConfig({
  testDir: "./e2e",
  // Shared backend state across specs → run serially.
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  timeout: 45_000,
  expect: { timeout: 10_000 },
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : [["list"]],
  globalSetup: "./e2e/global-setup.ts",
  globalTeardown: "./e2e/global-teardown.ts",
  use: {
    baseURL: `http://localhost:${PORT}`,
    // Pin the browser locale to zh-CN so the app's i18n (which flips to English
    // for non-zh navigator languages) stays Chinese — our selectors target the
    // Chinese strings.
    locale: "zh-CN",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: `npm run build && npm run start -- -p ${PORT}`,
    // Build a non-standalone bundle so `next start` serves it directly.
    env: {
      NEXT_OUTPUT_STANDALONE: "0",
      BACKEND_API_BASE_URL: "http://127.0.0.1:8080/api/v1",
      LUMEN_SERVE_URL: "http://127.0.0.1:8787",
      LUMEN_LAB_URL: "http://127.0.0.1:18992",
      LUMEN_PROVIDER_CONFIGURED: "true",
      WORKBENCH_DATABASE_URL: "e2e",
      STORAGE_DRIVER: "local",
      COMPUTE_RUNNER: "controlled",
    },
    url: `http://localhost:${PORT}`,
    reuseExistingServer: !process.env.CI,
    timeout: 240_000,
  },
});
