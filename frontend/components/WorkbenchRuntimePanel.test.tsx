import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LocaleProvider } from "@/lib/i18n";
import type { LabRuntimeDetail, WorkbenchSnapshot } from "@/lib/workbench-runtime";
import { WorkbenchRuntimePanel } from "./WorkbenchRuntimePanel";

const runningSnapshot: WorkbenchSnapshot = {
  kind: "lumen.workbench.snapshot",
  version: 1,
  surface: "lab",
  project: { id: "project-a", title: "Project A" },
  run: { id: "run_123", last_seq: 7, terminal: false },
  pending_approvals: 2,
};

const runningDetail: LabRuntimeDetail = {
  run: {
    id: "run_123",
    profile: "science",
    title: "Analyze evidence",
    status: "running",
    stop_reason: "",
    error: "",
    version: 3,
  },
  events: [
    { seq: 1, kind: "turn_started", level: "" },
    { seq: 2, kind: "tool_dispatch", level: "" },
  ],
  artifacts: [],
};

function renderPanel(
  overrides: Partial<React.ComponentProps<typeof WorkbenchRuntimePanel>> = {},
) {
  window.localStorage.setItem("vo_lang", "en");
  return render(
    <LocaleProvider>
      <WorkbenchRuntimePanel
        snapshot={null}
        detail={null}
        loading={false}
        error=""
        canceling={false}
        onCancel={vi.fn()}
        {...overrides}
      />
    </LocaleProvider>,
  );
}

afterEach(() => window.localStorage.clear());

describe("WorkbenchRuntimePanel", () => {
  it("shows an honest idle state when no Run identity exists", async () => {
    renderPanel();
    const toggle = await screen.findByRole("button", { name: /runtime details/i });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    await userEvent.click(toggle);
    expect(screen.getByText(/no active run/i)).toBeInTheDocument();
  });

  it("shows running identity, events, approvals, and an enabled cancel action", async () => {
    const onCancel = vi.fn();
    renderPanel({ snapshot: runningSnapshot, detail: runningDetail, onCancel });

    await userEvent.click(await screen.findByRole("button", { name: /runtime details/i }));
    expect(screen.getByText("run_123")).toBeInTheDocument();
    expect(screen.getAllByText(/running/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/2 events/i)).toBeInTheDocument();
    expect(screen.getByText(/2 pending approvals/i)).toBeInTheDocument();
    const cancel = screen.getByRole("button", { name: /cancel run/i });
    expect(cancel).toBeEnabled();
    await userEvent.click(cancel);
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("hides cancel for terminal Runs and exposes encoded same-origin artifact links", async () => {
    const detail: LabRuntimeDetail = {
      ...runningDetail,
      run: { ...runningDetail.run!, status: "succeeded", stop_reason: "finished" },
      artifacts: [
        {
          path: "reports/result 1.md",
          name: "result 1.md",
          size: 42,
          mtime: "2026-07-12T00:00:00Z",
          previewKind: "markdown",
          bucket: "reports",
        },
      ],
    };
    renderPanel({
      snapshot: { ...runningSnapshot, run: { ...runningSnapshot.run!, terminal: true } },
      detail,
    });

    await userEvent.click(await screen.findByRole("button", { name: /runtime details/i }));
    expect(screen.queryByRole("button", { name: /cancel run/i })).toBeNull();
    expect(screen.getByRole("link", { name: "result 1.md" })).toHaveAttribute(
      "href",
      "/api/lumen/lab/api/lab/files/download?project_id=project-a&path=reports%2Fresult+1.md",
    );
  });

  it("renders failed Run reasons and errors as text", async () => {
    const detail: LabRuntimeDetail = {
      ...runningDetail,
      run: {
        ...runningDetail.run!,
        status: "exhausted",
        stop_reason: "max_steps",
        error: "<img src=x onerror=alert(1)>",
      },
    };
    const { container } = renderPanel({
      snapshot: { ...runningSnapshot, run: { ...runningSnapshot.run!, terminal: true } },
      detail,
    });

    await userEvent.click(await screen.findByRole("button", { name: /runtime details/i }));
    expect(screen.getByText("max_steps")).toBeInTheDocument();
    expect(screen.getByText("<img src=x onerror=alert(1)>")).toBeInTheDocument();
    expect(container.querySelector("img")).toBeNull();
  });

  it("requires a fresh prompt before retrying a terminal Run", async () => {
    const onRetry = vi.fn();
    renderPanel({ snapshot: { ...runningSnapshot, version: 2, workspace: { id: "ws_1" }, run: { ...runningSnapshot.run!, status: "failed", terminal: true }, verification: "failed", artifact_count: 0 }, detail: { ...runningDetail, run: { ...runningDetail.run!, status: "failed" } }, onRetry });
    await userEvent.click(await screen.findByRole("button", { name: /runtime details/i }));
    const submit = screen.getByRole("button", { name: /create linked run/i });
    expect(submit).toBeDisabled();
    await userEvent.type(screen.getByLabelText(/new retry prompt/i), "Fix the failing verification");
    await userEvent.click(submit);
    expect(onRetry).toHaveBeenCalledWith("Fix the failing verification");
  });

  it("renders approval review cards and submits explicit decisions", async () => {
    const onApprovalDecision = vi.fn();
    renderPanel({ snapshot: { ...runningSnapshot, version: 2, workspace: { id: "ws_1" }, run: { ...runningSnapshot.run!, status: "waiting_approval" }, verification: "not_run", artifact_count: 0 }, detail: { ...runningDetail, approvals: [{ id: "approval_1", run_id: "run_123", step_id: "step_1", tool_call_id: "tool_1", risk_level: "high", effects: { commands: true, charge: false }, estimated_cost_micros: 2500, created_at: "2026-07-13T00:00:00Z", expires_at: "2026-07-13T00:05:00Z", decision: null, execution_state: "pending" }] }, onApprovalDecision });
    await userEvent.click(await screen.findByRole("button", { name: /runtime details/i }));
    expect(screen.getByText(/commands/i)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /approve/i }));
    expect(onApprovalDecision).toHaveBeenCalledWith("approval_1", true);
  });

  it("acts as a keyboard dialog and returns focus on Escape", async () => {
    renderPanel(); const toggle = await screen.findByRole("button", { name: /runtime details/i });
    await userEvent.click(toggle); expect(screen.getByRole("dialog")).toBeInTheDocument(); expect(screen.getByRole("button", { name: /close runtime details/i })).toHaveFocus();
    await userEvent.keyboard("{Escape}"); expect(screen.queryByRole("dialog")).toBeNull(); expect(toggle).toHaveFocus();
  });
});
