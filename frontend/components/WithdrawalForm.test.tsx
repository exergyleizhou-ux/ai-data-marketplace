import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LocaleProvider } from "@/lib/i18n";
import { WithdrawalForm } from "./WithdrawalForm";

const requestWithdrawal = vi.fn();
vi.mock("@/lib/api", async () => {
  // Keep yuan() real so the hint text renders; only stub the network method.
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    api: { ...actual.api, requestWithdrawal: (...a: unknown[]) => requestWithdrawal(...a) },
  };
});

const onCreated = vi.fn();
function renderForm(withdrawableCents = 100_00) {
  localStorage.setItem("vo_lang", "zh");
  return render(
    <LocaleProvider>
      <WithdrawalForm withdrawableCents={withdrawableCents} onCreated={onCreated} />
    </LocaleProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("WithdrawalForm", () => {
  it("submits cents conversion and channel + invokes onCreated", async () => {
    requestWithdrawal.mockResolvedValue({
      id: "w1", seller_id: "s1", amount_cents: 5000, channel: "alipay",
      account_label: "alipay 1234", status: "pending", requested_at: "2026-06-11T00:00:00Z",
    });
    renderForm(50_000_00);
    // Field wraps label+input+hint in one <label>, so the accessible name
    // includes the hint — match by prefix instead of exact.
    await userEvent.type(screen.getByLabelText(/^金额/), "50");
    await userEvent.type(screen.getByLabelText(/^收款账号备注/), "alipay 1234");
    await userEvent.click(screen.getByRole("button", { name: "申请提现" }));

    expect(requestWithdrawal).toHaveBeenCalledWith({
      amount_cents: 5000, channel: "alipay", account_label: "alipay 1234",
    });
    expect(onCreated).toHaveBeenCalledTimes(1);
    expect(await screen.findByText(/已提交/)).toBeInTheDocument();
  });

  it("blocks amount over the withdrawable balance without hitting the API", async () => {
    renderForm(10_00); // ¥10 cap
    await userEvent.type(screen.getByLabelText(/^金额/), "11");
    await userEvent.type(screen.getByLabelText(/^收款账号备注/), "x");
    await userEvent.click(screen.getByRole("button", { name: "申请提现" }));

    expect(await screen.findByText("超过可提现余额")).toBeInTheDocument();
    expect(requestWithdrawal).not.toHaveBeenCalled();
    expect(onCreated).not.toHaveBeenCalled();
  });

  it("surfaces the server error message verbatim", async () => {
    requestWithdrawal.mockRejectedValue(new Error("insufficient settled balance"));
    renderForm(100_00);
    await userEvent.type(screen.getByLabelText(/^金额/), "50");
    await userEvent.type(screen.getByLabelText(/^收款账号备注/), "alipay 1234");
    await userEvent.click(screen.getByRole("button", { name: "申请提现" }));

    expect(await screen.findByText("insufficient settled balance")).toBeInTheDocument();
    expect(onCreated).not.toHaveBeenCalled();
  });
});
