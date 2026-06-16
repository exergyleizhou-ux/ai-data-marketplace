#!/usr/bin/env python3
"""K-means clustering for the Verdant Oasis Compute-to-Data sandbox.

A TRUSTED whitelist algorithm (design §2 / §7.3): on an L1 offer the audited code
— not the sandbox — is what keeps raw rows from leaking, so this stays small,
dependency-light (pure numpy/pandas), and easy to audit. It mirrors the security
posture of the logreg algorithm:

  * Reads ONLY from the data dir; writes ONLY to the out dir. No network.
  * Emits ONLY structured progress logs (counts/metrics) — never raw rows (§7.4).
  * Output is a JSON model (centroids + standardisation), NOT a pickle, so it can
    never be a deserialization-RCE vector for the buyer (§7.4).
  * Returns AGGREGATES ONLY — centroids, per-cluster sizes, inertia. It NEVER
    emits per-row cluster assignments (which would be high-fidelity per-row
    leakage, the same reason logreg never returns per-row predictions, §7.3).
  * Deterministic — seeded k-means++ init, no wall-clock/random-device entropy —
    so a job can be reproduced for dispute re-computation (§3 / §21).

Container contract: read /data (the dataset, read-only), write /out/output.bin
(a zip of model.json + metrics.json). Paths overridable via env for testing.
Optional /params.json: { "k": 3, "max_iter": 50, "features": ["col", ...] }.
"""
import io
import json
import os
import sys
import zipfile

import numpy as np
import pandas as pd

DATA_DIR = os.environ.get("VO_DATA_DIR", "/data")
OUT_DIR = os.environ.get("VO_OUT_DIR", "/out")
PARAMS_FILE = os.environ.get("VO_PARAMS", "/params.json")

SEED = 0  # fixed → reproducible (no Math.random / wall clock)


def log(stage, **kw):
    """Structured progress log — counts/metrics only, never raw data (§7.4)."""
    print(json.dumps({"stage": stage, **kw}), flush=True)


def die(reason, code=2):
    log("error", reason=reason)
    sys.exit(code)


def load_params():
    if os.path.exists(PARAMS_FILE):
        try:
            with open(PARAMS_FILE) as f:
                return json.load(f) or {}
        except (OSError, ValueError):
            return {}
    return {}


def find_input():
    if not os.path.isdir(DATA_DIR):
        die("no_data_dir")
    names = sorted(os.listdir(DATA_DIR))
    for n in names:
        if n.lower().endswith((".csv", ".tsv")):
            return os.path.join(DATA_DIR, n)
    if names:
        return os.path.join(DATA_DIR, names[0])
    die("no_input_file")


def kmeans(x, k, max_iter):
    """Deterministic k-means with seeded k-means++ init. Returns centroids,
    labels (used only to compute aggregates here), and inertia."""
    rng = np.random.default_rng(SEED)
    n = x.shape[0]
    # k-means++ seeding (deterministic via the seeded rng).
    first = int(rng.integers(n))
    centers = [x[first]]
    for _ in range(1, k):
        d2 = np.min(
            np.stack([np.sum((x - c) ** 2, axis=1) for c in centers], axis=0),
            axis=0,
        )
        total = float(d2.sum())
        if total <= 0:
            centers.append(x[int(rng.integers(n))])
            continue
        centers.append(x[int(rng.choice(n, p=d2 / total))])
    c = np.asarray(centers, dtype=float)

    labels = np.zeros(n, dtype=int)
    for _ in range(max_iter):
        dists = ((x[:, None, :] - c[None, :, :]) ** 2).sum(axis=2)
        labels = dists.argmin(axis=1)
        new_c = np.array(
            [x[labels == j].mean(axis=0) if np.any(labels == j) else c[j] for j in range(k)]
        )
        if np.allclose(new_c, c):
            c = new_c
            break
        c = new_c
    inertia = float(((x - c[labels]) ** 2).sum())
    return c, labels, inertia


def write_output(model, metrics):
    os.makedirs(OUT_DIR, exist_ok=True)
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("model.json", json.dumps(model))
        z.writestr("metrics.json", json.dumps(metrics))
    with open(os.path.join(OUT_DIR, "output.bin"), "wb") as f:
        f.write(buf.getvalue())


def main():
    params = load_params()
    inp = find_input()
    sep = "\t" if inp.lower().endswith(".tsv") else ","
    try:
        df = pd.read_csv(inp, sep=sep)
    except Exception:  # noqa: BLE001 — any parse failure is just a bad input
        die("unreadable_input")
    log("loaded", rows=int(df.shape[0]), cols=int(df.shape[1]))

    # Feature selection: explicit list (if valid) else all numeric columns.
    want = params.get("features")
    feats = df.select_dtypes(include=[np.number])
    if isinstance(want, list) and want:
        cols = [c for c in want if c in feats.columns]
        if cols:
            feats = feats[cols]
    if feats.shape[1] == 0:
        die("no_numeric_features")

    feats = feats[feats.notna().all(axis=1)]
    n = int(feats.shape[0])
    if n < 2:
        die("too_few_complete_rows")

    k = int(params.get("k", 3))
    k = max(1, min(k, n))  # clamp: can't have more clusters than points
    max_iter = int(params.get("max_iter", 50))
    max_iter = max(1, min(max_iter, 500))

    mu = feats.mean().to_numpy()
    sd = feats.std(ddof=0).replace(0, 1).to_numpy()
    xs = (feats.to_numpy() - mu) / sd

    centroids, labels, inertia = kmeans(xs, k, max_iter)
    sizes = [int(np.sum(labels == j)) for j in range(k)]
    log("clustered", k=k, inertia=round(inertia, 4), cluster_sizes=sizes)

    # De-standardise centroids back to original feature units for interpretability.
    centroids_orig = centroids * sd + mu

    model = {
        "format": "vo-kmeans-1",
        "features": list(feats.columns),
        "k": k,
        "centroids": [[float(v) for v in row] for row in centroids_orig],
        "centroids_standardized": [[float(v) for v in row] for row in centroids],
        "mean": [float(v) for v in mu],
        "std": [float(v) for v in sd],
    }
    metrics = {
        "k": k,
        "n_samples": n,
        "n_features": int(feats.shape[1]),
        "inertia": round(inertia, 4),
        "cluster_sizes": sizes,
        # Aggregate spread only — no per-row labels (would be per-row leakage).
        "largest_cluster_fraction": round(max(sizes) / n, 4),
    }
    write_output(model, metrics)
    log("done", k=k, inertia=round(inertia, 4))


if __name__ == "__main__":
    main()
