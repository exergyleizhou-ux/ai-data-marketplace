# P4-b (slice 3) Real fed-logreg image + docker federated e2e — Spec

**Branch**: `feat/p4b-fed-image` off `origin/main @ 5c08596`
**Goal**: Replace the MockRunner stand-in for federated sub-jobs with a REAL trusted algorithm image. Each sub-job trains a local logistic regression inside its own `--network=none` sandbox and emits `fedparams-v1` local params; the platform aggregates with the existing real FedAvg. Verified end-to-end on real Docker via a digest-pinned image + local registry.

## Algorithm `algorithms/fed-logreg/`
- `train.py`: trusted whitelist trainer. Reads `/data` (read-only), optional `/params.json`, writes `/out/output.bin` = **raw `fedparams-v1` JSON** (NOT a zip — the federated path reads it via `parsePartial`):
  `{"_format":"fedparams-v1","features":[...],"weights":[f64...],"intercept":f64,"n":int}`.
  - target = `params.target` or last column; features = `params.features` (explicit, ordered) or all numeric non-target columns sorted by name.
  - Train logistic regression on **raw features** (deterministic batch GD, zero init, no shuffle) — NO per-party standardization, so weights are directly comparable for FedAvg.
  - Security (same as logreg, design §7.3/§7.4): no network; structured JSON logs only (never raw rows); JSON output (no pickle); aggregate params only.
  - **FL precondition (documented)**: parties must share the feature schema/order; cross-party standardization is out of scope (later slice).
- `Dockerfile`: `python:3.11-slim` + numpy/pandas, `USER 65534`, `ENTRYPOINT ["python","/app/train.py"]` (mirror logreg).
- `test_train.py`: pytest (runs in sidecar CI) — trains on a tiny CSV, asserts output is `fedparams-v1` with right dims + parsePartial-compatible; two schema-identical datasets produce averageable params.
- `README.md`: contract + build/publish + FedAvg note.

## Backend e2e `federated_docker_e2e_integration_test.go`
Gated (skips unless set): `DATABASE_URL`, `COMPUTE_FED_E2E_IMAGE`, `COMPUTE_FED_E2E_DIGEST`, reachable docker. Two datasets (real staged CSVs, shared schema), register the digest-pinned `fed-logreg` algorithm trusted, offers allow_federated, entitlements, `dockerRunner` worker, `SubmitFederatedJob` → real sandboxed sub-jobs each emit fedparams-v1 → real FedAvg → buyer downloads the joint `fedmodel-v1`. Assert joint == weighted mean of the two sandboxes' params; partials not buyer-downloadable.

## Verify (real Docker, local registry)
```
docker run -d -p 5001:5000 --name registry registry:2
docker build -t localhost:5001/vo-fed-logreg:1 algorithms/fed-logreg && docker push localhost:5001/vo-fed-logreg:1
# digest = docker inspect --format='{{index .RepoDigests 0}}' ...
DATABASE_URL=<ephemeral> COMPUTE_FED_E2E_IMAGE=localhost:5001/vo-fed-logreg COMPUTE_FED_E2E_DIGEST=sha256:... \
  go test -run TestComputeFederatedDockerE2E ./internal/modules/compute/
```
Plus: sidecar `pytest algorithms/fed-logreg`; backend gofmt/build/vet/test-race; `publish.sh` gains fed-logreg.

## Honest boundary
Real local training per party; central FedAvg. Still NOT: secure aggregation (platform sees each party's params), cross-party standardization, DP-SGD (central DP already available via fed.dp_epsilon). Those are later P4-b/c slices.
