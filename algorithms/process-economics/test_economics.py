"""Tests for the process-economics C2D algorithm.  Run: python test_economics.py"""
import numpy as np
import pandas as pd

import economics as ECO

ALLOWED = {"format", "design", "revenue", "cost", "margin", "unit_cost", "profitable_batch_fraction", "cost_breakdown_total"}


def _batches(n=200, seed=0):
    rng = np.random.default_rng(seed)
    product = rng.uniform(8, 12, n)              # kg product / batch
    substrate = rng.uniform(4, 6, n)
    energy = rng.uniform(3, 5, n)
    labor = rng.uniform(4, 6, n)
    return pd.DataFrame({"product_kg": product, "substrate_cost": substrate,
                         "energy_cost": energy, "labor_cost": labor})


def test_margin_and_profitability():
    df = _batches()
    # product~10kg @ price 2.5 = ~25 revenue; cost ~14 -> margin ~11 (profitable)
    model, metrics = ECO.compute(df, {"product": "product_kg", "price": 2.5,
                                       "costs": ["substrate_cost", "energy_cost", "labor_cost"]})
    assert model["margin"]["mean"] > 8
    assert model["profitable_batch_fraction"] == 1.0
    assert metrics["n_batches"] == 200
    assert set(model["cost_breakdown_total"]) == {"substrate_cost", "energy_cost", "labor_cost"}


def test_unprofitable_when_price_low():
    df = _batches()
    model, _ = ECO.compute(df, {"product": "product_kg", "price": 0.5,
                                "costs": ["substrate_cost", "energy_cost", "labor_cost"]})
    assert model["margin"]["mean"] < 0
    assert model["profitable_batch_fraction"] < 0.5


def test_aggregates_only():
    df = _batches(40)
    model, _ = ECO.compute(df, {"product": "product_kg", "price": 2.5,
                                "costs": ["substrate_cost", "energy_cost", "labor_cost"]})
    assert set(model).issubset(ALLOWED), set(model) - ALLOWED


if __name__ == "__main__":
    test_margin_and_profitability()
    test_unprofitable_when_price_low()
    test_aggregates_only()
    print("OK: all economics tests passed")
