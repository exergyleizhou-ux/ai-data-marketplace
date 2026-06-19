"""Tests for the PaperGuard C2D data-integrity screen.

Runs the REAL paperguard tabular detectors and asserts the aggregates-only
privacy invariant (the whole point of running it compute-to-data). Requires
paperguard installed — it is, inside the algorithm's Docker image; run with:

    docker run --rm -v "$PWD/test_screen.py:/app/test_screen.py" \
        --entrypoint python <image> /app/test_screen.py
"""
import json

import numpy as np
import pandas as pd

import screen as S

FORBIDDEN_KEYS = {"evidence", "detail", "summary"}
ALLOWED_DETECTOR_FIELDS = {
    "detector_id",
    "detector_name",
    "applicable",
    "finding_count",
    "severity_counts",
    "min_p_value",
    "min_p_value_adjusted",
    "test_names",
}


def _no_forbidden(obj):
    if isinstance(obj, dict):
        for k, v in obj.items():
            assert k not in FORBIDDEN_KEYS, f"leak: forbidden key {k!r} in output"
            _no_forbidden(v)
    elif isinstance(obj, list):
        for v in obj:
            _no_forbidden(v)


def test_screen_structure_and_aggregates_only():
    # A column whose values ALL end in 0/5 → terminal-digit over-representation.
    # Plant a distinctive sentinel value to prove it never leaks into the output.
    sentinel = 7654325
    vals = [(i % 200 + 1) * 5 for i in range(250)]
    vals[0] = sentinel
    df = pd.DataFrame({"measurement": vals, "idx": list(range(250))})

    model = S.screen({"sheet": df}, S.TABLE_DETECTORS)

    assert model["format"] == "paperguard-screen-1"
    ov = model["overall"]
    assert ov["n_detectors_run"] >= 1
    assert ov["verdict"] in {"clean", "anomalies_flagged"}

    # aggregates-only privacy invariant
    _no_forbidden(model)
    blob = json.dumps(model)
    assert str(sentinel) not in blob, "raw cell value leaked into the output"
    for d in model["detectors"]:
        extra = set(d) - ALLOWED_DETECTOR_FIELDS
        assert not extra, f"unexpected (possibly leaky) detector field(s): {extra}"


def test_screen_flags_obvious_anomaly():
    # 100% multiples of 5 is a blatant terminal-digit anomaly.
    df = pd.DataFrame({"x": [(i % 50 + 1) * 5 for i in range(300)]})
    model = S.screen({"sheet": df}, S.TABLE_DETECTORS)
    assert model["overall"]["total_findings"] >= 1, model["overall"]


def test_clean_data_structure_and_safety():
    rng = np.random.default_rng(0)
    df = pd.DataFrame(
        {"a": rng.normal(100, 15, 200), "b": rng.uniform(1, 9, 200)}
    )
    model = S.screen({"sheet": df}, S.TABLE_DETECTORS)
    assert model["overall"]["n_detectors_run"] >= 1
    _no_forbidden(model)


if __name__ == "__main__":
    test_screen_structure_and_aggregates_only()
    test_screen_flags_obvious_anomaly()
    test_clean_data_structure_and_safety()
    print("OK: all screen tests passed")
