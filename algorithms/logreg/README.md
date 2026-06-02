# logreg — Compute-to-Data sandbox algorithm

A **trusted whitelist** algorithm for the Verdant Oasis 可用不可见 (L1) sandbox:
it trains a logistic-regression classifier on a dataset **inside the platform
sandbox** and returns only the **model + aggregate metrics** — the buyer never
receives the raw data.

## Why it looks the way it does (security)

On an L1 offer the **audited algorithm code — not the sandbox — is the real
boundary** that keeps raw rows from leaking (design doc §2 / §7.3). So this
algorithm is deliberately:

- **Tiny & dependency-light** — pure `numpy` + `pandas`, ~150 lines, easy to audit.
- **Output-shape limited** — returns the final model + aggregate metrics only;
  **never** per-row predictions / embeddings / nearest-neighbours (high-fidelity
  leakage, §7.3).
- **JSON model, not pickle** — the output is a zip of `model.json` + `metrics.json`,
  so it can never be a deserialization-RCE vector for the buyer (§7.4).
- **No raw data in logs** — stdout carries only structured progress JSON
  (counts/metrics), so logs aren't an exfiltration side channel (§7.4).
- **Deterministic** — zero-init batch gradient descent, no shuffling, so a job
  can be reproduced for dispute re-computation (§3 / §21).

## Container contract

| Path | Mode | Purpose |
|------|------|---------|
| `/data` | read-only | the dataset (a `.csv`/`.tsv`); the algorithm reads the first tabular file |
| `/params.json` | read-only | optional `{ "target": "<column>" }` (default: last column) |
| `/out/output.bin` | write | a zip of `model.json` + `metrics.json` (the single output object) |

Run by the platform as:

```
docker run --rm --network=none --read-only --security-opt=no-new-privileges \
  --cap-drop=ALL --pids-limit=128 --memory=512m --cpus=1 \
  --tmpfs=/tmp:rw,size=64m,nodev,nosuid,noexec \
  -v <data>:/data:ro -v <out>:/out -v <params>:/params.json:ro \
  <registry>/vo-logreg@sha256:<digest>
```

(See `backend/internal/modules/compute/runner_docker.go` — `dockerRunArgs`.)

## Test locally (no Docker, no sklearn)

```
~/sdk/sidecar-venv/bin/python algorithms/logreg/test_train.py
# or: python -m pytest algorithms/logreg/test_train.py
```

## Register it (ops)

1. Build + push the image; capture the `sha256:` digest (see `Dockerfile`).
2. `POST /admin/compute/algorithms` with `runtime=python-sklearn`,
   `image=<registry>/vo-logreg`, `image_digest=sha256:…`, `output_kind=model`,
   `source_ref=<git ref>`.
3. `POST /admin/compute/algorithms/:id/review` with `status=approved`,
   `trusted=true` (required for L1 model output).
