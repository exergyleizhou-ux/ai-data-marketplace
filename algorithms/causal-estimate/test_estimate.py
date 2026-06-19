"""Tests for the causal-estimate C2D algorithm.  Run: python test_estimate.py"""
import numpy as np
import pandas as pd

import estimate as EST

ALLOWED = {"format", "design", "estimate", "treatment"}


def _data(ate=1.5, n=2000, binary=False):
    rng = np.random.default_rng(0)
    cov = rng.normal(0, 1, n)
    T = rng.integers(0, 2, n).astype(float) if binary else rng.normal(0, 1, n)
    Y = ate * T + 0.3 * cov + rng.normal(0, 0.4, n)
    return pd.DataFrame({"T": T, "Y": Y, "cov": cov})


def test_recovers_continuous_ate():
    model, _ = EST.compute(_data(1.5), {"treatment": "T", "outcome": "Y", "covariates": ["cov"]})
    e = model["estimate"]
    assert abs(e["ate_ols"] - 1.5) < 0.1, e
    assert abs(e["ate_dml"] - 1.5) < 0.15, e
    assert e["p_value"] < 0.01
    lo, hi = e["ci95"]
    assert lo <= e["ate_ols"] <= hi
    assert model["treatment"]["treatment_type"] == "continuous"


def test_binary_treatment():
    model, _ = EST.compute(_data(2.0, binary=True), {"treatment": "T", "outcome": "Y", "covariates": ["cov"]})
    tr = model["treatment"]
    assert tr["treatment_type"] == "binary"
    assert tr["n_treated"] + tr["n_control"] == 2000
    assert abs(model["estimate"]["ate_ols"] - 2.0) < 0.15


def test_aggregates_only():
    model, metrics = EST.compute(_data(n=200), {"treatment": "T", "outcome": "Y", "covariates": ["cov"]})
    assert set(model).issubset(ALLOWED), set(model) - ALLOWED
    assert metrics["n_rows"] == 200


if __name__ == "__main__":
    test_recovers_continuous_ate()
    test_binary_treatment()
    test_aggregates_only()
    print("OK: all estimate tests passed")
