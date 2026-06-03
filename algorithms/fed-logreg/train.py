#!/usr/bin/env python3
"""Federated logistic-regression LOCAL trainer for the Verdant Oasis C2D sandbox.

One federated job fans out N of these, one per dataset. Each instance trains a
local logistic regression INSIDE its own `--network none` sandbox on that
seller's data, and emits ONLY the local model parameters (`fedparams-v1`). The
platform never sees raw data; it later aggregates the N parties' params with
FedAvg (weighted by sample count) into a joint model. Raw data never leaves the
sandbox (design P4 §2.1).

This is a TRUSTED whitelist algorithm (design §2 / §7.3): on an L1/federated
offer the audited code — not the sandbox — keeps raw data from leaking, so this
script is deliberately small, dependency-light, and easy to audit.

Security properties (read the code, then enforced by the sandbox):
  * Reads ONLY /data (read-only); writes ONLY /out. No network use.
  * Emits ONLY structured progress logs (JSON, counts/metrics) — never raw rows.
  * Output is the LOCAL PARAMS as JSON (no pickle) — never per-row predictions.

Federated contract / output:
  /out/output.bin = RAW JSON (not a zip), so the platform reads it directly as a
  federated partial:
    {"_format":"fedparams-v1","features":[...],"weights":[f64...],
     "intercept":f64,"n":int}
  The aggregator weights by `n` and averages weights+intercept across parties.

FL precondition: all parties must share the SAME feature schema and order. Pass
`params.features` (explicit ordered list) to guarantee alignment; otherwise the
numeric non-target columns sorted by name are used. There is NO per-party
standardization (it would make weights incomparable across parties) — training
is on raw features. Cross-party standardization is a later slice.

Container contract: read /data (read-only), write /out/output.bin. Paths are
overridable via env for local testing.
"""
import json
import os
import sys

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
        return pd.Series(y).map(mapping).fillna(0).astype(int).to_numpy()
    thr = float(np.median(np.asarray(y, dtype=float)))
    return (np.asarray(y, dtype=float) > thr).astype(int)


def sigmoid(z):
    return 1.0 / (1.0 + np.exp(-np.clip(z, -30, 30)))


def train_logreg(x, y, epochs=400, lr=0.1, l2=1e-3):
    """Batch gradient descent on RAW features. Deterministic (zero init, no
    shuffle) so parties' params are comparable and jobs are reproducible."""
    n, d = x.shape
    w = np.zeros(d)
    b = 0.0
    for _ in range(epochs):
        p = sigmoid(x @ w + b)
        err = p - y
        w -= lr * (x.T @ err / n + l2 * w)
        b -= lr * float(np.mean(err))
    return w, b


def select_features(df, target, params):
    """Resolve the ordered feature list. An explicit params.features guarantees
    cross-party alignment; otherwise numeric non-target columns sorted by name."""
    requested = params.get("features")
    if requested:
        cols = [c for c in requested if c in df.columns and c != target]
        if not cols:
            die("requested_features_absent")
        feats = df[cols].select_dtypes(include=[np.number])
        if feats.shape[1] != len(cols):
            die("requested_features_not_numeric")
        return feats
    feats = df.drop(columns=[target]).select_dtypes(include=[np.number])
    return feats.reindex(sorted(feats.columns), axis=1)


def write_output(partial):
    os.makedirs(OUT_DIR, exist_ok=True)
    with open(os.path.join(OUT_DIR, "output.bin"), "wb") as f:
        f.write(json.dumps(partial).encode("utf-8"))


def compute_partial(df, params):
    target = params.get("target") or df.columns[-1]
    if target not in df.columns:
        die("target_not_found")
    feats = select_features(df, target, params)
    if feats.shape[1] == 0:
        die("no_numeric_features")
    keep = feats.notna().all(axis=1) & df[target].notna()
    feats = feats[keep]
    y = df[target][keep]
    if feats.shape[0] < 4:
        die("too_few_complete_rows")
    yb = binarize(y.to_numpy())
    x = feats.to_numpy(dtype=float)
    w, b = train_logreg(x, yb)
    return {
        "_format": "fedparams-v1",
        "features": list(feats.columns),
        "weights": [float(v) for v in w],
        "intercept": float(b),
        "n": int(feats.shape[0]),
    }


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
    partial = compute_partial(df, params)
    write_output(partial)
    log("done", n=partial["n"], n_features=len(partial["features"]))


if __name__ == "__main__":
    main()
