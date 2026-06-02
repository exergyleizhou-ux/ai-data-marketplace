// Package compute implements Compute-to-Data (隐私计算 / 可用不可见): a buyer
// purchases a compute entitlement on a dataset, submits a WHITELISTED algorithm
// job, and receives the OUTPUT (model / metrics) — never the raw data. This is
// the L1 "buyer-invisible" trust level of the design doc
// (docs/设计文档-隐私计算与可用不可见(Compute-to-Data).md). L2 (TEE) and L3
// (federated / MPC) build on this in later phases.
//
// Boundary: owns `algorithms`, `dataset_compute_offers`, `compute_entitlements`,
// `compute_jobs`, `dp_budget_ledger`. It depends on auth/dataset only through
// injected interfaces (IdentityChecker, DatasetReader) — never importing them.
//
// Job state machine (design §3): every transition is an optimistic, single-row
// UPDATE (WHERE status=from) so the machine is enforced at the DB too.
//
//	created → queued → running → output_pending ─┬→ released
//	                                              └→ output_reviewing → released / rejected
//	          running → queued (crash retry, attempts < max) | failed (exhausted)
//	          created/queued → canceled (buyer cancels before run)
//
// Security invariant (design §2 / §7.3): on an L1 offer a MODEL-producing job
// may run ONLY a platform-audited (trusted) whitelist algorithm — the audited
// code, not the sandbox, is the real boundary against the model memorizing raw
// rows. Enforced in resolveAlgorithm.
//
// Phase 1 scope: schema (migration 000010) + repo + service (this PR). The
// runner/worker, HTTP API, purchase wiring, and frontend land in follow-ups.
package compute
