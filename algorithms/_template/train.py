#!/usr/bin/env python3
"""TEMPLATE — a Verdant Oasis Compute-to-Data sandbox algorithm.

Copy this directory, rename it, and replace the `compute()` body with your own
analysis. As shipped it computes safe per-column summary statistics, so it runs
end-to-end out of the box — a working starting point, not pseudo-code.

It honors the container contract and the L1 security posture (design §2 / §7.3).
KEEP THESE PROPERTIES or your algorithm will fail review:

  * Read ONLY from /data; write ONLY to /out. No network (the sandbox enforces
    --network=none, but don't rely on it — audited code is the real boundary).
  * Log ONLY structured progress (counts/metrics). NEVER print raw rows/values —
    stdout must not become an exfiltration channel (§7.4).
  * Output a JSON bundle (zip of model.json + metrics.json), NOT a pickle, so the
    buyer can never be hit by a deserialization-RCE payload (§7.4).
  * Return AGGREGATES ONLY. Never per-row outputs (predictions, labels,
    embeddings, nearest-neighbours) — those are high-fidelity leakage (§7.3).
  * Be DETERMINISTIC (no wall clock, no random device) so a job can be reproduced
    for dispute re-computation (§3 / §21). If you need randomness, seed it.

Authoring tip: write and iterate this with Lumen (`/build` page). Then submit it
for review in the privacy-compute hub; it can only run after it's approved.
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


def compute(df, params):
    """↓↓↓ REPLACE THIS with your analysis. ↓↓↓

    Input:  df = the dataset (pandas DataFrame), params = optional dict.
    Return: (model: dict, metrics: dict) — both JSON-serializable AGGREGATES.

    The default: per-numeric-column summary stats (count/mean/std/min/max).
    """
    feats = df.select_dtypes(include=[np.number])
    if feats.shape[1] == 0:
        die("no_numeric_features")
    summary = {}
    for col in feats.columns:
        s = feats[col].dropna()
        if s.empty:
            continue
        summary[str(col)] = {
            "count": int(s.shape[0]),
            "mean": round(float(s.mean()), 6),
            "std": round(float(s.std(ddof=0)), 6),
            "min": round(float(s.min()), 6),
            "max": round(float(s.max()), 6),
        }
    model = {"format": "vo-summary-1", "columns": summary}
    metrics = {"n_rows": int(df.shape[0]), "n_numeric_cols": int(feats.shape[1])}
    return model, metrics


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
    if df.shape[0] < 1:
        die("empty_input")

    model, metrics = compute(df, params)
    write_output(model, metrics)
    log("done", **{k: v for k, v in metrics.items() if isinstance(v, (int, float))})


if __name__ == "__main__":
    main()
