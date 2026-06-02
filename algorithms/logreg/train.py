#!/usr/bin/env python3
"""Logistic-regression trainer for the Verdant Oasis Compute-to-Data sandbox.

This is a TRUSTED whitelist algorithm (design doc §2 / §7.3): on an L1 offer the
audited code — not the sandbox — is what keeps the raw data from leaking, so this
script is deliberately small, dependency-light, and easy to audit.

Security properties (enforced by reading this code, then by the sandbox):
  * Reads ONLY from the data dir; writes ONLY to the out dir. No network use.
  * Emits ONLY structured progress logs (JSON with counts/metrics) — it NEVER
    prints raw rows / values, so stdout is not an exfiltration channel (§7.4).
  * Produces a JSON model (weights + standardisation), NOT a pickle, so the
    output can never be a deserialization-RCE vector for the buyer (§7.4).
  * Returns the final model + aggregate metrics only — never per-row predictions,
    embeddings, or nearest-neighbours (which would be high-fidelity leakage, §7.3).

Container contract: read /data (the dataset, read-only), write /out/output.bin
(a zip of model.json + metrics.json). Paths are overridable via env for testing.
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


def binarize(y):
    """Map the target to {0,1}. Two-class -> direct map; many-valued numeric ->
    threshold at the median (deterministic, reproducible)."""
    uniq = pd.unique(pd.Series(y).dropna())
    if len(uniq) <= 2:
        classes = sorted(uniq.tolist(), key=lambda v: str(v))
        mapping = {classes[0]: 0}
        if len(classes) > 1:
            mapping[classes[1]] = 1
        yb = pd.Series(y).map(mapping).fillna(0).astype(int).to_numpy()
        return yb, [str(c) for c in classes]
    thr = float(np.median(np.asarray(y, dtype=float)))
    yb = (np.asarray(y, dtype=float) > thr).astype(int)
    return yb, [f"<= {thr:g}", f"> {thr:g}"]


def sigmoid(z):
    return 1.0 / (1.0 + np.exp(-np.clip(z, -30, 30)))


def train_logreg(x, y, epochs=400, lr=0.1, l2=1e-3):
    """Batch gradient descent. Deterministic (zero init, no shuffling) so a job
    can be reproduced for dispute re-computation (design §3/§21)."""
    n, d = x.shape
    w = np.zeros(d)
    b = 0.0
    for _ in range(epochs):
        p = sigmoid(x @ w + b)
        err = p - y
        w -= lr * (x.T @ err / n + l2 * w)
        b -= lr * float(np.mean(err))
    return w, b


def accuracy(x, y, w, b):
    pred = (sigmoid(x @ w + b) >= 0.5).astype(int)
    return float(np.mean(pred == y))


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
    if df.shape[0] < 4:
        die("too_few_rows")

    target = params.get("target") or df.columns[-1]
    if target not in df.columns:
        die("target_not_found")

    y_series = df[target]
    feats = df.drop(columns=[target]).select_dtypes(include=[np.number])
    if feats.shape[1] == 0:
        die("no_numeric_features")

    keep = feats.notna().all(axis=1) & y_series.notna()
    feats = feats[keep]
    y_series = y_series[keep]
    if feats.shape[0] < 4:
        die("too_few_complete_rows")

    yb, classes = binarize(y_series.to_numpy())
    mu = feats.mean().to_numpy()
    sd = feats.std(ddof=0).replace(0, 1).to_numpy()
    xs = (feats.to_numpy() - mu) / sd

    # Deterministic 80/20 holdout (no shuffle -> reproducible).
    n = xs.shape[0]
    cut = max(int(n * 0.8), 1)
    if cut >= n:
        cut = n - 1
    xtr, ytr = xs[:cut], yb[:cut]
    xte, yte = xs[cut:], yb[cut:]

    w, b = train_logreg(xtr, ytr)
    train_acc = accuracy(xtr, ytr, w, b)
    holdout_acc = accuracy(xte, yte, w, b) if len(yte) else train_acc
    log("trained", train_accuracy=round(train_acc, 4), holdout_accuracy=round(holdout_acc, 4))

    model = {
        "format": "vo-logreg-1",
        "features": list(feats.columns),
        "weights": [float(v) for v in w],
        "bias": float(b),
        "mean": [float(v) for v in mu],
        "std": [float(v) for v in sd],
        "classes": classes,
        "target": str(target),
    }
    metrics = {
        "train_accuracy": round(train_acc, 4),
        "holdout_accuracy": round(holdout_acc, 4),
        "n_samples": int(n),
        "n_features": int(feats.shape[1]),
        "positive_rate": round(float(np.mean(yb)), 4),
    }
    write_output(model, metrics)
    log("done", holdout_accuracy=round(holdout_acc, 4))


if __name__ == "__main__":
    main()
