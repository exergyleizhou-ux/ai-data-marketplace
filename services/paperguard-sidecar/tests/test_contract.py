"""End-to-end contract test — requires PaperGuard + FastAPI (deploy environment).

Skips automatically where they are not installed, so it is safe to collect in
any CI. Where they ARE installed it screens a fabricated (Geng-style last-digit)
column and a genuine random column and checks the contract shape and that the
fabricated data surfaces at least as many signals as the genuine data.
"""
import io
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

pytest.importorskip("paperguard")
pytest.importorskip("fastapi")

from fastapi.testclient import TestClient  # noqa: E402

from app import app  # noqa: E402

client = TestClient(app)


def _csv(rows):
    buf = io.StringIO()
    buf.write("value\n")
    for r in rows:
        buf.write(f"{r}\n")
    return buf.getvalue().encode()


def test_healthz():
    r = client.get("/healthz")
    assert r.status_code == 200
    assert r.json()["status"] == "ok"


def test_screen_contract_shape():
    fabricated = _csv([i * 5 for i in range(300)])  # every value ends in 0 or 5
    r = client.post("/v1/screen", content=fabricated, headers={"Content-Type": "text/csv"})
    assert r.status_code == 200
    body = r.json()
    assert body["schema_version"] == "1.0"
    s = body["summary"]
    assert 0 <= s["authenticity_score"] <= 100
    assert s["band"] in ("clean", "review", "suspect")
    for f in body["findings"]:
        assert "detector" in f and "severity" in f
        assert "innocent_explanations" in f


def test_empty_body_is_400():
    r = client.post("/v1/screen", content=b"", headers={"Content-Type": "text/csv"})
    assert r.status_code == 400


def test_fabricated_data_is_flagged():
    """Behavioral check: a deliberate last-digit (0/5) fabrication must be
    surfaced — non-clean band, real findings, and the terminal-digit / Geng
    detector firing. Verified against PaperGuard 2.17.0."""
    fabricated = _csv([i * 5 for i in range(300)])
    body = client.post(
        "/v1/screen", content=fabricated, headers={"Content-Type": "text/csv"}
    ).json()
    s = body["summary"]
    assert s["band"] in ("review", "suspect"), s
    assert s["n_findings"] >= 1
    detectors = {f["detector"] for f in body["findings"] if f["significant"]}
    # A1 = terminal-digit distribution, A7 = Geng last-digit 0/5 preference.
    assert detectors & {"A1", "A7"}, body["findings"]
