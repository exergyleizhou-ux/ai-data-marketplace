"""Local test for the GNB (Gaussian Naive Bayes) sandbox algorithm. Runs
train.py end-to-end on synthetic datasets with hand-computable answers and
asserts it emits a valid output bundle.

Pure stdlib — no numpy/pandas/sklearn — so it runs with the bare system python,
mirroring the container, which also needs no third-party deps (tiny audit
surface for an L1 trusted algorithm).

Run: python -m pytest algorithms/gnb/test_train.py    (or: python test_train.py)
"""
import io
import json
import os
import subprocess
import sys
import tempfile
import zipfile

HERE = os.path.dirname(os.path.abspath(__file__))


def _run_on(header, rows, params=None):
    """Write a CSV (header + rows) to a temp /data dir, run train.py with the
    VO_* path overrides, and return (output.bin bytes, stdout)."""
    d = tempfile.mkdtemp(prefix="gnb-test-")
    data_dir = os.path.join(d, "data")
    out_dir = os.path.join(d, "out")
    os.makedirs(data_dir)
    os.makedirs(out_dir)
    with open(os.path.join(data_dir, "input.csv"), "w") as f:
        f.write(",".join(header) + "\n")
        for r in rows:
            f.write(",".join(str(c) for c in r) + "\n")
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


def _approx(a, b, tol=1e-4):
    return abs(a - b) <= tol


def _tiny_two_class():
    # class 0: f1 = [1,2,3]   -> mean 2,  population variance 2/3
    # class 1: f1 = [11,12,13] -> mean 12, population variance 2/3
    # priors 0.5 / 0.5
    return ["f1", "label"], [[1, 0], [2, 0], [3, 0], [11, 1], [12, 1], [13, 1]]


def test_fit_exact_params():
    """The core of TDD: assert the closed-form GNB statistics exactly."""
    blob, _ = _run_on(*_tiny_two_class())
    model, _ = _bundle(blob)
    assert model["format"] == "vo-gnb-1"
    assert model["features"] == ["f1"]
    assert model["classes"] == ["0", "1"]
    assert _approx(model["priors"]["0"], 0.5)
    assert _approx(model["priors"]["1"], 0.5)
    assert _approx(model["theta"]["0"][0], 2.0)
    assert _approx(model["theta"]["1"][0], 12.0)
    assert _approx(model["var"]["0"][0], 2.0 / 3.0)
    assert _approx(model["var"]["1"][0], 2.0 / 3.0)


def test_predicts_separable_perfectly():
    """Two well-separated 2-D classes -> train accuracy 1.0."""
    header = ["f1", "f2", "label"]
    rows = []
    for i in range(20):
        rows.append([0 + i * 0.01, 0 + i * 0.01, 0])
        rows.append([10 + i * 0.01, 10 + i * 0.01, 1])
    blob, _ = _run_on(header, rows)
    model, metrics = _bundle(blob)
    assert metrics["n_train"] == 40
    assert metrics["n_features"] == 2
    assert metrics["class_counts"] == {"0": 20, "1": 20}
    assert _approx(metrics["accuracy"], 1.0)


def test_deterministic():
    """GNB fit is closed-form -> byte-identical model across runs."""
    b1, _ = _run_on(*_tiny_two_class())
    b2, _ = _run_on(*_tiny_two_class())
    assert _bundle(b1)[0] == _bundle(b2)[0]


def test_label_param_overrides_default():
    """Default label column is 'label'; a param can point at another column,
    and non-numeric class labels are supported."""
    header = ["f1", "y"]
    rows = [[1, "a"], [2, "a"], [9, "b"], [10, "b"]]
    blob, _ = _run_on(header, rows, params={"label": "y"})
    model, _ = _bundle(blob)
    assert model["features"] == ["f1"]
    assert model["classes"] == ["a", "b"]


def test_no_per_row_leakage():
    """Security (mirrors kmeans): the bundle carries only per-class/per-feature
    aggregates — never a per-row-length array of predictions."""
    header = ["f1", "f2", "label"]
    rows = [[i, i % 7, i % 2] for i in range(60)]
    model, metrics = _bundle(_run_on(header, rows)[0])
    assert "predictions" not in model and "predictions" not in metrics
    def _check(container):
        # Bound BOTH list lengths and dict key counts: the class-keyed dicts
        # (priors/theta/var/class_counts) carry one key per class, so an exploded
        # label cardinality would surface as an oversized dict, not just a list.
        assert len(container) <= 8, "no per-row-cardinality containers in output"
        for v in container.values():
            if isinstance(v, list):
                assert len(v) <= 8, "no per-row-length arrays in output"
            elif isinstance(v, dict):
                _check(v)
    _check(model)
    _check(metrics)


def test_refuses_high_cardinality_label():
    """L1 privacy guard: a high-cardinality label (an id/email-like column) must
    be refused — never echoed as one class per row."""
    header = ["f1", "label"]
    rows = [[i, f"id{i}"] for i in range(20)]  # 20 unique labels over 20 rows
    d = tempfile.mkdtemp(prefix="gnb-test-")
    data_dir = os.path.join(d, "data")
    out_dir = os.path.join(d, "out")
    os.makedirs(data_dir)
    os.makedirs(out_dir)
    with open(os.path.join(data_dir, "input.csv"), "w") as f:
        f.write(",".join(header) + "\n")
        for r in rows:
            f.write(",".join(str(c) for c in r) + "\n")
    env = dict(os.environ, VO_DATA_DIR=data_dir, VO_OUT_DIR=out_dir)
    res = subprocess.run(
        [sys.executable, os.path.join(HERE, "train.py")],
        env=env, capture_output=True, text=True,
    )
    assert res.returncode != 0, "high-cardinality label must be refused"
    assert not os.path.exists(os.path.join(out_dir, "output.bin")), \
        "a refused label must not produce an output bundle"


if __name__ == "__main__":
    for fn in [test_fit_exact_params, test_predicts_separable_perfectly,
               test_deterministic, test_label_param_overrides_default,
               test_no_per_row_leakage, test_refuses_high_cardinality_label]:
        fn()
        print(f"ok: {fn.__name__}")
    print("all passed")
