"""Local test for the DP descriptive-statistics algorithm. Pure numpy/pandas.

Verifies the output structure, that a large epsilon yields estimates close to
the true values (little noise), that a tiny epsilon is noticeably noisier, that
supplied bounds are honored, and that a missing/zero epsilon fails closed.
"""
import io
import json
import os
import subprocess
import sys
import tempfile
import zipfile

import numpy as np
import pandas as pd

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import stats  # noqa: E402


def _run(df, params):
    d = tempfile.mkdtemp(prefix="dpstats-test-")
    data, out = os.path.join(d, "data"), os.path.join(d, "out")
    os.makedirs(data)
    os.makedirs(out)
    df.to_csv(os.path.join(data, "input.csv"), index=False)
    pf = os.path.join(d, "params.json")
    with open(pf, "w") as f:
        json.dump(params, f)
    env = dict(os.environ, VO_DATA_DIR=data, VO_OUT_DIR=out, VO_PARAMS=pf)
    res = subprocess.run([sys.executable, os.path.join(HERE, "stats.py")], env=env,
                         capture_output=True, text=True)
    return res, out


def _metrics(out):
    with open(os.path.join(out, "output.bin"), "rb") as f:
        z = zipfile.ZipFile(io.BytesIO(f.read()))
    return json.loads(z.read("metrics.json"))


def _frame(n=2000):
    rng = np.random.default_rng(0)  # fixed seed for the DATA (not the algorithm)
    return pd.DataFrame({"age": rng.normal(40, 5, n), "score": rng.normal(100, 10, n)})


def test_structure_and_large_epsilon_accuracy():
    df = _frame()
    res, out = _run(df, {"_epsilon": 50.0, "bounds": {"age": [0, 120], "score": [0, 200]}})
    assert res.returncode == 0, res.stderr
    m = _metrics(out)
    assert m["format"] == "vo-dp-stats-1"
    assert m["mechanism"] == "laplace"
    assert set(m["columns"]) == {"age", "score"}
    # Large epsilon → little noise → close to truth.
    assert abs(m["count_dp"] - len(df)) < 50
    assert abs(m["columns"]["age"]["mean_dp"] - df["age"].mean()) < 2.0
    assert m["columns"]["age"]["bounds_source"] == "supplied"


def test_small_epsilon_is_noisier():
    df = _frame()
    # Average |error| over a few small-epsilon runs should exceed a large-epsilon run.
    def err(eps):
        errs = []
        for _ in range(5):
            _, out = _run(df, {"_epsilon": eps, "bounds": {"age": [0, 120]}, "columns": ["age"]})
            errs.append(abs(_metrics(out)["columns"]["age"]["mean_dp"] - df["age"].mean()))
        return float(np.mean(errs))

    assert err(0.05) > err(50.0)


def test_missing_epsilon_fails_closed():
    df = _frame(50)
    res, _ = _run(df, {})  # no epsilon
    assert res.returncode != 0  # DP must not run without a budget


def test_observed_bounds_flagged_honestly():
    df = _frame(200)
    _, out = _run(df, {"_epsilon": 10.0, "columns": ["age"]})  # no bounds supplied
    m = _metrics(out)
    assert "data-dependent" in m["columns"]["age"]["bounds_source"]


def test_sum_sensitivity_is_addremove_max_abs():
    # Under the add/remove-one neighbor model (the count uses sensitivity 1), the L1
    # sum sensitivity of a value clamped to [lo,hi] is max(|lo|,|hi|), NOT hi-lo.
    # Using hi-lo under-noises when the bounds don't straddle 0, so the achieved
    # epsilon exceeds the claimed budget — a silent privacy violation.
    assert stats.sum_sensitivity(10, 20) == 20
    assert stats.sum_sensitivity(18, 65) == 65
    assert stats.sum_sensitivity(-5, 3) == 5
    assert stats.sum_sensitivity(0, 120) == 120  # the only case where hi-lo coincided


def test_observed_bounds_not_leaked_into_output():
    # When bounds are NOT supplied, the raw column min & max are the most disclosive
    # order statistics; they must NOT be emitted verbatim (zero-noise) in the output.
    lo_raw, hi_raw = 1234.5, 9876.5
    df = pd.DataFrame({"v": [lo_raw] + [5000.0] * 98 + [hi_raw]})
    _, out = _run(df, {"_epsilon": 10.0, "columns": ["v"]})  # observed bounds path
    m = _metrics(out)
    blob = json.dumps(m)
    assert str(lo_raw) not in blob and str(hi_raw) not in blob, "raw extremes leaked: " + blob
    assert "bounds" not in m["columns"]["v"], "observed bounds must be withheld from output"


if __name__ == "__main__":
    test_structure_and_large_epsilon_accuracy()
    test_small_epsilon_is_noisier()
    test_missing_epsilon_fails_closed()
    test_observed_bounds_flagged_honestly()
    test_sum_sensitivity_is_addremove_max_abs()
    test_observed_bounds_not_leaked_into_output()
    print("OK: all dp_stats tests passed")
