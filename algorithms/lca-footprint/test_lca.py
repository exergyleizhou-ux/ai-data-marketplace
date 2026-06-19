"""Tests for the lca-footprint C2D algorithm.  Run: python test_lca.py"""
import numpy as np
import pandas as pd

import lca as LCA

ALLOWED = {"format", "design", "gwp_total_kgco2e", "gwp_per_run", "contribution_by_activity_kgco2e", "gwp_per_product_unit"}


def _runs(n=200, seed=0):
    rng = np.random.default_rng(seed)
    return pd.DataFrame({
        "electricity_kwh": rng.uniform(80, 120, n),
        "transport_km": rng.uniform(40, 60, n),
        "product_kg": rng.uniform(9, 11, n),
    })


def test_gwp_with_explicit_factors():
    df = _runs()
    # 100 kWh * 0.5 + 50 km * 0.12 = 56 kg CO2e per run (means)
    model, metrics = LCA.compute(df, {"activities": {"electricity_kwh": 0.5, "transport_km": 0.12}, "product": "product_kg"})
    assert abs(model["gwp_per_run"]["mean"] - 56.0) < 3, model["gwp_per_run"]
    assert set(model["contribution_by_activity_kgco2e"]) == {"electricity_kwh", "transport_km"}
    assert model["gwp_per_product_unit"] > 0
    assert metrics["n_runs"] == 200


def test_default_factor_matching():
    df = _runs(50)
    # electricity_kwh & transport_km are in DEFAULT_FACTORS -> matched without params
    model, _ = LCA.compute(df, {})
    assert "electricity_kwh" in model["contribution_by_activity_kgco2e"]
    assert model["gwp_total_kgco2e"] > 0


def test_aggregates_only():
    df = _runs(30)
    model, _ = LCA.compute(df, {"activities": {"electricity_kwh": 0.5}})
    assert set(model).issubset(ALLOWED), set(model) - ALLOWED


if __name__ == "__main__":
    test_gwp_with_explicit_factors()
    test_default_factor_matching()
    test_aggregates_only()
    print("OK: all lca tests passed")
