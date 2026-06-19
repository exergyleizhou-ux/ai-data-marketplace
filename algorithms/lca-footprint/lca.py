#!/usr/bin/env python3
"""LCA footprint — a Verdant Oasis Compute-to-Data algorithm.

For a dataset of process runs (each row = one run with activity quantities such as
electricity, transport, substrate), computes the **greenhouse-gas footprint** (GWP
in kg CO2e) via emission factors — INSIDE the sandbox, returning only the
aggregate footprint (never the per-run activity data). A buyer gets the carbon
footprint of an operation without seeing the confidential per-run energy/logistics.

The C2D-portable, dataset-analyzer reframing of bos-platform's LCA engine
(GWP = sum of activity * emission-factor). numpy/pandas only. Emission factors are
public constants supplied via params (or matched against built-in defaults).

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

# Public default GWP emission factors (kg CO2e per unit), matched by column name.
DEFAULT_FACTORS = {
    "electricity_kwh": 0.5, "energy_kwh": 0.5, "heat_kwh": 0.27,
    "transport_km": 0.12, "transport_tkm": 0.12, "diesel_l": 2.68,
    "natural_gas_m3": 2.0, "substrate_kg": 0.1, "water_m3": 0.34,
}


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
    nums = df.select_dtypes(include=[np.number]).columns.tolist()
    product = params.get("product")
    activities = dict(params.get("activities") or {})
    if not activities:
        # match numeric columns against the built-in default factors
        activities = {c: DEFAULT_FACTORS[c] for c in nums if c in DEFAULT_FACTORS}
    if not activities:
        die("no_activities (supply params.activities = {column: emission_factor})")
    for c in activities:
        if c not in df.columns:
            die(f"missing_column:{c}")
    cols = list(activities.keys())
    use = cols + ([product] if product and product in df.columns else [])
    d = df[use].dropna().astype(float)
    if len(d) < 5:
        die("too_few_valid_runs")

    contributions = {c: float((d[c].to_numpy(float) * activities[c]).sum()) for c in cols}
    per_run = np.zeros(len(d))
    for c in cols:
        per_run = per_run + d[c].to_numpy(float) * activities[c]
    total_gwp = float(per_run.sum())

    out = {
        "format": "lca-footprint-1",
        "design": {"activities": activities, "product": product, "impact": "GWP (kg CO2e)",
                   "method": "GWP = sum(activity * emission_factor) per run, aggregated"},
        "gwp_total_kgco2e": round(total_gwp, 4),
        "gwp_per_run": {"mean": round(float(per_run.mean()), 4), "std": round(float(per_run.std()), 4)},
        "contribution_by_activity_kgco2e": {c: round(v, 4) for c, v in contributions.items()},
    }
    if product and product in d.columns:
        prod_total = float(d[product].sum())
        if prod_total > 0:
            out["gwp_per_product_unit"] = round(total_gwp / prod_total, 6)
    metrics = {"n_runs": int(len(d)), "n_activities": len(cols)}
    return out, metrics


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
    log("done", gwp_total=model["gwp_total_kgco2e"], **metrics)


if __name__ == "__main__":
    main()
