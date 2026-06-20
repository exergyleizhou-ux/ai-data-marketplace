# Design — C2D Anti-Malicious-Algorithm Output Gate (Part A · A2) + Threat-Model Whitepaper (A1)

Date: 2026-06-20
Branch: `feat/c2d-output-gate`
Status: approved (forks: defer output-DP; code-constant policy, no migration)

## 0. Context

Oasis runs buyer-purchased computations against a seller's dataset inside a
hardened sandbox (`docker run --network=none --read-only --user 65534`,
`backend/internal/modules/compute/runner_docker.go`). The dataset is mounted
read-only at `/data`; the algorithm writes a single object `/out/output.bin`;
the network is severed, so **the output object is the only exfiltration
channel**.

Today's output gate (`worker.go` `processJob`, ~line 229) caps the output
**size** only:

```go
if maxOutputBytes > 0 && int64(len(res.Output)) > maxOutputBytes {
    s.rejectJob(ctx, jobID, job.EntitlementID, "output_exceeds_max_bytes")
}
```

This defends a *semi-honest buyer* but not a **malicious algorithm author**: the
default cap is 64 MiB, ample room to steganographically encode thousands of raw
rows into a claimed "aggregate" — e.g. base64 the whole CSV into one JSON string,
or emit a "model" that is a verbatim row-dump. A2 closes that gap.

## 1. Ground truth (what the runners actually emit)

- **Real algorithms** (`algorithms/*/`, e.g. `causal-mediation/mediate.py`,
  `dp_stats/stats.py`) write `output.bin` as a **ZIP** whose entries are
  `model.json` and/or `metrics.json` (`zipfile.ZIP_DEFLATED`, `z.writestr`).
- **MockRunner** (`runner.go`, used in CI/dev) emits a **single raw JSON object**
  (`mock-model-v1`, `mock-metrics-v1`).
- **Federated sub-jobs** emit raw JSON `fedparams-v1` (weights/intercept/n).
- **PSI sub-jobs** emit raw JSON `psi-set-v1` (a small `elements` string array).

Consequence: a "must be ZIP" gate (the handoff's first draft) would reject the
live MockRunner + federated/PSI MVP paths. The gate must accept **JSON object OR
zip-of-json**.

## 2. The gate

New file `backend/internal/modules/compute/output_gate.go`. Pure, fully
unit-testable, no Docker/DB:

```go
type GatePolicy struct {
    MaxBytes            int64   // outer size cap (offer.MaxOutputBytes, or kind default)
    MaxStringBytes      int     // total string-leaf bytes across all parsed JSON
    MaxNumericValues    int     // total numeric leaves
    MaxKeys             int     // total object keys (structural sanity)
    MaxDepth            int     // max JSON nesting depth
    EntropyStringMinLen int     // only strings >= this length get the entropy check
    EntropyMaxBits      float64 // reject a long string above this Shannon bits/byte
}

// GateOutput validates a runner's output against the policy for its kind.
// Returns a non-nil *GateViolation (reason code + detail) when the output must
// be withheld; nil when it passes. Never mutates the output.
func GateOutput(kind string, output []byte, p GatePolicy) *GateViolation
```

### 2.1 Container shape (rejects everything that is not structured JSON)

Accept exactly two shapes; reject all else with reason `output_not_structured`:

1. A single well-formed **JSON object** (`{...}`), OR
2. A **ZIP** whose every entry name ends in `.json` and whose every entry parses
   as a JSON value.

Rejected: raw binary blobs, tarballs, CSV, a ZIP containing a `.bin`/`.csv`
entry, malformed JSON, a top-level JSON array/scalar. This single rule kills
"dump the dataset as `output.bin`" and "smuggle a CSV inside the zip."

### 2.2 Information-content bounds (the anti-exfil teeth)

Walk the parsed JSON (summing across all zip entries) and enforce:

- **MaxStringBytes** — total bytes of all string leaves. *Highest leverage*:
  base64-a-dataset-into-a-field is the easiest exfil vector. Tight for
  aggregate/metrics/table (8 KiB); looser for model (64 KiB).
- **MaxNumericValues** — total numeric leaves. An aggregate is O(k) statistics; a
  flattened dataset is O(N·cols). 10k for aggregate/metrics, 200k for model
  (real weight vectors are large but bounded).
- **MaxKeys / MaxDepth** — structural sanity to bound a pathological tree.
- **MaxBytes** — keep the existing byte cap as the outer bound; apply a tighter
  *kind default* when the offer sets none.

### 2.3 Entropy (secondary heuristic, string-scoped)

For any string leaf with length ≥ `EntropyStringMinLen`, compute Shannon entropy
(bits/byte). Reject if > `EntropyMaxBits`. Rationale: a long, high-entropy string
is a compressed/encrypted/base64 payload. Applied **post-parse on strings only**
— raw-byte entropy is useless (a ZIP is always high-entropy). Conservative
thresholds (min length 256, ~4.7 bits/byte) so SHA-256 hashes / UUIDs / IDs do
not trip it. Documented as a heuristic; `MaxStringBytes` is the primary defense.

### 2.4 Kind-specific default policy (code constants, no migration)

```
aggregate / metrics / table : MaxBytes 1 MiB,  MaxStringBytes 8 KiB,  MaxNumericValues 10_000,  MaxKeys 5_000,  MaxDepth 12
model                       : MaxBytes 8 MiB,  MaxStringBytes 64 KiB, MaxNumericValues 200_000, MaxKeys 50_000, MaxDepth 16
```

`offer.MaxOutputBytes` (when > 0) overrides `MaxBytes`. Per-offer tuning of the
other bounds is a deliberate follow-up (would need a migration + job snapshot,
mirroring the `review_output`/`max_output_bytes` snapshot in migration 000028).

## 3. Integration

In `worker.go` `processJob`, replace the size-only block with a single
`GateOutput(res.OutputKind, res.Output, policyFor(res.OutputKind, maxOutputBytes))`
call. On violation → `s.rejectJob(ctx, jobID, job.EntitlementID, v.Reason)`
(refunds credit, audits `compute.job.reject`, increments the rejected metric —
identical to the current size-gate handling). The gate runs **before**
store/release on **every** path (regular, review-staged, federated/PSI sub-job),
so coverage is uniform.

The runner-level bounded read in `runner_docker.go` stays as defense-in-depth
(the OOM guard during collection).

### 3.1 Compatibility (must hold — regression-tested)

`fedparams-v1`, `psi-set-v1`, `mock-model-v1`, `mock-metrics-v1` are small JSON
objects → they pass. Explicit table-test cases pin each real payload so the live
federated/PSI/mock paths keep working. The aggregated *joint* federated model is
produced by our trusted FedAvg aggregator (`dp.go`/`federated.go`), not by a
buyer-supplied algorithm, so it is out of this gate's threat model (and its
release path does not run through `processJob`).

