"""Tests for the causal-refutation C2D algorithm.

A genuine effect should be VALIDATED: placebo collapses it to ~0, while
random-common-cause and data-subset leave it stable. Output is aggregates only.
Run:  python test_refute.py
"""
import numpy as np
import pandas as pd

import refute as REF

ALLOWED_MODEL_KEYS = {"format", "design", "original_effect", "refuters", "evidence_level"}


def _real_effect(n=500):
    rng = np.random.default_rng(0)
    T = rng.normal(0, 1, n)
    Y = 2.0 * T + rng.normal(0, 0.5, n)
    return pd.DataFrame({"T": T, "Y": Y})


def test_real_effect_is_validated():
    model, _ = REF.compute(_real_effect(), {"treatment": "T", "outcome": "Y"})
    assert abs(model["original_effect"] - 2.0) < 0.2
    by = {r["name"]: r for r in model["refuters"]}
    assert by["placebo_treatment"]["passed"] is True, by["placebo_treatment"]
    assert abs(by["placebo_treatment"]["placebo_effect_mean"]) < 0.2
    assert by["random_common_cause"]["passed"] is True
    assert by["data_subset"]["passed"] is True
    assert model["evidence_level"] == "validated"


def test_aggregates_only():
    model, metrics = REF.compute(_real_effect(120), {"treatment": "T", "outcome": "Y"})
    assert set(model).issubset(ALLOWED_MODEL_KEYS), set(model) - ALLOWED_MODEL_KEYS
    assert metrics["n_rows"] == 120
    # placebo permutation p-value should be tiny for a strong real effect
    p = next(r for r in model["refuters"] if r["name"] == "placebo_treatment")["permutation_p_value"]
    assert p < 0.05, p


def test_default_first_two_numeric_columns():
    df = _real_effect(60).rename(columns={"T": "dose", "Y": "resp"})
    model, _ = REF.compute(df, {})
    assert model["design"]["treatment"] == "dose" and model["design"]["outcome"] == "resp"


def test_zero_effect_is_not_validated():
    # A constant outcome → original effect 0. The old 1e-12 floor made every
    # refuter pass → "validated", certifying a non-existent effect (the worst
    # inversion). It must be "undefined", never "validated".
    rng = np.random.default_rng(0)
    n = 300
    df = pd.DataFrame({"T": rng.normal(0, 1, n), "Y": np.full(n, 5.0)})
    model, _ = REF.compute(df, {"treatment": "T", "outcome": "Y"})
    assert abs(model["original_effect"]) < 1e-9, model["original_effect"]
    assert model["evidence_level"] == "undefined", model["evidence_level"]


if __name__ == "__main__":
    test_real_effect_is_validated()
    test_aggregates_only()
    test_default_first_two_numeric_columns()
    test_zero_effect_is_not_validated()
    print("OK: all refutation tests passed")
