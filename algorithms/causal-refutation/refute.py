#!/usr/bin/env python3
"""Causal refutation — a Verdant Oasis Compute-to-Data algorithm.

Stress-tests a linear treatment effect with three standard refuters — placebo
treatment, random common cause, and data subset — INSIDE the sandbox, returning
only the aggregate refutation verdict (never per-row outputs). A buyer learns
whether an effect survives validity checks on someone's dataset without seeing it.

The C2D-portable core of bos-platform's `causal_refute_engine` (which wraps
DoWhy's mandatory refuters: random_common_cause / placebo_treatment /
data_subset). bos calls a finding "validated" when the refuters all pass; this
port reproduces the same three checks with numpy OLS — no DoWhy:

  * placebo treatment   — permute T; the re-estimated effect must collapse to ~0
  * random common cause — add a random covariate; the effect must stay stable
  * data subset         — re-estimate on random subsets; the effect must stay stable

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
R_PLACEBO = 200
R_RCC = 50
R_SUBSET = 100


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


def _effect(df, T, Y, covars):
    """OLS coefficient of T in Y ~ 1 + T + covars."""
    n = len(df)
    cols = [np.ones((n, 1)), df[[T]].to_numpy(float)]
    if covars:
        cols.append(df[covars].to_numpy(float))
    X = np.hstack(cols)
    beta, *_ = np.linalg.lstsq(X, df[Y].to_numpy(float), rcond=None)
    return float(beta[1])


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

    orig = _effect(d, T, Y, covars)
    rng = np.random.default_rng(SEED)
    n = len(d)
    absorig = abs(orig) if orig != 0 else 1e-12

    # 1. placebo treatment — permute T; effect should vanish.
    tvals = d[T].to_numpy(float)
    plac = np.empty(R_PLACEBO)
    for i in range(R_PLACEBO):
        dd = d.copy()
        dd[T] = rng.permutation(tvals)
        plac[i] = _effect(dd, T, Y, covars)
    placebo_mean = float(plac.mean())
    perm_p = float((np.abs(plac) >= abs(orig)).mean())  # permutation p-value
    placebo_pass = bool(abs(placebo_mean) < 0.1 * absorig)

    # 2. random common cause — add a random covariate; effect should be stable.
    rcc = np.empty(R_RCC)
    for i in range(R_RCC):
        dd = d.copy()
        dd["__rcc__"] = rng.normal(0, 1, n)
        rcc[i] = _effect(dd, T, Y, covars + ["__rcc__"])
    rcc_mean = float(rcc.mean())
    rcc_change = abs(rcc_mean - orig) / absorig
    rcc_pass = bool(rcc_change < 0.1)

    # 3. data subset — re-estimate on random 80% subsets; effect should be stable.
    k = max(int(0.8 * n), 10)
    sub = np.empty(R_SUBSET)
    for i in range(R_SUBSET):
        idx = rng.choice(n, k, replace=False)
        sub[i] = _effect(d.iloc[idx], T, Y, covars)
    sub_mean = float(sub.mean())
    sub_std = float(sub.std())
    sub_change = abs(sub_mean - orig) / absorig
    sub_pass = bool(sub_change < 0.15)

    evidence = "validated" if (placebo_pass and rcc_pass and sub_pass) else "weak"

    model = {
        "format": "causal-refutation-1",
        "design": {"treatment": T, "outcome": Y, "covariates": covars,
                   "method": "DoWhy-style refuters (placebo / random-common-cause / data-subset)"},
        "original_effect": round(orig, 6),
        "refuters": [
            {"name": "placebo_treatment", "placebo_effect_mean": round(placebo_mean, 6),
             "permutation_p_value": round(perm_p, 4), "passed": placebo_pass,
             "note": "effect should vanish (~0) under a permuted treatment"},
            {"name": "random_common_cause", "new_effect_mean": round(rcc_mean, 6),
             "relative_change": round(rcc_change, 4), "passed": rcc_pass,
             "note": "effect should be stable when a random covariate is added"},
            {"name": "data_subset", "new_effect_mean": round(sub_mean, 6),
             "new_effect_std": round(sub_std, 6), "relative_change": round(sub_change, 4),
             "passed": sub_pass, "note": "effect should be stable on random subsets"},
        ],
        "evidence_level": evidence,
    }
    metrics = {"n_rows": int(n), "n_covariates": len(covars),
               "n_placebo": R_PLACEBO, "n_subset": R_SUBSET}
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
    log("done", evidence_level=model["evidence_level"], **metrics)


if __name__ == "__main__":
    main()
