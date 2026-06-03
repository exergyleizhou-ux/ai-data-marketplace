# Federated compute, end-to-end usable — Spec

**Branch**: `feat/federated-ux` off `origin/main @ 5cb9247`
**Strategic goal**: The L3 federated backend (FedAvg + tolerance + DP + real image) is built but unreachable from the UI. Close the UX gap so the headline differentiator is actually usable/demoable. Convert backend capability → product capability.

## Backend (the UI needs a list endpoint)
- `repo.ListFederatedJobsByBuyer(ctx, buyerID, limit, offset) ([]FederatedJob, error)` — mirror `ListJobsByBuyer`.
- `Service.ListFederatedJobs(ctx, buyerID, limit, offset)`.
- `handler.listMyFederated` + route `GET /users/me/compute/federated-jobs`.
- Test: real-PG list returns the buyer's federated jobs (newest first), scoped to buyer.

## Frontend
- `api.ts`: `FederatedJob` + `FederatedSubJob` types; methods `submitFederatedJob`, `getFederatedJob` (→ `{federated_job, sub_jobs}`), `listMyFederatedJobs`, `downloadFederatedOutput` (bearer-fetch blob like `downloadComputeOutput`).
- `Compute.tsx`: a **Federated** section in the buyer view —
  - pick ≥2 datasets that `allow_federated` (entitlement required per dataset), choose a `fed-logreg` algorithm, set `min_participants` (default = N), submit.
  - list my federated jobs with status; expand to per-sub-job status; when `released`, a "download joint model" button; show DP note when dp_epsilon set.
  - honest labels: "数据不出域 / data-stays-home", "联合模型 = N 方 FedAvg", participants count.
- Seller offer editor: `allow_federated` toggle (the API field exists; expose it).
- i18n: all new strings via `useT` (中/EN), matching the existing pattern in `i18n.tsx`.

## Verify
Backend: gofmt/build/vet + `go test -race` + real PG (new list test).
Frontend: `npm run typecheck && npm run lint && npm run build`.
(UI interaction is best-effort: build-verified here; full click-through needs the running stack.)

## Scope
This makes federated reachable + observable in the product. NOT in scope: secure aggregation, real TEE, MPC (gated/research, tracked separately).
