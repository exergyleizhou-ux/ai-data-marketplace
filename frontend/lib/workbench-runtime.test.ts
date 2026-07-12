// @vitest-environment node

import { describe, expect, it, vi } from "vitest";
import {
  WorkbenchRuntimeError,
  cancelLabRun,
  decideRuntimeApproval,
  isTerminalRunStatus,
  loadLabRuntime,
  parseTrustedWorkbenchMessage,
  parseWorkbenchSnapshot,
} from "./workbench-runtime";

const validMessage = {
  kind: "lumen.workbench.snapshot",
  version: 1,
  surface: "lab",
  project: { id: "project-a", title: "Project A" },
  run: { id: "run_1", last_seq: 7, terminal: false },
  pending_approvals: 2,
};

describe("parseWorkbenchSnapshot", () => {
  it("accepts the versioned minimal Lab bridge message", () => {
    expect(parseWorkbenchSnapshot(validMessage)).toEqual(validMessage);
  });

  it("rejects unknown versions, malformed ids, and injected fields", () => {
    expect(parseWorkbenchSnapshot({ ...validMessage, version: 2 })).toBeNull();
    expect(
      parseWorkbenchSnapshot({ ...validMessage, run: { id: "../secret", last_seq: 0, terminal: false } }),
    ).toBeNull();
    expect(
      parseWorkbenchSnapshot({ ...validMessage, project: { id: "project/a", title: "Project A" } }),
    ).toBeNull();
    expect(parseWorkbenchSnapshot({ ...validMessage, prompt: "do not forward me" })).toBeNull();
  });

  it("accepts messages only from the expected same-origin iframe source", () => {
    const source = {} as MessageEventSource;
    const otherSource = {} as MessageEventSource;
    const event = { origin: "https://oasis.test", source, data: validMessage };
    expect(parseTrustedWorkbenchMessage(event, "https://oasis.test", source)).toEqual(validMessage);
    expect(parseTrustedWorkbenchMessage(event, "https://evil.test", source)).toBeNull();
    expect(parseTrustedWorkbenchMessage(event, "https://oasis.test", otherSource)).toBeNull();
  });
});

describe("Workbench v2 union", () => {
  const v2 = { kind: "lumen.workbench.snapshot", version: 2, surface: "code", workspace: { id: "ws_1" }, project: null, run: { id: "run_2", last_seq: 8, status: "verifying", terminal: false }, pending_approvals: 1, verification: "running", artifact_count: 3 };
  it("accepts the exact Code and Lab contracts", () => {
    expect(parseWorkbenchSnapshot(v2)).toEqual(v2);
    expect(parseWorkbenchSnapshot({ ...v2, surface: "lab", project: { id: "p_1", title: "Study" } })?.surface).toBe("lab");
  });
  it("rejects unknown, sensitive, and invalid union fields", () => {
    expect(parseWorkbenchSnapshot({ ...v2, prompt: "secret" })).toBeNull();
    expect(parseWorkbenchSnapshot({ ...v2, verification: "success" })).toBeNull();
    expect(parseWorkbenchSnapshot({ ...v2, run: { ...v2.run, status: "done" } })).toBeNull();
    expect(parseWorkbenchSnapshot({ ...v2, workspace: { id: "../escape" } })).toBeNull();
  });
  it("posts approval decisions without exposing review content", async () => {
    const fetcher = vi.fn(async () => Response.json({ ok: true }));
    const snapshot = parseWorkbenchSnapshot(v2)!;
    await decideRuntimeApproval(snapshot, "approval_1", false, fetcher);
    expect(fetcher).toHaveBeenCalledWith("/api/lumen/code/v1/approve", expect.objectContaining({ method: "POST", body: JSON.stringify({ id: "approval_1", allow: false }) }));
  });
});

describe("Lab runtime client", () => {
  it("loads the authoritative Run, events, and artifacts with encoded identifiers", async () => {
    const fetcher = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url === "/api/lab/runs/run_1") {
        return Response.json({
          run: {
            id: "run_1",
            profile: "science",
            title: "Analyze evidence",
            status: "running",
            stop_reason: "",
            error: "",
            version: 3,
          },
        });
      }
      if (url === "/api/lab/runs/run_1/events?after=0") {
        return Response.json({
          events: [
            { seq: 1, kind: "turn_started", text: "secret prompt is dropped" },
            { seq: 2, kind: "tool_dispatch", tool: { args: "secret args are dropped" } },
          ],
        });
      }
      if (url === "/api/lab/artifacts?project_id=project-a") {
        return Response.json({
          artifacts: [
            {
              path: "reports/result.md",
              name: "result.md",
              size: 42,
              mtime: "2026-07-12T00:00:00Z",
              previewKind: "markdown",
              bucket: "reports",
            },
          ],
        });
      }
      throw new Error(`unexpected URL ${url}`);
    });

    const detail = await loadLabRuntime(validMessage, fetcher);

    expect(fetcher).toHaveBeenCalledTimes(3);
    expect(detail.run?.status).toBe("running");
    expect(detail.events).toEqual([
      { seq: 1, kind: "turn_started", level: "" },
      { seq: 2, kind: "tool_dispatch", level: "" },
    ]);
    expect(detail.artifacts[0]?.path).toBe("reports/result.md");
  });

  it("posts cancellation and never exposes an upstream response body", async () => {
    const okFetcher = vi.fn(async () => Response.json({ ok: true, run_id: "run_1" }, { status: 202 }));
    await cancelLabRun("run_1", okFetcher);
    expect(okFetcher).toHaveBeenCalledWith(
      "/api/lab/runs/run_1/cancel",
      expect.objectContaining({ method: "POST" }),
    );

    const failingFetcher = vi.fn(async () => new Response("upstream-token=secret", { status: 502 }));
    await expect(cancelLabRun("run_1", failingFetcher)).rejects.toMatchObject({ status: 502 });
    await expect(cancelLabRun("run_1", failingFetcher)).rejects.not.toThrow(/secret/);
  });

  it("recognizes every immutable terminal status", () => {
    expect(["succeeded", "failed", "canceled", "timed_out", "exhausted"].every(isTerminalRunStatus)).toBe(true);
    expect(isTerminalRunStatus("running")).toBe(false);
    expect(WorkbenchRuntimeError).toBeTypeOf("function");
  });
});
