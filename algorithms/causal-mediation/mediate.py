#!/usr/bin/env python3
"""Causal mediation (Pearl NDE/NIE) — a Verdant Oasis Compute-to-Data algorithm.

Estimates the natural direct effect (NDE), natural indirect effect (NIE), total
effect (ATE) and proportion-mediated for a single-mediator linear model
(treatment T -> mediator M -> outcome Y) INSIDE the C2D sandbox, and emits ONLY
the aggregate effect estimates with bootstrap CIs — never per-row outputs. A
buyer learns the causal decomposition of someone's dataset without seeing the
data.

This is the C2D-portable core of the bos-platform causal layer: linear
two-stage-regression mediation — DoWhy's `mediation.two_stage_regression`
reduces to exactly this for the linear single-mediator case (Pearl natural
effects: NDE = beta_T, NIE = alpha_T * beta_M, with NDE + NIE == ATE) —
implemented with numpy/pandas only, so the sandbox image stays tiny and the
audited surface small (no DoWhy / EconML / PyMC).

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
SEED = 42
N_BOOT = 1000


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


def _ols_coef(X, y):
    beta, *_ = np.linalg.lstsq(X, y, rcond=None)
    return beta


def _design(df, T, M, covars, include_m):
    n = len(df)
    cols = [np.ones((n, 1)), df[[T]].to_numpy(float)]
    if include_m:
        cols.append(df[[M]].to_numpy(float))
    if covars:
        cols.append(df[covars].to_numpy(float))
    return np.hstack(cols)


def _effects(df, T, M, Y, covars):
    # M ~ 1 + T (+cov): alpha_T is coef index 1
    bm = _ols_coef(_design(df, T, M, covars, include_m=False), df[M].to_numpy(float))
    alpha_T = bm[1]
    # Y ~ 1 + T + M (+cov): beta_T index 1 (= NDE), beta_M index 2
    by = _ols_coef(_design(df, T, M, covars, include_m=True), df[Y].to_numpy(float))
    beta_T, beta_M = by[1], by[2]
    nde = float(beta_T)
    nie = float(alpha_T * beta_M)
    return nde, nie, nde + nie


def _r2(df, T, M, Y, covars):
    X = _design(df, T, M, covars, include_m=True)
    y = df[Y].to_numpy(float)
    resid = y - X @ _ols_coef(X, y)
    ss_res = float(resid @ resid)
    ss_tot = float(((y - y.mean()) ** 2).sum())
    return 1.0 - ss_res / ss_tot if ss_tot > 0 else 0.0


def _ci(a):
    if not a:
        return [None, None]
    return [round(float(np.percentile(a, 2.5)), 6), round(float(np.percentile(a, 97.5)), 6)]


def compute(df, params):
    nums = df.select_dtypes(include=[np.number]).columns.tolist()
    T = params.get("treatment")
    M = params.get("mediator")
    Y = params.get("outcome")
    covars = list(params.get("covariates") or [])
    # Fall back to the first 3 numeric columns so it runs out of the box.
    if not (T and M and Y):
        if len(nums) < 3:
            die("need_treatment_mediator_outcome")
        T, M, Y = nums[0], nums[1], nums[2]
    for c in [T, M, Y] + covars:
        if c not in df.columns:
            die(f"missing_column:{c}")
    d = df[[T, M, Y] + covars].dropna().astype(float)
    if len(d) < 10:
        die("too_few_complete_rows")

    nde, nie, ate = _effects(d, T, M, Y, covars)
    rng = np.random.default_rng(SEED)
    boots = {"nde": [], "nie": [], "ate": [], "prop": []}
    n = len(d)
    idx = np.arange(n)
    for _ in range(N_BOOT):
        s = d.iloc[rng.choice(idx, n, replace=True)]
        try:
            bn, bi, ba = _effects(s, T, M, Y, covars)
        except Exception:  # noqa: BLE001 — a singular resample just skips
            continue
        boots["nde"].append(bn)
        boots["nie"].append(bi)
        boots["ate"].append(ba)
        # Proportion-mediated is only meaningful when the total effect is bounded away
        # from 0. When NDE and NIE nearly cancel (inconsistent/suppression mediation),
        # ATE≈0 and nie/ate explodes (e.g. "548% mediated"). Gate on the total being
        # non-trivial relative to the effects it sums, not just != 0.
        if abs(ba) > 0.05 * (abs(bn) + abs(bi)):
            boots["prop"].append(bi / ba)
    prop = nie / ate if abs(ate) > 0.05 * (abs(nde) + abs(nie)) else None

    model = {
        "format": "causal-mediation-1",
        "design": {
            "treatment": T,
            "mediator": M,
            "outcome": Y,
            "covariates": covars,
            "method": "linear two-stage regression (Pearl NDE/NIE)",
        },
        "effects": {
            "nde": round(nde, 6),
            "nie": round(nie, 6),
            "ate": round(ate, 6),
            "proportion_mediated": round(prop, 6) if prop is not None else None,
            "nde_ci95": _ci(boots["nde"]),
            "nie_ci95": _ci(boots["nie"]),
            "ate_ci95": _ci(boots["ate"]),
            "proportion_mediated_ci95": _ci(boots["prop"]),
        },
        # NDE + NIE == ATE exactly in the linear model — the Pearl identity.
        "pearl_invariant_residual": round(abs(ate - (nde + nie)), 9),
    }
    metrics = {
        "n_rows": int(len(d)),
        "n_covariates": len(covars),
        "outcome_r2": round(_r2(d, T, M, Y, covars), 6),
        "n_bootstrap": len(boots["nde"]),
    }
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
    log("done", **metrics)


if __name__ == "__main__":
    main()
