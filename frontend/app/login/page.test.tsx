import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LocaleProvider } from "@/lib/i18n";
import { AuthProvider } from "@/lib/auth";
import LoginPage from "./page";

// Router + API are the page's two side-effect boundaries; stub both.
const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));

const login = vi.fn();
const verify2FA = vi.fn();
const tokenSet = vi.fn();
const me = vi.fn();
vi.mock("@/lib/api", () => ({
  api: {
    login: (...a: unknown[]) => login(...a),
    verify2FA: (...a: unknown[]) => verify2FA(...a),
    me: (...a: unknown[]) => me(...a),
  },
  // AuthProvider checks tokenStore.access on mount — undefined here makes its
  // initial refresh() a no-op so the test never touches api.me().
  tokenStore: { set: (...a: unknown[]) => tokenSet(...a), access: undefined },
}));

const tokens = { access_token: "AT", refresh_token: "RT", expires_in: 900 };
const user = { id: "u1", account: "a@b.c", account_type: "email", role: "buyer", kyc_status: "none", status: "active" };

function renderLogin() {
  localStorage.setItem("vo_lang", "zh");
  return render(
    <LocaleProvider>
      <AuthProvider>
        <LoginPage />
      </AuthProvider>
    </LocaleProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("LoginPage", () => {
  it("logs in, stores tokens, and routes to the catalog on success", async () => {
    login.mockResolvedValue({ tokens, user });
    renderLogin();
    await userEvent.type(screen.getByLabelText("账号"), "a@b.c");
    await userEvent.type(screen.getByLabelText("密码"), "password123");
    await userEvent.click(screen.getByRole("button", { name: "登录" }));

    expect(login).toHaveBeenCalledWith("a@b.c", "password123");
    expect(tokenSet).toHaveBeenCalledWith(tokens);
    expect(push).toHaveBeenCalledWith("/datasets");
  });

  it("switches to the 2FA form when the backend demands a second factor", async () => {
    login.mockResolvedValue({ need_2fa: true, challenge_token: "CHAL" });
    verify2FA.mockResolvedValue({ tokens, user });
    renderLogin();
    await userEvent.type(screen.getByLabelText("账号"), "a@b.c");
    await userEvent.type(screen.getByLabelText("密码"), "password123");
    await userEvent.click(screen.getByRole("button", { name: "登录" }));

    // 2FA challenge UI appears; no routing yet.
    expect(await screen.findByText("两步验证")).toBeInTheDocument();
    expect(push).not.toHaveBeenCalled();

    await userEvent.type(screen.getByRole("textbox"), "123456");
    await userEvent.click(screen.getByRole("button", { name: "验证" }));
    expect(verify2FA).toHaveBeenCalledWith("CHAL", "123456");
    expect(tokenSet).toHaveBeenCalledWith(tokens);
    expect(push).toHaveBeenCalledWith("/datasets");
  });

  it("surfaces the error message when login is rejected", async () => {
    login.mockRejectedValue(new Error("账号或密码错误"));
    renderLogin();
    await userEvent.type(screen.getByLabelText("账号"), "a@b.c");
    await userEvent.type(screen.getByLabelText("密码"), "wrong");
    await userEvent.click(screen.getByRole("button", { name: "登录" }));

    expect(await screen.findByText("账号或密码错误")).toBeInTheDocument();
    expect(push).not.toHaveBeenCalled();
  });
});
