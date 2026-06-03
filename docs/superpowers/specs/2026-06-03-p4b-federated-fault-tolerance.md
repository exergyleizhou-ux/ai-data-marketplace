# P4-b (slice 1) Federated Fault Tolerance — Spec

**Branch**: `feat/p4b-fault-tolerance` off `origin/main @ 60a9d05`
**Source**: P4 design doc §4 ("失败/部分参与策略:可设最小参与方数;掉队方按超时剔除"). Completes the `min_participants` field introduced in P4-a (currently always = N).

## Goal
A federated job tolerates sub-job dropouts: it succeeds and aggregates the survivors when `released ≥ min_participants`, instead of requiring all N. Below threshold it fails the whole job.

## Behavior (new `tryAdvanceFederated`)
Wait until **every** sub-job has settled (no pending), then decide once (idempotent via state transition):
- `released ≥ min_participants` (and ≥1): aggregate **only the released** partials → joint model released. The dropout (failed/rejected/canceled) sub-jobs are **refunded** (not billed); the released participants are billed.
- else: fail the whole federated job; **refund all** (buyer got no usable output).

MVP keeps the simple rule "decide once all sub-jobs are terminal" (no early-timeout dropout; that's a later step). No double-refund: each sub-job's quota is refunded at most once across all paths.

## Changes
- `model`/`FederatedSubmitInput` + HTTP `federatedSubmitRequest`: `min_participants` (0 ⇒ N; validated `2 ≤ min ≤ N`).
- `SubmitFederatedJob`: validate + set `MinParticipants`.
- `tryAdvanceFederated`: survivors-vs-fail decision above.
- `aggregateAndRelease(fed, released)`: takes the released subset; failure path refunds only those.
- `CancelJob`: block canceling a federated sub-job directly (consistency with GetJob/OpenOutput).

## Tests (real PG)
- **Tolerance success**: 3 datasets (ds3 has a tiny output cap → its sub-job is gate-rejected), `min_participants=2` → federated released; joint == FedAvg of ds1+ds2 partials (participants=2); ds3 refunded (jobs_used 0); ds1/ds2 billed (jobs_used 1).
- **Below threshold**: ds2+ds3 tiny cap (2 fail), `min_participants=2` → only 1 released < 2 → federated failed; all 3 refunded.
- Existing federated tests (all-success, all-or-nothing failure with default min=N) keep passing.

## Verify
`cd backend`; gofmt/build/vet; `go test -race` + ephemeral PG (all packages); frontend typecheck/lint/build.
