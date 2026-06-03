"""Unit tests for the fed-logreg local trainer (runs in the sidecar CI job)."""
import json
import os
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))


def _run(csv_text, params=None):
    """Run train.py against a temp dataset; return the parsed fedparams output."""
    d = tempfile.mkdtemp()
    data_dir = os.path.join(d, "data")
    out_dir = os.path.join(d, "out")
    os.makedirs(data_dir)
    with open(os.path.join(data_dir, "data.csv"), "w") as f:
        f.write(csv_text)
    env = dict(os.environ, VO_DATA_DIR=data_dir, VO_OUT_DIR=out_dir)
    if params is not None:
        pf = os.path.join(d, "params.json")
        with open(pf, "w") as f:
            json.dump(params, f)
        env["VO_PARAMS"] = pf
    else:
        env["VO_PARAMS"] = os.path.join(d, "missing.json")
    r = subprocess.run([sys.executable, os.path.join(HERE, "train.py")],
                       env=env, capture_output=True, text=True)
    assert r.returncode == 0, f"train failed: {r.stderr}\n{r.stdout}"
    with open(os.path.join(out_dir, "output.bin"), "rb") as f:
        return json.loads(f.read().decode("utf-8"))


CSV_A = "x1,x2,y\n0,0,0\n0,1,0\n1,0,0\n1,1,1\n2,2,1\n2,1,1\n0,2,0\n2,0,1\n"
CSV_B = "x1,x2,y\n0,0,0\n1,1,1\n2,2,1\n0,1,0\n1,0,0\n2,1,1\n0,2,0\n1,2,1\n"


def test_outputs_fedparams_v1():
    out = _run(CSV_A)
    assert out["_format"] == "fedparams-v1"
    assert out["features"] == ["x1", "x2"]
    assert len(out["weights"]) == 2
    assert isinstance(out["intercept"], float)
    assert out["n"] == 8


def test_explicit_feature_order_aligns_parties():
    # Both datasets share schema → same feature order → averageable params.
    a = _run(CSV_A, params={"target": "y", "features": ["x1", "x2"]})
    b = _run(CSV_B, params={"target": "y", "features": ["x1", "x2"]})
    assert a["features"] == b["features"] == ["x1", "x2"]
    assert len(a["weights"]) == len(b["weights"]) == 2


def test_deterministic():
    assert _run(CSV_A)["weights"] == _run(CSV_A)["weights"]


def test_rejects_too_few_rows():
    d = tempfile.mkdtemp()
    data_dir = os.path.join(d, "data")
    os.makedirs(data_dir)
    with open(os.path.join(data_dir, "data.csv"), "w") as f:
        f.write("x1,y\n0,0\n1,1\n")
    r = subprocess.run([sys.executable, os.path.join(HERE, "train.py")],
                       env=dict(os.environ, VO_DATA_DIR=data_dir,
                                VO_OUT_DIR=os.path.join(d, "out"),
                                VO_PARAMS=os.path.join(d, "missing.json")),
                       capture_output=True, text=True)
    assert r.returncode != 0
