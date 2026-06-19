#!/usr/bin/env python3
"""Causal effect estimation (ATE) — a Verdant Oasis Compute-to-Data algorithm.

Estimates the average treatment effect of T on Y (adjusting for covariates)
INSIDE the sandbox by two methods — linear OLS and cross-fitted double-ML
(partialling-out with a quadratic nuisance basis) — returning only the aggregate
estimates (never per-row outputs). The foundation of the causal suite that
mediation / sensitivity / refutation build on.

The C2D-portable core of bos-platform's causal estimate layer. numpy only (the
DML nuisance is plain OLS on a quadratic basis, k-fold cross-fitted; the p-value
uses an inline normal CDF via math.erf) — no DoWhy / EconML / sklearn.

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

DATA_DIR = os.environ.get("VO_DATA_DIR", "/data")
OUT_DIR = os.environ.get("VO_OUT_DIR", "/out")
PARAMS_FILE = os.environ.get("VO_PARAMS", "/params.json")
SEED = 42
N_BOOT = 400
K_FOLDS = 5


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


def _phi(z):  # standard normal CDF
    return 0.5 * (1 + math.erf(z / math.sqrt(2)))


def _ols_ate(d, T, Y, covars):
    n = len(d)
    cols = [np.ones((n, 1)), d[[T]].to_numpy(float)]
    if covars:
        cols.append(d[covars].to_numpy(float))
    X = np.hstack(cols)
    y = d[Y].to_numpy(float)
    p = X.shape[1]
    dof = n - p
    if dof <= 0:
        die("too_few_rows_for_params")
    XtX_inv = np.linalg.pinv(X.T @ X)
    beta = XtX_inv @ X.T @ y
    resid = y - X @ beta
    sigma2 = float(resid @ resid) / dof
    se = float(np.sqrt(sigma2 * XtX_inv[1, 1]))
    return float(beta[1]), se, int(dof)


def _quad_basis(C):
    if C.shape[1] == 0:
        return C
    return np.hstack([C, C ** 2])


def _crossfit_resid(v, B, rng):
    """Out-of-fold residuals of v regressed on [1, B] via K-fold cross-fitting."""
    n = len(v)
    if B.shape[1] == 0:
        return v - v.mean()
    Xi = np.hstack([np.ones((n, 1)), B])
    resid = np.empty(n)
    folds = np.array_split(rng.permutation(n), K_FOLDS)
    for f in folds:
        mask = np.ones(n, bool)
        mask[f] = False
        beta, *_ = np.linalg.lstsq(Xi[mask], v[mask], rcond=None)
        resid[f] = v[f] - Xi[f] @ beta
    return resid


def _dml_ate(d, T, Y, covars, rng):
    B = _quad_basis(d[covars].to_numpy(float)) if covars else np.empty((len(d), 0))
    tr = _crossfit_resid(d[T].to_numpy(float), B, rng)
    yr = _crossfit_resid(d[Y].to_numpy(float), B, rng)
    denom = float(tr @ tr)
    return float((tr @ yr) / denom) if denom > 0 else 0.0


def compute(df, params):
    nums = df.select_dtypes(include=[np.number]).columns.tolist()
    T = params.get("treatment")
    Y = params.get("outcome")
    covars = list(params.get("covariates") or [])
    if not (T and Y):
        if len(nums) < 2:
            die("need_treatment_outcome")
        T, Y = nums[0], nums[1]
        covars = [c for c in nums[2:] if c not in (T, Y)]
    for c in [T, Y] + covars:
        if c not in df.columns:
            die(f"missing_column:{c}")
    d = df[[T, Y] + covars].dropna().astype(float).reset_index(drop=True)
    if len(d) < 20:
        die("too_few_complete_rows")

    ate, se, dof = _ols_ate(d, T, Y, covars)
    t_stat = ate / se if se > 0 else 0.0
    p_value = 2 * (1 - _phi(abs(t_stat)))
    ci = [round(ate - 1.959964 * se, 6), round(ate + 1.959964 * se, 6)]

    rng = np.random.default_rng(SEED)
    ate_dml = _dml_ate(d, T, Y, covars, rng)
    n = len(d)
    boot = []
    for _ in range(N_BOOT):
        s = d.iloc[rng.choice(n, n, replace=True)].reset_index(drop=True)
        try:
            boot.append(_dml_ate(s, T, Y, covars, rng))
        except Exception:  # noqa: BLE001
            continue
    dml_ci = [round(float(np.percentile(boot, 2.5)), 6), round(float(np.percentile(boot, 97.5)), 6)] if boot else [None, None]

    nuniq = int(d[T].nunique())
    if nuniq == 2:
        vals = sorted(d[T].unique())
        g1 = d[d[T] == vals[1]][Y]
        g0 = d[d[T] == vals[0]][Y]
        treat = {"treatment_type": "binary", "n_treated": int(len(g1)), "n_control": int(len(g0)),
                 "unadjusted_diff": round(float(g1.mean() - g0.mean()), 6)}
    else:
        treat = {"treatment_type": "continuous"}

    model = {
        "format": "causal-estimate-1",
        "design": {"treatment": T, "outcome": Y, "covariates": covars,
                   "method": "OLS adjustment + cross-fitted DML (partialling-out)"},
        "estimate": {
            "ate_ols": round(ate, 6), "se": round(se, 6), "t_stat": round(t_stat, 6),
            "p_value": round(p_value, 6), "ci95": ci, "dof": dof,
            "ate_dml": round(ate_dml, 6), "ate_dml_ci95": dml_ci,
        },
        "treatment": treat,
    }
    metrics = {"n_rows": int(n), "n_covariates": len(covars), "n_bootstrap": len(boot)}
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
    log("done", ate_ols=model["estimate"]["ate_ols"], ate_dml=model["estimate"]["ate_dml"], **metrics)


if __name__ == "__main__":
    main()
