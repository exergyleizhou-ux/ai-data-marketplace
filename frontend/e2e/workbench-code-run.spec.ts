import { expect, test } from "@playwright/test";
import { WorkbenchPage } from "./pages/workbench-page";
import { devsql } from "./helpers";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

test("Code fails verification, self-fixes, and passes against real fixture bytes", async ({ page }) => {
  const workbench = new WorkbenchPage(page); await workbench.register("code"); await workbench.goto("coding");
  const owner = await workbench.owner(); mkdirSync(owner.root, { recursive: true }); writeFileSync(join(owner.root, "go.mod"), "module e2e.local/fixture\n\ngo 1.23\n");
  devsql(`INSERT INTO workbench_workspaces(id,account_id,slug,display_name,status) VALUES('${owner.wid}','${owner.uid}','personal','Personal','active') ON CONFLICT(id) DO NOTHING`);
  devsql(`DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM users WHERE id='${owner.uid}'::uuid) OR NOT EXISTS(SELECT 1 FROM workbench_workspaces WHERE id='${owner.wid}'::uuid AND account_id='${owner.uid}'::uuid) THEN RAISE EXCEPTION 'owner fixture query-back failed'; END IF; END $$`);
  const failed = await workbench.runCode("E2E_CODE_INVALID implement this Go code change and run tests");
  const failedRun = await page.request.get(`/api/lumen/code/v1/runs/${failed.runID}`); const failedBody = await failedRun.json(); expect(failedBody.run, JSON.stringify({ failedBody, sse: failed.sse })).toBeTruthy(); expect(failedBody.run.status).toBe("failed"); expect(failed.sse).toContain('"ok":false');
  expect(existsSync(join(owner.root, "main.go")), JSON.stringify({ failedBody, sse: failed.sse })).toBeTruthy(); expect(readFileSync(join(owner.root, "main.go"), "utf8")).toContain("func main( {");
  const fixed = await workbench.runCode("E2E_CODE_FIX implement the Go fix and run tests to verify it");
  const fixedRun = await page.request.get(`/api/lumen/code/v1/runs/${fixed.runID}`); expect((await fixedRun.json()).run.status).toBe("succeeded"); expect(fixed.sse).toContain('"kind":"verify_result"'); expect(fixed.sse).toContain('"ok":true');
  expect(readFileSync(join(owner.root, "main.go"), "utf8")).toBe("package main\n\nfunc main() {}\n");
});
