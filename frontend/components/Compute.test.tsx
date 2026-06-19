import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LocaleProvider } from "@/lib/i18n";
import type { FederatedJob, User } from "@/lib/api";
import { FederatedComputePanel } from "./Compute";

// ---------------------------------------------------------------------------
// The federated panel and the PSI panel SHARE one endpoint
// (listMyFederatedJobs returns both fed and psi jobs). The federated panel
// filters psi out for display, but pagination offsets are over the RAW result
// set. So when psi jobs are interleaved, using the *filtered* fed count as the
// load-more offset re-requests rows already shown → duplicate rows / duplicate
// React keys. These tests pin the correct behavior.
// ---------------------------------------------------------------------------

const buyer: User = {
  id: "u1", account: "a@b.c", account_type: "email", role: "buyer", kyc_status: "none", status: "active",
};

vi.mock("@/lib/auth", () => ({ useAuth: () => ({ user: buyer, loading: false }) }));

const listMyFederatedJobs = vi.fn();
vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    api: {
      ...actual.api,
      listMyFederatedJobs: (...a: unknown[]) => listMyFederatedJobs(...a),
      listMyComputeEntitlements: vi.fn().mockResolvedValue({ items: [] }),
      listComputeAlgorithms: vi.fn().mockResolvedValue({ items: [] }),
      getDataset: vi.fn().mockResolvedValue({ id: "ds1", title: "DS One" }),
    },
  };
});

// A "canceled" status is terminal (no background polling fires) and renders a
// plain row with no certificate/download network calls — ideal for an
// isolated pagination test.
const pad = (n: number) => String(n).padStart(2, "0");
const fedJob = (n: number): FederatedJob => ({
  id: `fedjob${pad(n)}`, buyer_id: "u1", dataset_ids: ["ds1"], mode: "fed",
  status: "canceled", min_participants: 2,
});
const psiJob = (n: number): FederatedJob => ({
  id: `psijob${pad(n)}`, buyer_id: "u1", dataset_ids: ["ds1"], mode: "psi",
  status: "canceled", min_participants: 2,
});

// 20 jobs, strictly interleaved fed/psi: fed01,psi01,fed02,psi02,…,fed10,psi10.
// The endpoint returns this raw order; offsets are over the WHOLE list.
const master: FederatedJob[] = Array.from({ length: 10 }, (_, i) => [fedJob(i + 1), psiJob(i + 1)]).flat();

function renderPanel() {
  return render(
    <LocaleProvider>
      <FederatedComputePanel />
    </LocaleProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  // Serve the requested raw window [offset, offset+limit) of the shared list.
  listMyFederatedJobs.mockImplementation((limit: number, offset: number) => {
    const start = offset ?? 0;
    return Promise.resolve({ items: master.slice(start, start + (limit ?? 10)) });
  });
});

const loadMoreBtn = () => screen.getByRole("button", { name: /加载更多|Load more/ });

describe("FederatedComputePanel — load more with interleaved PSI jobs", () => {
  it("does not duplicate federated rows after loading more", async () => {
    renderPanel();
    // Page 1 (raw offset 0, limit 10) = fed01,psi01…fed05,psi05 → 5 fed shown.
    await screen.findByText("fedjob01");
    expect(screen.getByText("fedjob05")).toBeInTheDocument();

    await userEvent.click(loadMoreBtn());
    // After load-more, page 2's fed jobs (fed06…fed10) appear…
    await screen.findByText("fedjob06");
    expect(screen.getByText("fedjob10")).toBeInTheDocument();

    // …and EVERY fed row appears exactly once. The bug re-requested rows from a
    // too-small offset, re-rendering fed04/fed05 a second time (duplicate keys).
    for (let n = 1; n <= 10; n++) {
      expect(screen.getAllByText(`fedjob${pad(n)}`)).toHaveLength(1);
    }
  });

  it("never shows PSI jobs in the federated list after loading more", async () => {
    renderPanel();
    await screen.findByText("fedjob01");
    expect(screen.queryByText("psijob01")).not.toBeInTheDocument();

    await userEvent.click(loadMoreBtn());
    await screen.findByText("fedjob06");

    // No psi job — already-seen or newly fetched — leaks into the fed list.
    for (let n = 1; n <= 10; n++) {
      expect(screen.queryByText(`psijob${pad(n)}`)).not.toBeInTheDocument();
    }
  });
});
