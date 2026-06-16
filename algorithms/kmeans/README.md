# kmeans — Compute-to-Data sandbox algorithm

A **trusted whitelist** algorithm for the Verdant Oasis 可用不可见 (L1) sandbox: it
clusters a dataset **inside the platform sandbox** with k-means and returns only
the **centroids + aggregate metrics** — the buyer never receives the raw data, and
never per-row cluster assignments.

## Why it looks the way it does (security)

Same posture as [`logreg`](../logreg/README.md). On an L1 offer the **audited
algorithm code — not the sandbox — is the real boundary** (design §2 / §7.3):

- **Tiny & dependency-light** — pure `numpy` + `pandas`, easy to audit.
- **Aggregates only** — centroids, per-cluster sizes, inertia. **Never** per-row
  cluster labels (that would be high-fidelity per-row leakage, §7.3). A test
  asserts the output contains no per-row-length arrays.
- **JSON model, not pickle** — output is a zip of `model.json` + `metrics.json`.
- **No raw data in logs** — stdout carries only structured progress JSON.
- **Deterministic** — seeded k-means++ init, no wall-clock/random-device entropy,
  so a job reproduces for dispute re-computation (§3 / §21).

## Container contract

| Path | Mode | Purpose |
|------|------|---------|
| `/data` | read-only | the dataset (`.csv`/`.tsv`); reads the first tabular file |
| `/params.json` | read-only | optional `{ "k": 3, "max_iter": 50, "features": ["x","y"] }` |
| `/out/output.bin` | write | a zip of `model.json` + `metrics.json` (the single output) |

Run by the platform (identical hardening to logreg):

```
docker run --rm --network=none --read-only --security-opt=no-new-privileges \
  --cap-drop=ALL --pids-limit=128 --memory=512m --cpus=1 \
  --tmpfs=/tmp:rw,size=64m,nodev,nosuid,noexec \
  -v <data>:/data:ro -v <out>:/out -v <params>:/params.json:ro \
  <registry>/vo-kmeans@sha256:<digest>
```

## Output

`model.json` → `{ format, features, k, centroids, centroids_standardized, mean, std }`
`metrics.json` → `{ k, n_samples, n_features, inertia, cluster_sizes, largest_cluster_fraction }`

## Test
```
python -m pytest algorithms/kmeans/test_train.py     # needs numpy/pandas
# or end-to-end in the image (no local deps): see the contract above
```
