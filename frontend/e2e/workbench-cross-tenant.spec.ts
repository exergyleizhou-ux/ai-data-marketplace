import { expect, test } from "@playwright/test";
import { WorkbenchPage } from "./pages/workbench-page";

test("A-owned real Run, events, project, and artifact are all 404 to B", async ({ browser }) => {
  const a = await browser.newContext({ locale: "zh-CN" }); const b = await browser.newContext({ locale: "zh-CN" });
  try {
    const pa = new WorkbenchPage(await a.newPage()); const pb = new WorkbenchPage(await b.newPage());
    await pa.register("tenant-a"); await pb.register("tenant-b"); await pa.goto(); await pb.goto();
    const ac = await pa.sessionCookie(); const bc = await pb.sessionCookie();
    expect(ac?.value).toBeTruthy(); expect(bc?.value).toBeTruthy(); expect(ac?.value).not.toBe(bc?.value);
    await pa.goto("lab"); const created = await pa.page.request.post("/api/lumen/lab/api/lab/projects", { data: { title: "Tenant A Project", template: "" } }); const project = await created.json() as { slug: string };
    const run = await pa.runLab(project.slug, "E2E_LAB_REPORT tenant isolation artifact"); const list = await (await pa.page.request.get(`/api/lumen/lab/api/lab/runs/${run.runID}/artifacts`)).json(); const artifactID = list.artifacts[0].id as string;
    for (const path of [`/api/lumen/lab/api/lab/projects/${project.slug}`, `/api/lumen/lab/api/lab/runs/${run.runID}`, `/api/lumen/lab/api/lab/runs/${run.runID}/events?after=0`, `/api/lumen/lab/api/lab/runs/${run.runID}/artifacts/${artifactID}/download`]) {
      expect((await pb.page.request.get(path)).status(), path).toBe(404);
    }
  } finally { await a.close(); await b.close(); }
});
