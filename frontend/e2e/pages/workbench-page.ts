import { expect, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";

export class WorkbenchPage {
  constructor(readonly page: Page) {}
  async register(prefix = "workbench") {
    const account = `${prefix}-${randomUUID().slice(0, 8)}@e2e.local`;
    const actor = 10 + Math.floor(Math.random() * 220);
    await this.page.context().setExtraHTTPHeaders({ "X-Forwarded-For": `203.0.113.${actor}` });
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
  async postSnapshot(surface: "code" | "lab", run: { id: string; status: string; terminal?: boolean } | null = null) {
    const handle = await this.page.locator("iframe").elementHandle();
    const frame = await handle?.contentFrame();
    if (!frame) throw new Error("runtime iframe missing");
    await frame.evaluate(({ surface, run }) => window.parent.postMessage({ kind: "lumen.workbench.snapshot", version: 2, surface, workspace: { id: "personal" }, project: null, run: run ? { ...run, last_seq: 0, terminal: run.terminal ?? false } : null, pending_approvals: 0, verification: run?.status === "succeeded" ? "passed" : "running", artifact_count: 0 }, "*"), { surface, run });
  }
}
