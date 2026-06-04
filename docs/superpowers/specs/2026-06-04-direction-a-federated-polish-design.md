# Direction A: Federated Polish — Metrics + Dataset Names + Pagination

**Date**: 2026-06-04  
**Baseline**: `origin/main @ 4e718cc`  
**Scope**: One PR, pure local, no external dependencies.

## A1. Federated Prometheus Metrics

### New metrics in `platform/metrics/metrics.go`

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `marketplace_federated_jobs_total` | Counter | `status` (released/failed/rejected) | Federated jobs reaching terminal state |
| `marketplace_federated_aggregation_duration_seconds` | Histogram | — | Wall-clock of `aggregateAndRelease` |
| `marketplace_federated_participants` | Counter | `role` (submitted/survived/dropped) | Participant outcomes across all federated jobs |

### Instrumentation points in `compute/federated.go`

- `tryAdvanceFederated`: at each terminal transition → `RecordFederatedJob(status)`
- `aggregateAndRelease` entry/exit → `ObserveFederatedAggregation(seconds)`
- `aggregateAndRelease` count survivors → `RecordFederatedParticipants("survived", n)`; count dropped via `total - survived` → `RecordFederatedParticipants("dropped", n)`
- `SubmitFederatedJob` after successful fanout → `RecordFederatedParticipants("submitted", len(datasets))`

### Testing

Unit test: call each `Record*`/`Observe*` function, assert no panic and counter increments (prometheus testutil).

## A2. Dataset Names in Federated Panel

### Problem

`FederatedComputePanel` shows `e.dataset_id.slice(0,8)` — cryptic UUIDs instead of human-readable titles.

### Solution (frontend-only)

1. On mount, collect all unique dataset IDs from `ents` + `feds[].dataset_ids`.
2. Batch-fetch via existing `api.getDataset(id)` (public endpoint, no auth). Use `Promise.allSettled` for resilience.
3. Store in `Map<string, string>` (id → title). Display `names.get(id) || id.slice(0,8)` everywhere.
4. Applies to both the entitlement checklist (picking datasets) and the federated job list.

No backend change needed.

## A3. Federated Job Pagination + Detail Expand

### Pagination

- Backend `listMyFederated` already accepts `limit`/`offset` query params.
- Frontend: add `limit=10` to the initial fetch. Add "加载更多 / Load more" button that increments offset and appends results.
- `api.listMyFederatedJobs` gains optional `limit`/`offset` params.
- Track `hasMore` (if returned items < limit, no more pages).

### Detail expand

- Click a federated job row → toggle expanded state.
- Expanded view calls `api.getFederatedJob(id)` → shows sub-jobs table:
  - Dataset name (from the name map above), status badge, timing.
- Only one job expanded at a time (accordion).

### Testing

- Backend: existing integration tests cover `limit`/`offset`; no new backend code.
- Frontend: `npm run typecheck && npm run lint && npm run build`.

## Files touched

- `backend/internal/platform/metrics/metrics.go` — 3 new metrics + 3 helper functions
- `backend/internal/modules/compute/federated.go` — instrument 4 points
- `frontend/components/Compute.tsx` — dataset name resolution, pagination, detail expand
- `frontend/lib/api.ts` — add `limit`/`offset` to `listMyFederatedJobs` signature