## 4. Decisions (confirmed)

- **Accept JSON OR zip-of-json**, not zip-only (live MockRunner + federated/PSI
  emit JSON).
- **Defer output-layer DP noise.** Real DP already runs *inside* the algorithm
  (`_epsilon` → `dp.go`/`dp_stats`). Post-hoc noise on an opaque output we'd have
  to parse-and-rewrite is weaker (the author controls structure) and risks
  corrupting a valid model. The gate **detects + rejects**, never mutates.
  Output-DP is documented as a follow-up.
- **Code-constant policy, no migration.** Keeps the PR focused; per-offer
  tunability is a later migration.

## 5. Testing (TDD)

`output_gate_test.go`, table-driven:
- pass: single JSON object within bounds; zip{model.json,metrics.json} within
  bounds; real `mock-model-v1` / `mock-metrics-v1` / `fedparams-v1` /
  `psi-set-v1` payloads (live-path regression guards).
- reject: raw binary blob (not structured); zip with a `.csv` entry; zip with a
  malformed `.json` entry; top-level JSON array; a huge base64 string (string
  bytes / entropy); an N×cols numeric array (numeric count); oversize (bytes);
  malformed bytes.

Worker-level: extend the MockRunner test hook (`_mock_output_raw` for non-JSON
bytes; reuse `_mock_output_bytes` for oversize) so an integration test asserts a
malicious mock output → job `rejected` + quota refunded.

`go test ./internal/modules/compute/...` green; `go vet` clean.

## 6. A1 — Threat-model & guarantees whitepaper

- New `docs/威胁模型与保证-C2D可验证证据层.md` — formal doc.
- Upgrade `frontend/app/c2d/honesty/page.tsx` to frame **4 adversaries** and link
  the whitepaper.

Four adversaries, each with *where the system defends / where it fails*:
1. **Curious buyer** — defended: sandbox returns only the gated output, never raw
   rows; output gate (A2) bounds information content.
2. **Malicious buyer** — defended: custom algos need seller opt-in + (for model
   output on L1) a trusted/audited algorithm; DP budget ledger; output gate.
3. **Malicious algorithm author** — *partially* mitigated by A2's structural +
   magnitude gate. Residual: a determined author can still exfil *within* the
   bounds (a few KB of strings / 10k numbers). Disclosed honestly.
4. **Compromised operator** — **NOT** defended at L1 (operator can see staged
   data). L2 (TEE) is the design answer and is gated on real TEE hardware.
   Disclosed as a known gap.

The honest through-line, stated explicitly: **the certificate guarantees
*provenance* — that this audited algorithm image + this dataset produced this
exact output (re-hash verifiable) — NOT correctness, NOT safety, NOT
operator-invisibility.**
