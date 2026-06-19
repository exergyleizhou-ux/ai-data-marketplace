import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { LocaleProvider } from "@/lib/i18n";
import type { User } from "@/lib/api";

// Mutable auth holder (matches the Protected.test.tsx pattern) so each test can
// swap between anonymous and authenticated without re-mocking.
let authState: { user: User | null; loading: boolean; logout: () => void };
let pathname = "/";
vi.mock("@/lib/auth", () => ({ useAuth: () => authState }));
vi.mock("next/navigation", () => ({ usePathname: () => pathname, useRouter: () => ({ push: vi.fn() }) }));
vi.mock("@/lib/api", () => ({ api: { countUnreadNotifications: vi.fn().mockResolvedValue({ unread: 0 }) } }));

import { Nav } from "./Nav";

function renderNav() {
  localStorage.setItem("vo_lang", "zh");
  return render(
    <LocaleProvider>
      <Nav />
    </LocaleProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  pathname = "/";
});

describe("Nav", () => {
  it("shows sign-in / sign-up when anonymous", () => {
    authState = { user: null, loading: false, logout: vi.fn() };
    renderNav();
    expect(screen.getByRole("link", { name: "登录" })).toHaveAttribute("href", "/login");
    expect(screen.getByRole("link", { name: "注册" })).toHaveAttribute("href", "/register");
    expect(screen.queryByRole("button", { name: "退出" })).toBeNull();
  });

  it("marks the active link with aria-current=page", () => {
    authState = {
      user: { id: "u1", account: "seller@vo.test", account_type: "email", role: "seller", kyc_status: "verified", status: "active" },
      loading: false,
      logout: vi.fn(),
    };
    pathname = "/datasets";
    renderNav();
    // The active nav link must expose aria-current for screen-reader users.
    expect(screen.getByRole("link", { name: "数据市场" })).toHaveAttribute("aria-current", "page");
    // A non-active link must NOT claim to be the current page.
    expect(screen.getByRole("link", { name: "我的订单" })).not.toHaveAttribute("aria-current");
  });

  it("shows the account + sign-out when authenticated", () => {
    authState = {
      user: { id: "u1", account: "seller@vo.test", account_type: "email", role: "seller", kyc_status: "verified", status: "active" },
      loading: false,
      logout: vi.fn(),
    };
    renderNav();
    expect(screen.getByText("seller@vo.test")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "退出" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "登录" })).toBeNull();
  });
});
