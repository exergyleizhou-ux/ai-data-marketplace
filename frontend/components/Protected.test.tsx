import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import type { User } from "@/lib/api";
import { Protected } from "./Protected";

// Drive Protected through its auth-state branches by stubbing the useAuth
// boundary it consumes. A mutable holder lets each test set the state.
let authState: { user: User | null; loading: boolean };
vi.mock("@/lib/auth", () => ({
  useAuth: () => authState,
}));

const buyer: User = {
  id: "u1", account: "a@b.c", account_type: "email", role: "buyer", kyc_status: "none", status: "active",
};

beforeEach(() => {
  authState = { user: null, loading: false };
});

describe("Protected", () => {
  it("shows a spinner while auth is loading and hides children", () => {
    authState = { user: null, loading: true };
    render(<Protected><div>secret</div></Protected>);
    expect(screen.queryByText("secret")).not.toBeInTheDocument();
    expect(screen.getByText("加载中…")).toBeInTheDocument();
  });

  it("prompts anonymous users to log in", () => {
    authState = { user: null, loading: false };
    render(<Protected><div>secret</div></Protected>);
    expect(screen.getByText("请先登录")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "去登录" })).toHaveAttribute("href", "/login");
    expect(screen.queryByText("secret")).not.toBeInTheDocument();
  });

  it("blocks non-ops users from an ops-gated page", () => {
    authState = { user: buyer, loading: false };
    render(<Protected requireOps><div>ops-panel</div></Protected>);
    expect(screen.getByText("需要运营权限")).toBeInTheDocument();
    expect(screen.queryByText("ops-panel")).not.toBeInTheDocument();
  });

  it("admits ops users to an ops-gated page", () => {
    authState = { user: { ...buyer, role: "ops" }, loading: false };
    render(<Protected requireOps><div>ops-panel</div></Protected>);
    expect(screen.getByText("ops-panel")).toBeInTheDocument();
  });

  it("requires verified KYC when requireKYC is set", () => {
    authState = { user: { ...buyer, kyc_status: "none" }, loading: false };
    render(<Protected requireKYC><div>trade</div></Protected>);
    expect(screen.getByText("需要完成实名认证")).toBeInTheDocument();
    expect(screen.queryByText("trade")).not.toBeInTheDocument();
  });

  it("renders children for a verified, authorized user", () => {
    authState = { user: { ...buyer, kyc_status: "verified" }, loading: false };
    render(<Protected requireKYC><div>trade</div></Protected>);
    expect(screen.getByText("trade")).toBeInTheDocument();
  });
});
