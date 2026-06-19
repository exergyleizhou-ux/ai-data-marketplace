"""Tests for the causal-sensitivity C2D algorithm.

A strong effect should yield a high robustness value (robust); a weak, noisy
effect a low one (fragile). Output must be aggregates only. Run:
    python test_sensitivity.py
"""
import numpy as np
import pandas as pd

import sensitivity as SEN

ALLOWED_MODEL_KEYS = {"format", "design", "estimate", "sensitivity", "interpretation"}


def test_strong_effect_is_robust():
    rng = np.random.default_rng(0)
    n = 2000
    T = rng.normal(0, 1, n)
    Y = 3.0 * T + rng.normal(0, 0.3, n)
    df = pd.DataFrame({"T": T, "Y": Y})
    model, _ = SEN.compute(df, {"treatment": "T", "outcome": "Y"})
    s = model["sensitivity"]
    assert 0.0 <= s["robustness_value"] <= 1.0
    assert s["robustness_value"] > 0.10 and s["robust"] is True, s
    assert model["estimate"]["partial_r2_treatment_outcome"] > 0.5


def test_weak_noisy_effect_is_fragile():
    rng = np.random.default_rng(1)
    n = 500
    T = rng.normal(0, 1, n)
    Y = 0.02 * T + rng.normal(0, 3.0, n)
    df = pd.DataFrame({"T": T, "Y": Y})
    model, _ = SEN.compute(df, {"treatment": "T", "outcome": "Y"})
    s = model["sensitivity"]
    assert s["robustness_value"] < 0.10 and s["robust"] is False, s
    assert s["robustness_value_ci"] <= s["robustness_value"]


def test_aggregates_only_and_bounds():
    rng = np.random.default_rng(2)
    n = 300
    T = rng.normal(0, 1, n)
    Y = 1.0 * T + rng.normal(0, 1, n)
    df = pd.DataFrame({"T": T, "Y": Y})
    model, metrics = SEN.compute(df, {"treatment": "T", "outcome": "Y"})
    assert set(model).issubset(ALLOWED_MODEL_KEYS), set(model) - ALLOWED_MODEL_KEYS
    pr2 = model["estimate"]["partial_r2_treatment_outcome"]
    assert 0.0 <= pr2 <= 1.0
    assert 0.0 <= model["sensitivity"]["robustness_value"] <= 1.0
    assert metrics["n_rows"] == n


def test_default_first_two_numeric_columns():
    rng = np.random.default_rng(3)
    df = pd.DataFrame({"a": rng.normal(0, 1, 200), "b": rng.normal(0, 1, 200)})
    model, _ = SEN.compute(df, {})
    assert model["design"]["treatment"] == "a" and model["design"]["outcome"] == "b"


if __name__ == "__main__":
    test_strong_effect_is_robust()
    test_weak_noisy_effect_is_fragile()
    test_aggregates_only_and_bounds()
    test_default_first_two_numeric_columns()
    print("OK: all sensitivity tests passed")
