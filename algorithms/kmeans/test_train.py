"""Local test for the kmeans sandbox algorithm. Runs train.py end-to-end on a
synthetic 3-blob dataset and asserts it emits a valid output bundle with the
right cluster count and aggregates. Pure numpy/pandas — no sklearn.

Run: VO python -m pytest algorithms/kmeans/test_train.py
  (or inside the image: docker run ... — see README)
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
    d = tempfile.mkdtemp(prefix="kmeans-test-")
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
    res = subprocess.run(
        [sys.executable, os.path.join(HERE, "train.py")],
        env=env, capture_output=True, text=True,
    )
    assert res.returncode == 0, f"train.py failed: {res.stderr}\n{res.stdout}"
    with open(os.path.join(out_dir, "output.bin"), "rb") as f:
        blob = f.read()
    return blob, res.stdout


def _bundle(blob):
    z = zipfile.ZipFile(io.BytesIO(blob))
    return json.loads(z.read("model.json")), json.loads(z.read("metrics.json"))


def _three_blobs():
    rng = np.random.default_rng(42)
    a = rng.normal([0, 0], 0.3, (40, 2))
    b = rng.normal([6, 6], 0.3, (40, 2))
    c = rng.normal([0, 6], 0.3, (40, 2))
    pts = np.vstack([a, b, c])
    return pd.DataFrame({"x": pts[:, 0], "y": pts[:, 1]})


def test_finds_three_clusters():
    blob, _ = _run_on(_three_blobs(), {"k": 3})
    model, metrics = _bundle(blob)
    assert model["format"] == "vo-kmeans-1"
    assert model["k"] == 3
    assert len(model["centroids"]) == 3
    assert metrics["n_samples"] == 120
    assert sum(metrics["cluster_sizes"]) == 120
    # 3 well-separated blobs of 40 → roughly balanced clusters.
    assert max(metrics["cluster_sizes"]) <= 60
    assert metrics["inertia"] >= 0


def test_deterministic():
    df = _three_blobs()
    b1, _ = _run_on(df, {"k": 3})
    b2, _ = _run_on(df, {"k": 3})
    assert _bundle(b1)[0]["centroids"] == _bundle(b2)[0]["centroids"]


def test_k_clamped_to_n():
    df = pd.DataFrame({"x": [1.0, 2.0], "y": [1.0, 2.0]})
    _, metrics = _bundle(_run_on(df, {"k": 9})[0])
    assert metrics["k"] == 2  # cannot exceed sample count


def test_output_has_no_per_row_labels():
    # Security: the bundle must NOT contain per-row cluster assignments.
    model, metrics = _bundle(_run_on(_three_blobs(), {"k": 3})[0])
    blob_text = json.dumps(model) + json.dumps(metrics)
    assert "labels" not in model and "assignments" not in model
    # cluster_sizes (aggregate) is fine; a 120-length array would be leakage.
    for v in list(model.values()) + list(metrics.values()):
        if isinstance(v, list):
            assert len(v) <= 10, "no per-row-length arrays in the output"


if __name__ == "__main__":
    for fn in [test_finds_three_clusters, test_deterministic, test_k_clamped_to_n, test_output_has_no_per_row_labels]:
        fn()
        print(f"ok: {fn.__name__}")
    print("all passed")
