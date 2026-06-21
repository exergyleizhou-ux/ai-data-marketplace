import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { VerifyChip, classifyVerifyError } from "./VerifyChip";
import { api } from "@/lib/api";

vi.mock("@/lib/api", () => ({ api: { verifyCertificate: vi.fn() } }));
const mockVerify = vi.mocked(api.verifyCertificate);

// The substantive behaviour change is the error classification: a 404 (cert not
// hosted on THIS instance — e.g. a production-only flagship cert viewed on the
// demo) must read as an honest "absent", never as a failure/retry that implies
// the cert is broken; everything else stays retryable. Tested as a pure function
// so it's deterministic and free of the mount-effect async noise.
describe("classifyVerifyError", () => {
  it("classifies a 404 as 'absent' (not on this instance)", () => {
    expect(classifyVerifyError({ status: 404 })).toBe("absent");
  });
  it("classifies a transient/network error as 'neterr' (retryable)", () => {
    expect(classifyVerifyError({ status: 0 })).toBe("neterr");
    expect(classifyVerifyError({ status: 500 })).toBe("neterr");
    expect(classifyVerifyError(new Error("boom"))).toBe("neterr");
    expect(classifyVerifyError(null)).toBe("neterr");
  });
});

describe("VerifyChip", () => {
  it("shows live-verifiable when the cert verifies", async () => {
    // useT (real i18n) falls back to the zh string with no provider.
    mockVerify.mockResolvedValue({ verifiable: true } as never);
    render(<VerifyChip certId="VO-REAL" />);
    expect(await screen.findByText(/实时可验证/)).toBeInTheDocument();
  });
});
