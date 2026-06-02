"""Local test for the logreg sandbox algorithm. Runs train.py end-to-end on a
synthetic separable dataset and asserts it emits a valid output bundle with a
sensible holdout accuracy. Pure numpy/pandas — no sklearn needed.

Run: VO python -m pytest algorithms/logreg/test_train.py
  (or: python algorithms/logreg/test_train.py for a plain run)
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


def _run_on(df, params=None):
    d = tempfile.mkdtemp(prefix="logreg-test-")
    data_dir = os.path.join(d, "data")
    out_dir = os.path.join(d, "out")
    os.makedirs(data_dir)
    os.makedirs(out_dir)
    df.to_csv(os.path.join(data_dir, "input.csv"), index=False)
    env = dict(os.environ, VO_DATA_DIR=data_dir, VO_OUT_DIR=out_dir)
    if params is not None:
        pf = os.path.join(d, "params.json")
        with open(pf, "w") as f:
            json.dump(params, f)
        env["VO_PARAMS"] = pf
    res = subprocess.run([sys.executable, os.path.join(HERE, "train.py")], env=env,
                         capture_output=True, text=True)
    assert res.returncode == 0, f"train.py failed: {res.stderr}\n{res.stdout}"
    with open(os.path.join(out_dir, "output.bin"), "rb") as f:
        blob = f.read()
    return blob, res.stdout


def _separable(n=300, seed=0):
    rng = np.random.default_rng(seed)
    x1 = rng.normal(0, 1, n)
    x2 = rng.normal(0, 1, n)
    label = (x1 + x2 + rng.normal(0, 0.2, n) > 0).astype(int)
    return pd.DataFrame({"f1": x1, "f2": x2, "label": label})


def test_produces_valid_model_bundle():
    blob, _ = _run_on(_separable())
    z = zipfile.ZipFile(io.BytesIO(blob))
    names = set(z.namelist())
    assert names == {"model.json", "metrics.json"}, names
    model = json.loads(z.read("model.json"))
    metrics = json.loads(z.read("metrics.json"))
    assert model["format"] == "vo-logreg-1"
    assert model["features"] == ["f1", "f2"]
    assert len(model["weights"]) == 2
    # A linearly separable problem should train well.
    assert metrics["holdout_accuracy"] >= 0.85, metrics
    assert metrics["n_features"] == 2


def test_logs_carry_no_raw_data():
    df = _separable(n=50)
    _, stdout = _run_on(df)
    # Every stdout line must be a structured JSON log with a "stage" key —
    # i.e. no raw rows leaked to stdout (design §7.4).
    for line in stdout.strip().splitlines():
        obj = json.loads(line)
        assert "stage" in obj


def test_explicit_target_param():
    df = _separable()
    df = df.rename(columns={"label": "y"})
    blob, _ = _run_on(df, params={"target": "y"})
    model = json.loads(zipfile.ZipFile(io.BytesIO(blob)).read("model.json"))
    assert model["target"] == "y"
    assert "y" not in model["features"]


if __name__ == "__main__":
    test_produces_valid_model_bundle()
    test_logs_carry_no_raw_data()
    test_explicit_target_param()
    print("OK: all logreg tests passed")
