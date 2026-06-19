"""Tests for the causal-mediation C2D algorithm.

Plants a KNOWN linear mediation and asserts the Pearl NDE/NIE decomposition is
recovered, the Pearl identity (NDE+NIE==ATE) holds, and the output is aggregates
only. Run with:  python test_mediate.py
"""
import json

import numpy as np
import pandas as pd

import mediate as MED

ALLOWED_MODEL_KEYS = {"format", "design", "effects", "pearl_invariant_residual"}
ALLOWED_EFFECT_KEYS = {
    "nde", "nie", "ate", "proportion_mediated",
    "nde_ci95", "nie_ci95", "ate_ci95", "proportion_mediated_ci95",
}


def _planted(n=6000):
    # T -> M (alpha_T=2), {T,M} -> Y (beta_T=3 = NDE, beta_M=4).
    # => NIE = alpha_T*beta_M = 8, NDE = 3, ATE = 11, prop_mediated = 8/11.
    rng = np.random.default_rng(0)
    T = rng.normal(0, 1, n)
    M = 2.0 * T + rng.normal(0, 0.5, n)
    Y = 3.0 * T + 4.0 * M + rng.normal(0, 0.5, n)
    return pd.DataFrame({"T": T, "M": M, "Y": Y})


def test_recovers_known_mediation():
    df = _planted()
    model, metrics = MED.compute(df, {"treatment": "T", "mediator": "M", "outcome": "Y"})
    e = model["effects"]
    assert abs(e["nde"] - 3.0) < 0.2, e
    assert abs(e["nie"] - 8.0) < 0.3, e
    assert abs(e["ate"] - 11.0) < 0.3, e
    assert abs(e["proportion_mediated"] - 8.0 / 11.0) < 0.05, e
    assert model["pearl_invariant_residual"] < 1e-6, model["pearl_invariant_residual"]
    # bootstrap CI brackets the point estimate
    lo, hi = e["nie_ci95"]
    assert lo <= e["nie"] <= hi, e


def test_aggregates_only():
    df = _planted(200)
    model, metrics = MED.compute(df, {"treatment": "T", "mediator": "M", "outcome": "Y"})
    assert set(model).issubset(ALLOWED_MODEL_KEYS), set(model) - ALLOWED_MODEL_KEYS
    assert set(model["effects"]).issubset(ALLOWED_EFFECT_KEYS)
    # nothing per-row: the only n is the aggregate count
    assert metrics["n_rows"] == 200
    # design echoes only the (caller-chosen) column names, never values
    assert model["design"]["treatment"] == "T"
    json.dumps(model)  # serializable


def test_default_first_three_numeric_columns():
    df = _planted(300)
    model, _ = MED.compute(df, {})  # no params -> first 3 numeric cols T,M,Y
    assert model["design"]["treatment"] == "T"


if __name__ == "__main__":
    test_recovers_known_mediation()
    test_aggregates_only()
    test_default_first_three_numeric_columns()
    print("OK: all mediate tests passed")
