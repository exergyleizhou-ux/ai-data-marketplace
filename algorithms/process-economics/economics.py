#!/usr/bin/env python3
"""Process economics — a Verdant Oasis Compute-to-Data algorithm.

For a dataset of production batches (each row = one batch with a product mass and
its cost components), computes batch economics — revenue, cost, margin, unit cost
— INSIDE the sandbox, returning only the aggregate indicators (never the per-batch
figures). A buyer gets the economics of someone's operation without seeing the
confidential per-batch cost data.

The C2D-portable, dataset-analyzer reframing of bos-platform's TEA engine: instead
of a single project NPV, it aggregates per-batch profitability across a run log.
numpy/pandas only.

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
    return {"mean": round(float(np.mean(a)), 4), "std": round(float(np.std(a)), 4),
            "total": round(float(np.sum(a)), 4)}


def compute(df, params):
    nums = df.select_dtypes(include=[np.number]).columns.tolist()
    product = params.get("product")
    price = params.get("price")
    costs = list(params.get("costs") or [])
    if not product:
        if not nums:
            die("no_numeric_columns")
        product = nums[0]
        costs = costs or [c for c in nums[1:] if c != product]
    if product not in df.columns:
        die(f"missing_column:{product}")
    if not costs:
        die("need_cost_columns")
    for c in costs:
        if c not in df.columns:
            die(f"missing_column:{c}")
    if price is None:
        die("need_price_param")
    price = float(price)

    d = df[[product] + costs].dropna().astype(float)
    d = d[d[product] > 0]
    if len(d) < 5:
        die("too_few_valid_batches")

    prod = d[product].to_numpy(float)
    total_cost = d[costs].sum(axis=1).to_numpy(float)
    revenue = prod * price
    margin = revenue - total_cost
    unit_cost = total_cost / prod
    profitable = float(np.mean(margin > 0))
    cost_breakdown = {c: round(float(d[c].sum()), 4) for c in costs}

    model = {
        "format": "process-economics-1",
        "design": {"product": product, "price_per_unit": price, "costs": costs,
                   "method": "per-batch revenue/cost/margin aggregated across the run log"},
        "revenue": _stats(revenue),
        "cost": _stats(total_cost),
        "margin": _stats(margin),
        "unit_cost": {"mean": round(float(unit_cost.mean()), 4), "std": round(float(unit_cost.std()), 4)},
        "profitable_batch_fraction": round(profitable, 4),
        "cost_breakdown_total": cost_breakdown,
    }
    metrics = {"n_batches": int(len(d)), "n_cost_components": len(costs)}
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
    log("done", margin_mean=model["margin"]["mean"], profitable=model["profitable_batch_fraction"], **metrics)


if __name__ == "__main__":
    main()
