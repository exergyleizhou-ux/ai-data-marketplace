#!/usr/bin/env python3
"""Causal sensitivity (Cinelli-Hazlett robustness value) — a Verdant Oasis
Compute-to-Data algorithm.

For a linear treatment effect (Y ~ T + covariates), quantifies how strong an
UNOBSERVED confounder would have to be to overturn the estimate — INSIDE the
sandbox, emitting only the aggregate sensitivity statistics (no per-row outputs).
A buyer learns how robust a finding is on someone's dataset without seeing it.

This is the C2D-portable core of bos-platform's `"linear"` sensitivity branch:
the closed-form Cinelli & Hazlett (2020) robustness value + partial R^2 from a
single OLS fit (no DoWhy / statsmodels — the partial R^2 and RV are functions of
the treatment coefficient's t-statistic and the residual degrees of freedom):

  partial_r2_yd = t^2 / (t^2 + dof)             # treatment<->outcome partial R^2
  f            = |t| / sqrt(dof)                # partial Cohen's f
  RV_q         = 0.5 * (sqrt(f_q^4 + 4 f_q^2) - f_q^2),  f_q = q * f
  RV_qa        = same with f_qa = (q|t| - z_alpha) / sqrt(dof), clamped at 0

RV_q is the partial R^2 a confounder must share with BOTH treatment and outcome
to reduce the estimate by 100*q percent (q=1 -> to zero). Conventionally "robust"
when RV_q > 0.10 (Cinelli-Hazlett 2020).

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
RV_ROBUST_THRESHOLD = 0.10  # Cinelli-Hazlett 2020 conventional "highly robust"


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


def _norm_ppf(p):
    """Inverse standard-normal CDF (Acklam's approximation, |err| < 1.2e-9).

    Used for the large-sample critical value of the RV confidence bound, so we
    need no scipy — keeping the audited image tiny and consistent with the other
    causal C2D algorithms.
    """
    a = [-3.969683028665376e1, 2.209460984245205e2, -2.759285104469687e2,
         1.383577518672690e2, -3.066479806614716e1, 2.506628277459239]
    b = [-5.447609879822406e1, 1.615858368580409e2, -1.556989798598866e2,
         6.680131188771972e1, -1.328068155288572e1]
    c = [-7.784894002430293e-3, -3.223964580411365e-1, -2.400758277161838,
         -2.549732539343734, 4.374664141464968, 2.938163982698783]
    d = [7.784695709041462e-3, 3.224671290700398e-1, 2.445134137142996,
         3.754408661907416]
    plow, phigh = 0.02425, 1 - 0.02425
    if p < plow:
        q = math.sqrt(-2 * math.log(p))
        return (((((c[0] * q + c[1]) * q + c[2]) * q + c[3]) * q + c[4]) * q + c[5]) / \
               ((((d[0] * q + d[1]) * q + d[2]) * q + d[3]) * q + 1)
    if p > phigh:
        q = math.sqrt(-2 * math.log(1 - p))
        return -(((((c[0] * q + c[1]) * q + c[2]) * q + c[3]) * q + c[4]) * q + c[5]) / \
               ((((d[0] * q + d[1]) * q + d[2]) * q + d[3]) * q + 1)
    q = p - 0.5
    r = q * q
    return (((((a[0] * r + a[1]) * r + a[2]) * r + a[3]) * r + a[4]) * r + a[5]) * q / \
           (((((b[0] * r + b[1]) * r + b[2]) * r + b[3]) * r + b[4]) * r + 1)


def _ols_treatment_t(d, T, Y, covars):
    """OLS Y ~ 1 + T + covars; return (coef_T, se_T, t_T, dof)."""
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
    # Refuse a rank-deficient design (perfect collinearity, incl. a constant
    # treatment collinear with the intercept): pinv would silently split the
    # coefficient and report a confident-but-wrong robustness value.
    if np.linalg.matrix_rank(X) < p:
        die("rank_deficient_design")
    XtX_inv = np.linalg.pinv(X.T @ X)
    beta = XtX_inv @ X.T @ y
    resid = y - X @ beta
    sigma2 = float(resid @ resid) / dof
    cov = sigma2 * XtX_inv
    se = np.sqrt(np.diag(cov))
    coef_T = float(beta[1])
    se_T = float(se[1])
    t_T = coef_T / se_T if se_T > 0 else 0.0
    return coef_T, se_T, t_T, dof


def _rv(fq):
    return 0.5 * (math.sqrt(fq ** 4 + 4 * fq ** 2) - fq ** 2)


def compute(df, params):
    nums = df.select_dtypes(include=[np.number]).columns.tolist()
    T = params.get("treatment")
    Y = params.get("outcome")
    covars = list(params.get("covariates") or [])
    q = float(params.get("q", 1.0))
    alpha = float(params.get("alpha", 0.05))
    if not (T and Y):
        if len(nums) < 2:
            die("need_treatment_outcome")
        T, Y = nums[0], nums[1]
        covars = [c for c in nums[2:] if c not in (T, Y)]  # rest as adjustment set
    for c in [T, Y] + covars:
        if c not in df.columns:
            die(f"missing_column:{c}")
    d = df[[T, Y] + covars].dropna().astype(float)
    if len(d) < 10:
        die("too_few_complete_rows")

    coef, se, t, dof = _ols_treatment_t(d, T, Y, covars)
    partial_r2 = (t * t) / (t * t + dof)
    f = abs(t) / math.sqrt(dof)
    fq = q * f
    rv_q = _rv(fq)
    z = _norm_ppf(1 - alpha / 2)
    fqa = (q * abs(t) - z) / math.sqrt(dof)
    rv_qa = _rv(fqa) if fqa > 0 else 0.0
    robust = rv_q > RV_ROBUST_THRESHOLD

    model = {
        "format": "causal-sensitivity-1",
        "design": {
            "treatment": T,
            "outcome": Y,
            "covariates": covars,
            "q": q,
            "alpha": alpha,
            "method": "Cinelli-Hazlett 2020 robustness value (linear)",
        },
        "estimate": {
            "coef": round(coef, 6),
            "se": round(se, 6),
            "t_stat": round(t, 6),
            "dof": int(dof),
            "partial_r2_treatment_outcome": round(partial_r2, 6),
        },
        "sensitivity": {
            "robustness_value": round(rv_q, 6),
            "robustness_value_ci": round(rv_qa, 6),
            "robust_threshold": RV_ROBUST_THRESHOLD,
            "robust": bool(robust),
        },
        "interpretation": (
            "An unobserved confounder would need to explain more than "
            f"{round(rv_q * 100, 1)}% of the residual variance of BOTH treatment "
            "and outcome to reduce the estimate by "
            f"{round(q * 100)}% (to zero at q=1)."
        ),
    }
    metrics = {"n_rows": int(len(d)), "n_covariates": len(covars), "dof": int(dof)}
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
