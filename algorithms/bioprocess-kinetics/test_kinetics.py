"""Tests for the bioprocess-kinetics C2D algorithm.

Fits synthetic logistic growth and asserts the carrying capacity / rate are
recovered with a high R², and that the output is aggregates only. Run:
    python test_kinetics.py
"""
import numpy as np
import pandas as pd

import kinetics as KIN

ALLOWED_MODEL_KEYS = {"format", "design", "fits", "best_model", "best_r2", "derived", "fit_ok"}


def _logistic_data(K=10.0, B0=0.1, r=0.8, n=40, noise=0.02, seed=0):
    rng = np.random.default_rng(seed)
    t = np.linspace(0, 15, n)
    B = K / (1 + ((K - B0) / B0) * np.exp(-r * t))
    B = B * (1 + rng.normal(0, noise, n))
    return pd.DataFrame({"hour": t, "biomass": B})


def test_recovers_logistic():
    df = _logistic_data()
    model, metrics = KIN.compute(df, {"time": "hour", "value": "biomass"})
    assert model["best_model"] in ("logistic", "gompertz")
    assert model["best_r2"] > 0.98, model["best_r2"]
    lg = next(f for f in model["fits"] if f["model"] == "logistic")
    assert lg["converged"] and lg["r2"] > 0.98
    K = lg["params"][1]
    assert abs(K - 10.0) < 1.0, K  # carrying capacity recovered
    assert metrics["n_points"] == 40


def test_aggregates_only():
    df = _logistic_data(n=20)
    model, _ = KIN.compute(df, {"time": "hour", "value": "biomass"})
    assert set(model).issubset(ALLOWED_MODEL_KEYS), set(model) - ALLOWED_MODEL_KEYS
    # no per-row predictions leak: a fit carries only params + scalar fit stats
    for f in model["fits"]:
        assert set(f).issubset({"model", "converged", "params", "r2", "rmse", "reason"})


def test_default_first_two_numeric_columns():
    df = _logistic_data(n=15).rename(columns={"hour": "a", "biomass": "b"})
    model, _ = KIN.compute(df, {})
    assert model["design"]["time"] == "a" and model["design"]["value"] == "b"


def test_noise_data_not_confidently_fitted():
    # Pure noise (no growth) must NOT yield confident kinetic constants. The old code
    # picked a "best" model by relative R² with no quality floor and emitted a
    # populated `derived` block (carrying_capacity, doubling_time, …) for noise.
    rng = np.random.default_rng(1)
    df = pd.DataFrame({"hour": np.linspace(0, 15, 40), "biomass": rng.uniform(1, 5, 40)})
    model, _ = KIN.compute(df, {"time": "hour", "value": "biomass"})
    assert model["fit_ok"] is False, model.get("best_r2")
    assert model["best_model"] is None
    assert model["derived"] == {}


def test_good_fit_still_reported():
    # A genuine logistic curve must still pass the gate and report its constants.
    model, _ = KIN.compute(_logistic_data(), {"time": "hour", "value": "biomass"})
    assert model["fit_ok"] is True
    assert model["best_model"] in ("logistic", "gompertz")
    assert model["derived"]  # non-empty


if __name__ == "__main__":
    test_recovers_logistic()
    test_aggregates_only()
    test_default_first_two_numeric_columns()
    test_noise_data_not_confidently_fitted()
    test_good_fit_still_reported()
    print("OK: all kinetics tests passed")
