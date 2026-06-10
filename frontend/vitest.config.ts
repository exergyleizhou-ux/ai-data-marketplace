import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";

// Component tests run in jsdom by default. Pure-logic tests that exercise fetch
// (lib/api) opt into the node environment with a `// @vitest-environment node`
// comment at the top of the file.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      // Mirror tsconfig's "@/*": ["./*"] so imports resolve identically.
      "@": fileURLToPath(new URL("./", import.meta.url)),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./vitest.setup.ts"],
    include: ["{app,components,lib}/**/*.{test,spec}.{ts,tsx}"],
    // e2e/ is Playwright's territory — keep it out of the Vitest run.
    exclude: ["node_modules", ".next", "e2e/**"],
    coverage: {
      provider: "v8",
      include: ["lib/**", "components/**", "app/**"],
      exclude: ["**/*.test.*", "**/*.spec.*", "app/**/layout.tsx"],
    },
  },
});
