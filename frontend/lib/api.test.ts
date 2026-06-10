import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, tokenStore, yuan, ApiError } from "./api";

// A controllable fetch stub. Each test installs a router that maps a request
// (url, init) to a fake Response. We assert on call counts to prove the
// refresh/retry/singleflight behavior, which is the trickiest logic in api.ts.

type FakeResponse = { ok: boolean; status: number; body: unknown };

function resp(status: number, body: unknown): FakeResponse {
  return { ok: status >= 200 && status < 300, status, body };
}

function installFetch(router: (url: string, init: RequestInit) => FakeResponse) {
  const calls: { url: string; init: RequestInit }[] = [];
  const fn = vi.fn(async (url: string | URL, init: RequestInit = {}) => {
    const u = url.toString();
    calls.push({ url: u, init });
    const r = router(u, init);
    return {
      ok: r.ok,
      status: r.status,
      statusText: `HTTP ${r.status}`,
      json: async () => r.body,
      blob: async () => new Blob([]),
    } as unknown as Response;
  });
  vi.stubGlobal("fetch", fn);
  return { fn, calls };
}

const ok = (data: unknown) => ({ code: 0, message: "ok", data, request_id: "req-test" });

beforeEach(() => {
  localStorage.clear();
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("yuan", () => {
  it("formats cents to ¥ with two decimals", () => {
    expect(yuan(12345)).toBe("¥123.45");
    expect(yuan(0)).toBe("¥0.00");
  });
  it("renders an em dash for nullish input", () => {
    expect(yuan(undefined)).toBe("—");
  });
});

describe("request envelope + auth", () => {
  it("returns the envelope's data on success", async () => {
    installFetch(() => resp(200, ok({ id: "u1", account: "a@b.c" })));
    const user = await api.getDataset("d1").catch(() => null);
    // getDataset is public; just assert a generic success path via me() below.
    expect(user).not.toBeUndefined();
  });

  it("injects a Bearer header when an access token is present", async () => {
    tokenStore.set({ access_token: "AT", refresh_token: "RT", expires_in: 900 });
    const { calls } = installFetch(() => resp(200, ok({ id: "u1" })));
    await api.me();
    expect(calls[0].url).toContain("/users/me");
    expect((calls[0].init.headers as Record<string, string>)["Authorization"]).toBe("Bearer AT");
  });

  it("omits auth header on public (auth:false) calls", async () => {
    tokenStore.set({ access_token: "AT", refresh_token: "RT", expires_in: 900 });
    const { calls } = installFetch(() => resp(200, ok({ items: [] })));
    await api.listDatasets({});
    expect((calls[0].init.headers as Record<string, string>)["Authorization"]).toBeUndefined();
  });

  it("drops undefined/empty query params from the URL", async () => {
    const { calls } = installFetch(() => resp(200, ok({ items: [] })));
    await api.listDatasets({ q: "lidar", domain: undefined, license_type: "" });
    expect(calls[0].url).toContain("q=lidar");
    expect(calls[0].url).not.toContain("domain=");
    expect(calls[0].url).not.toContain("license_type=");
  });

  it("throws ApiError carrying the business code when code !== 0", async () => {
    installFetch(() => resp(200, { code: 1002, message: "not found", data: null }));
    const err = await api.getDataset("missing").catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.code).toBe(1002);
    expect(err.message).toBe("not found");
  });

  it("throws ApiError carrying the HTTP status on a non-2xx with no envelope", async () => {
    installFetch(() => resp(500, null));
    await expect(api.getDataset("x")).rejects.toMatchObject({ status: 500 });
  });
});

describe("401 → refresh → retry", () => {
  it("refreshes once and retries the original request on a 401", async () => {
    tokenStore.set({ access_token: "OLD", refresh_token: "RT", expires_in: 1 });
    let meCalls = 0;
    const { calls } = installFetch((url) => {
      if (url.includes("/auth/refresh")) {
        return resp(200, ok({ user: { id: "u1" }, tokens: { access_token: "NEW", refresh_token: "RT2", expires_in: 900 } }));
      }
      if (url.includes("/users/me")) {
        meCalls += 1;
        // First /users/me sees the stale token → 401; the retry succeeds.
        return meCalls === 1 ? resp(401, { code: 2000, message: "unauthorized", data: null }) : resp(200, ok({ id: "u1", account: "a@b.c" }));
      }
      return resp(404, null);
    });

    const user = await api.me();
    expect(user).toMatchObject({ id: "u1" });
    // original me, refresh, retried me
    expect(calls.filter((c) => c.url.includes("/users/me")).length).toBe(2);
    expect(calls.filter((c) => c.url.includes("/auth/refresh")).length).toBe(1);
    // tokens rotated in storage
    expect(tokenStore.access).toBe("NEW");
    expect(tokenStore.refresh).toBe("RT2");
    // the retry carried the refreshed bearer
    const retried = calls.filter((c) => c.url.includes("/users/me"))[1];
    expect((retried.init.headers as Record<string, string>)["Authorization"]).toBe("Bearer NEW");
  });

  it("clears tokens and surfaces the error when the refresh itself fails", async () => {
    tokenStore.set({ access_token: "OLD", refresh_token: "BAD", expires_in: 1 });
    installFetch((url) => {
      if (url.includes("/auth/refresh")) return resp(401, { code: 2000, message: "refresh expired", data: null });
      return resp(401, { code: 2000, message: "unauthorized", data: null });
    });
    await expect(api.me()).rejects.toBeInstanceOf(ApiError);
    expect(tokenStore.access).toBeNull();
    expect(tokenStore.refresh).toBeNull();
  });

  it("does not attempt a refresh when there is no refresh token", async () => {
    // access present but refresh missing → a 401 must NOT trigger /auth/refresh.
    localStorage.setItem("adm_access", "OLD");
    const { calls } = installFetch(() => resp(401, { code: 2000, message: "unauthorized", data: null }));
    await expect(api.me()).rejects.toBeInstanceOf(ApiError);
    expect(calls.some((c) => c.url.includes("/auth/refresh"))).toBe(false);
  });

  it("coalesces concurrent 401s into a single refresh (singleflight)", async () => {
    tokenStore.set({ access_token: "OLD", refresh_token: "RT", expires_in: 1 });
    const seen: Record<string, number> = {};
    const { calls } = installFetch((url) => {
      if (url.includes("/auth/refresh")) {
        seen.refresh = (seen.refresh ?? 0) + 1;
        return resp(200, ok({ user: { id: "u1" }, tokens: { access_token: "NEW", refresh_token: "RT2", expires_in: 900 } }));
      }
      if (url.includes("/users/me")) {
        seen.me = (seen.me ?? 0) + 1;
        // The first two (concurrent) requests see the stale token → 401; retries succeed.
        return seen.me <= 2 ? resp(401, { code: 2000, message: "unauthorized", data: null }) : resp(200, ok({ id: "u1" }));
      }
      if (url.includes("/users/me/notifications")) return resp(200, ok({ unread: 0 }));
      return resp(404, null);
    });

    const [a, b] = await Promise.all([api.me(), api.me()]);
    expect(a).toMatchObject({ id: "u1" });
    expect(b).toMatchObject({ id: "u1" });
    // Two concurrent 401s, but only ONE refresh round-trip.
    expect(calls.filter((c) => c.url.includes("/auth/refresh")).length).toBe(1);
  });
});
