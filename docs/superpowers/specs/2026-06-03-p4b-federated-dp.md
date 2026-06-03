# P4-b (slice 2) Federated Differential Privacy — Spec

**Branch**: `feat/p4b-federated-dp` off `origin/main @ 6b52139`
**Source**: P4 design §5 + handoff §7.6 ("联邦仍可能经模型泄漏 → 叠加 DP"). Adds optional central-DP noise to the federated joint model.

## Goal
When a federated job sets `dp_epsilon`, the aggregated joint model is released with **central differential-privacy Laplace noise** instead of the raw FedAvg mean.

## Mechanism (honest, documented — no overclaiming)
1. **Clip** each party's weights/intercept per-coordinate to `[-C, C]` (`C` = `dp_clip`, default 10; overridable via `fed.Params["dp_clip"]`).
2. Weighted mean (FedAvg) on the clipped values.
3. Per-coordinate **Laplace(0, b)** noise, `b = Δ/ε`, sensitivity `Δ = 2·C·maxFrac`, `maxFrac = max(n_k)/Σn_k` (the max single-party weight fraction — the bounded-mean L1 sensitivity under clipping).
4. Output `fedmodel-v1` with a `dp` block `{mechanism:"laplace-central", epsilon, clip}`.

**Honesty caveat (in code + output + docs):** this is *central* DP on the released aggregate (platform clips+noises). It is NOT local DP / DP-SGD (per-example clipping at training time) — that's a later step. Noise uses `crypto/rand` (fresh, non-reproducible, matching `dp_stats`).

## Changes
- `compute/dp.go`: `laplaceNoise(b)` (crypto/rand), `dpFedAvg(partials, epsilon, clip, noiseFn)` (clip→mean→noise→fedmodel-v1+dp meta). Noise injected for deterministic tests.
- `aggregateAndRelease`: if `fed.DPEpsilon != nil` → use `dpFedAvg` (clip from `fed.Params["dp_clip"]` or default) instead of plain FedAvg; record DP spend per released dataset via `repo.SpendDP`.
- `dp_epsilon` already plumbed through `FederatedSubmitInput`→`fed`.

## Tests
- Unit (`dp_test.go`): noiseFn=0 ⇒ output == clipped weighted mean; clipping actually bounds an out-of-range weight; scale calc (noiseFn returns its `b`) ⇒ output == mean + Δ/ε; epsilon≤0 ⇒ error.
- Integration (real PG): federated job with `dp_epsilon` set → released joint model carries the `dp` block; two runs give different weights (noise is live/non-reproducible); dp ledger rows recorded.
- Existing federated tests (no dp_epsilon) unchanged (plain FedAvg).

## Verify
`cd backend`; gofmt/build/vet; `go test -race` + ephemeral PG.
