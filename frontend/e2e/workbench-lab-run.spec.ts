import { expect, test } from "@playwright/test";
import { WorkbenchPage } from "./pages/workbench-page";
import { createHash } from "node:crypto";

test("Lab creates a real report, provenance, artifact, and evidence bundle", async ({ page }) => {
  const workbench = new WorkbenchPage(page); await workbench.register("lab"); await workbench.goto("lab");
  const created = await page.request.post("/api/lumen/lab/api/lab/projects", { data: { title: "Controlled Evidence Project", template: "" } }); expect(created.ok()).toBeTruthy();
  const project = await created.json() as { slug: string }; const run = await workbench.runLab(project.slug, "E2E_LAB_REPORT generate the evidence-backed report");
  expect(run.sse, run.sse).toContain('"kind":"tool_result"');
  const detail = await page.request.get(`/api/lumen/lab/api/lab/runs/${run.runID}`); expect((await detail.json()).run.status).toBe("succeeded");
  const artifactsResponse = await page.request.get(`/api/lumen/lab/api/lab/runs/${run.runID}/artifacts`); const artifacts = (await artifactsResponse.json()).artifacts as Array<{ id: string; run_id: string; path: string; sha256: string; provenance: { event_id: string; tool: string } }>;
  const report = artifacts.find(item => item.path === "reports/result.md"); expect(report, JSON.stringify({ artifacts, sse: run.sse })).toBeTruthy(); expect(report).toMatchObject({ run_id: run.runID, provenance: { tool: "write_file" } }); expect(report!.provenance.event_id).toMatch(new RegExp(`^${run.runID}:\\d+$`));
  const download = await page.request.get(`/api/lumen/lab/api/lab/runs/${run.runID}/artifacts/${report!.id}/download`); const bytes = await download.body(); expect(bytes.toString()).toBe("# Controlled Lab Report\n\nEvidence-backed conclusion.\n"); expect(createHash("sha256").update(bytes).digest("hex")).toBe(report!.sha256);
  const evidence = await page.request.get(`/api/lumen/lab/api/lab/runs/${run.runID}/evidence`); expect(evidence.ok()).toBeTruthy(); expect(evidence.headers()["content-type"]).toContain("application/zip"); expect((await evidence.body()).length).toBeGreaterThan(100);
});
