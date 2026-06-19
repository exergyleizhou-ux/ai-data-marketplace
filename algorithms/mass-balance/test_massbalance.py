"""Tests for the mass-balance C2D algorithm.  Run: python test_massbalance.py"""
import numpy as np
import pandas as pd

import massbalance as MB

ALLOWED = {"format", "design", "closure", "residual_epsilon", "within_tolerance_fraction", "mean_output_fraction"}


def _runs(n=300, closure=0.97, seed=0):
    rng = np.random.default_rng(seed)
    feed = rng.uniform(50, 200, n)
    # outputs sum to ~closure * feed, split across product/residue/loss
    accounted = feed * closure * (1 + rng.normal(0, 0.01, n))
    product = accounted * 0.45
    residue = accounted * 0.40
    loss = accounted * 0.15
    return pd.DataFrame({"feed_g": feed, "product_g": product, "residue_g": residue, "loss_g": loss})


def test_closure_recovered():
    model, metrics = MB.compute(_runs(closure=0.97),
                                {"input": "feed_g", "outputs": ["product_g", "residue_g", "loss_g"]})
    assert abs(model["closure"]["mean"] - 0.97) < 0.02, model["closure"]
    assert abs(model["residual_epsilon"]["mean"] - 0.03) < 0.02
    assert metrics["n_runs"] == 300
    assert set(model["mean_output_fraction"]) == {"product_g", "residue_g", "loss_g"}


def test_within_tolerance():
    # near-perfect closure -> most runs within 5% tolerance
    model, _ = MB.compute(_runs(closure=0.99), {"input": "feed_g", "outputs": ["product_g", "residue_g", "loss_g"]})
    assert model["within_tolerance_fraction"] > 0.9


def test_aggregates_only():
    model, _ = MB.compute(_runs(60), {"input": "feed_g", "outputs": ["product_g", "residue_g", "loss_g"]})
    assert set(model).issubset(ALLOWED), set(model) - ALLOWED


if __name__ == "__main__":
    test_closure_recovered()
    test_within_tolerance()
    test_aggregates_only()
    print("OK: all mass-balance tests passed")
