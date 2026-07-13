import { expect, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { INFO_PATH, type HarnessInfo } from "../global-setup";

export class WorkbenchPage {
  constructor(readonly page: Page) {}
  async register(prefix = "workbench") {
    const account = `${prefix}-${randomUUID().slice(0, 8)}@e2e.local`;
    const actor = randomUUID().replaceAll("-", "").slice(0, 16);
    await this.page.context().setExtraHTTPHeaders({ "X-Forwarded-For": `2001:db8:${actor.slice(0, 4)}:${actor.slice(4, 8)}:${actor.slice(8, 12)}:${actor.slice(12)}::1` });
    await this.page.goto("/register");
    await this.page.getByRole("textbox", { name: "账号" }).fill(account);
    await this.page.getByRole("textbox", { name: "密码" }).fill("password123");
    await this.page.getByRole("checkbox").check();
    await this.page.getByRole("button", { name: "注册" }).click();
    await this.page.waitForURL("**/account");
    return account;
  }
  async goto(tab: "coding" | "lab" = "lab") {
    await this.page.goto(`/workspace?tab=${tab}`);
    await expect(this.page.getByRole("tab", { name: tab === "coding" ? "编程智能体" : "实验室" })).toHaveAttribute("aria-selected", "true");
    await expect(this.page.locator("iframe")).toBeVisible();
  }
  async sessionCookie() {
    return (await this.page.context().cookies()).find(cookie => cookie.name === "oasis_workbench");
  }
  async owner() {
    const cookie = await this.sessionCookie(); if (!cookie) throw new Error("workbench cookie missing");
    const claims = JSON.parse(Buffer.from(cookie.value.split(".")[1]!, "base64url").toString()) as { uid: string; workspace_id: string };
    const harness = JSON.parse(readFileSync(INFO_PATH, "utf8")) as HarnessInfo;
    return { ...claims, wid: claims.workspace_id, root: join(harness.tempDir, "workspaces", claims.uid, claims.workspace_id) };
  }
  async startCode(prompt: string, mode: "bypass" | "default" = "bypass") {
    const key = `e2e_${randomUUID().replaceAll("-", "")}`;
    await this.page.evaluate(({ prompt, mode, key }) => {
      const state = { runID: "", done: false, text: "" }; (window as unknown as Record<string, unknown>)[key] = state;
      void fetch("/api/lumen/code/v1/chat", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ prompt, mode }) }).then(async response => {
        const reader = response.body?.getReader(); const decoder = new TextDecoder(); if (!reader) throw new Error("missing Code stream");
        for (;;) { const chunk = await reader.read(); if (chunk.done) break; state.text += decoder.decode(chunk.value, { stream: true }); state.runID ||= state.text.match(/"run_id":"([^"]+)"/)?.[1] ?? ""; }
        state.done = true;
      });
    }, { prompt, mode, key });
    await expect.poll(() => this.page.evaluate(key => ((window as unknown as Record<string, { runID?: string }>)[key]?.runID ?? ""), key)).toMatch(/^run_/);
    const runID = await this.page.evaluate(key => (window as unknown as Record<string, { runID: string }>)[key].runID, key);
    await this.requestCodeRefresh(runID);
    return { runID, key };
  }
  async requestCodeRefresh(runID: string) {
    const handle = await this.page.locator("iframe").elementHandle(); if (!handle || !await handle.contentFrame()) throw new Error("runtime iframe missing");
    const frame = await handle.contentFrame();
    try { await expect.poll(() => frame!.evaluate(() => typeof (window as unknown as { CodeUI?: unknown }).CodeUI)).toBe("object"); }
    catch (error) { const diagnostic = await frame!.evaluate(() => ({ href: location.href, html: document.documentElement.outerHTML.slice(0, 1000), scripts: [...document.scripts].map(script => script.src) })); throw new Error(`${String(error)} ${JSON.stringify(diagnostic)}`); }
    await handle.evaluate((element, runID) => (element as HTMLIFrameElement).contentWindow?.postMessage({ kind: "lumen.workbench.refresh", version: 1, run_id: runID }, window.origin), runID);
    await expect(this.page.getByRole("button", { name: "运行详情", exact: true })).not.toContainText(/空闲|idle/);
  }
  async runCode(prompt: string) {
    const response = await this.page.request.post("/api/lumen/code/v1/chat", { data: { prompt, mode: "bypass" } });
    const text = await response.text(); expect(response.ok(), `Code HTTP ${response.status()}: ${text}`).toBeTruthy();
    const ids = [...text.matchAll(/"run_id":"([^"]+)"/g)].map(match => match[1]);
    const runID = ids.at(-1); if (!runID) throw new Error(`Code SSE missing run id: ${text}`);
    return { runID, sse: text };
  }
  async runLab(projectID: string, prompt: string) {
    const response = await this.page.request.post("/api/lumen/lab/api/lab/chat", { data: { project_id: projectID, prompt, mode: "bypass" } });
    expect(response.ok()).toBeTruthy(); const text = await response.text();
    const ids = [...text.matchAll(/"run_id":"([^"]+)"/g)].map(match => match[1]);
    const runID = ids.at(-1); if (!runID) throw new Error(`Lab SSE missing run id: ${text}`);
    return { runID, sse: text };
  }
}
