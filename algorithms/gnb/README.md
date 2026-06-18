# gnb — Compute-to-Data sandbox algorithm (Gaussian Naive Bayes)

A **trusted whitelist** classifier for the Verdant Oasis 可用不可见 (L1) sandbox: it
fits a Gaussian Naive Bayes model on a labelled dataset **inside the platform
sandbox** and returns only the **per-class feature statistics + aggregate
metrics** — the buyer never receives the raw rows, and never per-row predictions.

## Why it looks the way it does (security)

Same posture as [`kmeans`](../kmeans/README.md). On an L1 offer the **audited
algorithm code — not the sandbox — is the real boundary** (design §2 / §7.3):

- **Tiny & dependency-light** — *pure Python stdlib*, no numpy/pandas/sklearn.
  The whole audited surface is one small file; the image needs no pip install.
- **Aggregates only** — per-class priors, feature means/variances, and the train
  accuracy. **Never** per-row predictions (that would be high-fidelity per-row
  leakage, the same reason kmeans never returns per-row cluster labels, §7.3).
- **JSON model, not pickle** — output is a zip of `model.json` + `metrics.json`,
  so it can never be a deserialization-RCE vector for the buyer (§7.4).
- **No raw data in logs** — stdout carries only structured progress JSON.
- **Deterministic** — closed-form fit, no RNG / wall-clock entropy, so a job
  reproduces for dispute re-computation (§3 / §21).

## Container contract

| Path | Mode | Purpose |
|------|------|---------|
| `/data` | read-only | the dataset (`.csv`/`.tsv`); reads the first tabular file |
| `/params.json` | read-only | optional `{ "label": "label", "features": ["f1","f2"] }` |
| `/out/output.bin` | write | a zip of `model.json` + `metrics.json` (the single output) |

Paths are overridable via `VO_DATA_DIR` / `VO_OUT_DIR` / `VO_PARAMS` for local tests.
Run by the platform with identical hardening to kmeans (`--network=none
--read-only --cap-drop=ALL --pids-limit --memory --cpus --tmpfs`).

## Output

`model.json` → `{ format, features, classes, priors, theta, var, var_epsilon }`
`metrics.json` → `{ format, n_train, n_features, classes, class_counts, accuracy, n_correct }`

## Test
```
python -m pytest algorithms/gnb/test_train.py      # pure stdlib, no deps needed
```
