#!/usr/bin/env python3
"""Bioprocess growth kinetics — a Verdant Oasis Compute-to-Data algorithm.

Fits microbial / larval growth-curve models (Logistic and modified Gompertz) to a
time-series of biomass measurements INSIDE the sandbox, returning only the
aggregate fitted parameters + goodness-of-fit (never the per-row curve). A buyer
gets the growth kinetics of someone's fermentation / bioconversion run without
seeing the raw measurements.

The C2D-portable core of bos-platform's `kinetics_engine` (which models insect /
microbial growth via Monod / Logistic / Gompertz / Baranyi-Roberts). This port
keeps the two closed-form sigmoids — bos's exact Logistic
`B(t)=K/(1+((K-B0)/B0)e^{-rt})` and the Zwietering modified Gompertz — fit with
scipy least-squares, so no ODE solver and a small audited surface.

Contract (design §7.3, L1): read ONLY /data + /params.json, write ONLY
/out/output.bin (zip of model.json + metrics.json); AGGREGATES ONLY; deterministic.
"""
import io
import json
import math
import os
import sys
import zipfile

import numpy as np
import pandas as pd
from scipy.optimize import curve_fit

DATA_DIR = os.environ.get("VO_DATA_DIR", "/data")
OUT_DIR = os.environ.get("VO_OUT_DIR", "/out")
PARAMS_FILE = os.environ.get("VO_PARAMS", "/params.json")
_E = math.e


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


def logistic(t, B0, K, r):
    # bos kinetics_engine.logistic_growth — density-dependent growth, carrying cap K.
    return K / (1 + ((K - B0) / B0) * np.exp(-r * t))


def gompertz(t, A, mu_m, lam):
    # Zwietering modified Gompertz: A = asymptote, mu_m = max growth rate, lam = lag.
    inner = (mu_m * _E / A) * (lam - t) + 1
    return A * np.exp(-np.exp(np.clip(inner, -700, 700)))


def _fit(name, fn, t, y, p0, bounds):
    try:
        popt, _ = curve_fit(fn, t, y, p0=p0, bounds=bounds, maxfev=20000)
    except Exception as e:  # noqa: BLE001 — a non-converging fit is reported, not fatal
        return {"model": name, "converged": False, "reason": str(e)[:120]}
    pred = fn(t, *popt)
    resid = y - pred
    ss_res = float(resid @ resid)
    ss_tot = float(((y - y.mean()) ** 2).sum())
    r2 = 1.0 - ss_res / ss_tot if ss_tot > 0 else 0.0
    rmse = float(np.sqrt(ss_res / len(y)))
    return {"model": name, "converged": True, "params": [round(float(p), 6) for p in popt], "r2": round(r2, 6), "rmse": round(rmse, 6)}


def compute(df, params):
    nums = df.select_dtypes(include=[np.number]).columns.tolist()
    tcol = params.get("time")
    vcol = params.get("value")
    if not (tcol and vcol):
        if len(nums) < 2:
            die("need_time_and_value_columns")
        tcol, vcol = nums[0], nums[1]
    for c in (tcol, vcol):
        if c not in df.columns:
            die(f"missing_column:{c}")
    d = df[[tcol, vcol]].dropna().astype(float).sort_values(tcol)
    if len(d) < 5:
        die("too_few_points")
    t = d[tcol].to_numpy(float)
    y = d[vcol].to_numpy(float)
    if (y <= 0).any():
        die("non_positive_biomass")  # B0 in the denominator / asymptote ratios

    y0, ymax, tmax = float(y[0]), float(y.max()), float(t.max())
    fits = []
    fits.append(_fit("logistic", logistic, t, y,
                     p0=[max(y0, 1e-6), ymax, 0.5],
                     bounds=([1e-9, ymax * 0.5, 1e-6], [ymax, ymax * 10, 100])))
    fits.append(_fit("gompertz", gompertz, t, y,
                     p0=[ymax, 0.5, t.min()],
                     bounds=([ymax * 0.5, 1e-6, -abs(tmax)], [ymax * 10, 100, tmax])))

    converged = [f for f in fits if f.get("converged")]
    best = max(converged, key=lambda f: f["r2"]) if converged else None

    derived = {}
    if best and best["model"] == "logistic":
        B0, K, r = best["params"]
        derived = {
            "carrying_capacity": round(K, 6),
            "growth_rate_r": round(r, 6),
            "max_dB_dt": round(r * K / 4, 6),         # max slope at B=K/2
            "doubling_time": round(math.log(2) / r, 6) if r > 0 else None,
        }
    elif best and best["model"] == "gompertz":
        A, mu_m, lam = best["params"]
        derived = {
            "asymptote": round(A, 6),
            "max_specific_growth_rate": round(mu_m, 6),
            "lag_time": round(lam, 6),
        }

    model = {
        "format": "bioprocess-kinetics-1",
        "design": {"time": tcol, "value": vcol, "models": ["logistic", "gompertz"],
                   "method": "closed-form growth-curve least squares (bos kinetics_engine port)"},
        "fits": fits,
        "best_model": best["model"] if best else None,
        "best_r2": best["r2"] if best else None,
        "derived": derived,
    }
    metrics = {"n_points": int(len(d)), "t_min": round(float(t.min()), 6), "t_max": round(tmax, 6),
               "value_min": round(y0, 6), "value_max": round(ymax, 6)}
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
    log("done", best=model["best_model"], best_r2=model["best_r2"], **metrics)


if __name__ == "__main__":
    main()
