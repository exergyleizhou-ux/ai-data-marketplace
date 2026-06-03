# fed-logreg — federated logistic regression (local trainer)

A TRUSTED whitelist algorithm for the Compute-to-Data sandbox. One instance runs
per dataset inside its own `--network none --read-only` sandbox, trains a local
logistic regression on that seller's data, and emits ONLY the local model
parameters. The platform aggregates the N parties' params with **FedAvg**
(sample-weighted mean) into a joint model. Raw data never leaves a sandbox
(design P4 §2.1).

## Container contract

- Input: dataset mounted read-only at `/data` (first `.csv`/`.tsv`). Optional
  `/params.json`.
- Output: `/out/output.bin` = **raw JSON** (not a zip) — the federated partial:

  ```json
  {"_format":"fedparams-v1","features":["x1","x2"],"weights":[0.1,-0.2],"intercept":0.05,"n":120}
  ```

- Params: `target` (default last column), `features` (explicit ordered list —
  **use this to guarantee cross-party alignment**).

## FedAvg precondition

All parties must share the **same feature schema and order**. Training is on
**raw features** (no per-party standardization — that would make weights
incomparable across parties). Pass `params.features` to lock the order. Joint
weights = `Σ(n_k·w_k)/Σ n_k`.

## Honest scope

Real local training + central FedAvg. NOT (yet): secure aggregation (the
platform sees each party's params), cross-party feature standardization, or
DP-SGD (central DP is available separately via the federated job's `dp_epsilon`).

## Build & publish

```bash
docker build -t <registry>/vo-fed-logreg:1 algorithms/fed-logreg
docker push <registry>/vo-fed-logreg:1
docker inspect --format='{{index .RepoDigests 0}}' <registry>/vo-fed-logreg:1
# register: image=<registry>/vo-fed-logreg, image_digest=sha256:...,
#           runtime=fed-logreg, output_kind=model, trusted=true
```

## Test

```bash
pytest algorithms/fed-logreg          # local unit tests
```
