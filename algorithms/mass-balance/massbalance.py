#!/usr/bin/env python3
"""Mass balance — a Verdant Oasis Compute-to-Data algorithm.

For a dataset of bioconversion / process runs (each row = one run with an input
mass and its output fractions), checks **mass closure** — how much of the input
is accounted for in the outputs — INSIDE the sandbox, returning only the
aggregate closure statistics (never the per-run values). A buyer learns whether a
process's mass balances on someone's run log without seeing the runs.

The C2D-portable core of bos-platform's mass-balance engine (`epsilon` = closure
residual). numpy/pandas only.

  closure_i  = sum(outputs_i) / input_i        # fraction of input accounted for
  residual_i = 1 - closure_i  (= epsilon)       # unaccounted fraction

Contract (design §7.3, L1): read ONLY /data + /params.json, write ONLY
/out/output.bin (zip of model.json + metrics.json); AGGREGATES ONLY; deterministic.
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


def _stats(a):
    return {"mean": round(float(np.mean(a)), 6), "std": round(float(np.std(a)), 6),
            "min": round(float(np.min(a)), 6), "max": round(float(np.max(a)), 6)}


def compute(df, params):
    nums = df.select_dtypes(include=[np.number]).columns.tolist()
    inp = params.get("input")
    outs = list(params.get("outputs") or [])
    tol = float(params.get("tolerance", 0.05))
    if not (inp and outs):
        if len(nums) < 2:
            die("need_input_and_output_columns")
        inp = nums[0]
        outs = [c for c in nums[1:] if c != inp]
    for c in [inp] + outs:
        if c not in df.columns:
            die(f"missing_column:{c}")
    d = df[[inp] + outs].dropna().astype(float)
    d = d[d[inp] > 0]  # input mass must be positive to form a ratio
    if len(d) < 5:
        die("too_few_valid_runs")

    inv = d[inp].to_numpy(float)
    out_total = d[outs].sum(axis=1).to_numpy(float)
    closure = out_total / inv
    residual = 1.0 - closure
    within = float(np.mean(np.abs(residual) <= tol))
    per_output = {c: round(float((d[c].to_numpy(float) / inv).mean()), 6) for c in outs}

    model = {
        "format": "mass-balance-1",
        "design": {"input": inp, "outputs": outs, "tolerance": tol,
                   "method": "per-run mass closure (sum outputs / input); residual = 1 - closure"},
        "closure": _stats(closure),
        "residual_epsilon": _stats(residual),
        "within_tolerance_fraction": round(within, 6),
        "mean_output_fraction": per_output,
    }
    metrics = {"n_runs": int(len(d)), "n_outputs": len(outs)}
    return model, metrics


def main():
    params = load_params()
    inp = find_input()
    sep = "\t" if inp.lower().endswith(".tsv") else ","
    try:
        df = pd.read_csv(inp, sep=sep)
    except Exception:  # noqa: BLE001
        die("unreadable_input")
    log("loaded", rows=int(df.shape[0]), cols=int(df.shape[1]))
    if df.shape[0] < 1:
        die("empty_input")

    model, metrics = compute(df, params)
    os.makedirs(OUT_DIR, exist_ok=True)
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("model.json", json.dumps(model))
        z.writestr("metrics.json", json.dumps(metrics))
    with open(os.path.join(OUT_DIR, "output.bin"), "wb") as f:
        f.write(buf.getvalue())
    log("done", closure_mean=model["closure"]["mean"], within=model["within_tolerance_fraction"], **metrics)


if __name__ == "__main__":
    main()
