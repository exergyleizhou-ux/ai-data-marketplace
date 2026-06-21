#!/usr/bin/env python3
"""Differentially-private descriptive statistics for the Verdant Oasis sandbox.

A TRUSTED whitelist aggregate algorithm (design §8, §20). It returns ONLY noised
aggregates — a DP count and a DP mean per numeric column — never raw rows. The
privacy budget epsilon is injected by the PLATFORM (from the dataset's offer),
NOT chosen by the buyer, so the buyer cannot turn the noise off.

Design notes (auditable, §7.3/§7.4/§8):
  * Reads ONLY /data, writes ONLY /out/output.bin; structured logs only.
  * Laplace mechanism per query; the total epsilon is split evenly across the
    queries (sequential composition — conservative, §8).
  * Sensitivity is bounded by CLAMPING each column to public bounds. If bounds
    aren't supplied they fall back to the observed min/max, which is
    data-dependent and therefore NOT a formal DP guarantee — this is reported
    honestly in the output (bounds_source).
  * NOT seeded: DP requires fresh randomness each run (a fixed seed would let an
    attacker average the noise away). So results differ run-to-run by design.
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


def laplace(rng, scale):
    return float(rng.laplace(0.0, scale)) if scale > 0 else 0.0


def sum_sensitivity(lo, hi):
    """L1 sensitivity of a sum over values clamped to [lo, hi] under the
    add/remove-one neighbor model — the SAME model the count uses (sensitivity 1).
    Adding or removing one clamped record changes the sum by at most max(|lo|,|hi|).

    (hi-lo is the *bounded-DP* change-one-value sensitivity, a different and
    inconsistent neighbor relation; it under-noises whenever the bounds do not
    straddle 0 — e.g. [10,20] would be 2x under-noised — silently breaking the
    claimed epsilon. So we always use max(|lo|,|hi|).)"""
    return max(abs(lo), abs(hi))


def main():
    params = load_params()
    # Epsilon is platform-injected (offer.dp_epsilon -> job.dp_epsilon -> _epsilon).
    eps = float(params.get("_epsilon") or 0.0)
    if eps <= 0:
        die("missing_or_invalid_epsilon")  # DP requires a positive budget

    inp = find_input()
    sep = "\t" if inp.lower().endswith(".tsv") else ","
    try:
        df = pd.read_csv(inp, sep=sep)
    except Exception:  # noqa: BLE001
        die("unreadable_input")

    requested = params.get("columns")
    numeric = df.select_dtypes(include=[np.number])
    if requested:
        numeric = numeric[[c for c in requested if c in numeric.columns]]
    cols = list(numeric.columns)
    log("loaded", rows=int(df.shape[0]), numeric_cols=len(cols))

    bounds = params.get("bounds") or {}
    rng = np.random.default_rng()  # fresh entropy — DP must not be reproducible

    # Queries: 1 count + 1 mean per column. Split epsilon (sequential composition).
    k = 1 + len(cols)
    eps_each = eps / k

    n = int(df.shape[0])
    count_dp = n + laplace(rng, 1.0 / eps_each)  # count sensitivity = 1

    per_column = {}
    for c in cols:
        series = pd.to_numeric(numeric[c], errors="coerce").dropna()
        if series.empty:
            continue
        supplied = c in bounds and isinstance(bounds[c], (list, tuple)) and len(bounds[c]) == 2
        if supplied:
            lo, hi = float(bounds[c][0]), float(bounds[c][1])
            src = "supplied"
        else:
            lo, hi = float(series.min()), float(series.max())
            src = "observed (data-dependent — not a formal DP guarantee; raw bounds withheld)"
        clamped = series.clip(lo, hi)
        # Sum sensitivity under add/remove-one (consistent with the count) = max(|lo|,|hi|).
        noisy_sum = float(clamped.sum()) + laplace(rng, sum_sensitivity(lo, hi) / eps_each)
        denom = max(round(count_dp), 1)
        col_out = {
            "mean_dp": round(noisy_sum / denom, 6),
            "bounds_source": src,
        }
        if supplied:
            # Public, caller-supplied bounds are safe to echo. Observed bounds are the
            # raw min/max of private data and are NEVER emitted (zero-noise leak).
            col_out["bounds"] = [lo, hi]
        per_column[c] = col_out

    metrics = {
        "format": "vo-dp-stats-1",
        "epsilon_total": eps,
        "epsilon_per_query": round(eps_each, 6),
        "mechanism": "laplace",
        "composition": "sequential (epsilon split evenly across count + per-column means)",
        "count_dp": round(count_dp, 3),
        "columns": per_column,
        "note": "All values are differentially-private estimates (Laplace noise); they will differ run-to-run by design.",
    }

    os.makedirs(OUT_DIR, exist_ok=True)
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("metrics.json", json.dumps(metrics))
    with open(os.path.join(OUT_DIR, "output.bin"), "wb") as f:
        f.write(buf.getvalue())
    log("done", epsilon=eps, columns=len(per_column))


if __name__ == "__main__":
    main()
