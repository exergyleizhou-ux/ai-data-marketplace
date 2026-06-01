"""Pure-logic tests for the screening core — no FastAPI/PaperGuard needed.

Runs on stdlib unittest so it executes anywhere (CI uses pytest, which also
collects unittest.TestCase). The PaperGuard end-to-end path is covered by
test_contract.py, which skips when PaperGuard is not installed.
"""
import sys
import unittest
from pathlib import Path
from types import SimpleNamespace

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from screening import (  # noqa: E402
    bucket_for,
    is_significant,
    map_finding,
    score_findings,
)


def fake_finding(**kw):
    base = dict(
        detector_id="A2",
        detector_name="Benford first digit",
        severity=3,  # SUSPICIOUS
        summary="leading-digit distribution deviates",
        p_value=0.002,
        p_value_adjusted=0.01,
        test_statistic=31.4,
        test_name="chi-square",
        evidence={"column": "yield"},
        innocent_explanations=["single-scale column", "derived ratios", "rounding"],
        academic_reference="Benford 1938",
    )
    base.update(kw)
    return SimpleNamespace(**base)


class TestBuckets(unittest.TestCase):
    def test_severity_mapping(self):
        self.assertEqual(bucket_for(4), "high")
        self.assertEqual(bucket_for(3), "medium")
        self.assertEqual(bucket_for(2), "low")
        self.assertEqual(bucket_for(1), "info")
        self.assertEqual(bucket_for(0), "info")

    def test_significance_uses_fdr_when_present(self):
        self.assertTrue(is_significant(fake_finding(p_value_adjusted=0.01)))
        self.assertFalse(is_significant(fake_finding(p_value_adjusted=0.20)))

    def test_significance_falls_back_to_severity(self):
        self.assertTrue(is_significant(fake_finding(p_value_adjusted=None, severity=2)))
        self.assertFalse(is_significant(fake_finding(p_value_adjusted=None, severity=1)))


class TestMapping(unittest.TestCase):
    def test_map_finding_shape(self):
        m = map_finding(fake_finding(), "Sheet1")
        self.assertEqual(m["detector"], "A2")
        self.assertEqual(m["column"], "yield")
        self.assertEqual(m["severity"], "medium")  # SUSPICIOUS -> medium
        self.assertTrue(m["significant"])
        self.assertEqual(m["reference"], "Benford 1938")
        self.assertEqual(len(m["innocent_explanations"]), 3)


class TestScoring(unittest.TestCase):
    def test_clean_when_no_significant(self):
        f = [{"significant": False, "severity": "high"}]
        self.assertEqual(score_findings(f), (100, "clean"))

    def test_band_thresholds(self):
        # one CRITICAL(high=20) -> 80 -> review
        self.assertEqual(score_findings([{"significant": True, "severity": "high"}]), (80, "review"))
        # two high -> 60 -> review (>=60)
        self.assertEqual(score_findings([{"significant": True, "severity": "high"}] * 2), (60, "review"))
        # three high -> 40 -> suspect
        self.assertEqual(score_findings([{"significant": True, "severity": "high"}] * 3), (40, "suspect"))

    def test_score_clamped_at_zero(self):
        score, band = score_findings([{"significant": True, "severity": "high"}] * 10)
        self.assertEqual(score, 0)
        self.assertEqual(band, "suspect")

    def test_consistency_with_go_weights(self):
        # info 0, low 4, medium 10, high 20 — same as quality/authenticity.go
        mix = [
            {"significant": True, "severity": "low"},
            {"significant": True, "severity": "medium"},
            {"significant": True, "severity": "info"},
        ]
        self.assertEqual(score_findings(mix)[0], 100 - 4 - 10 - 0)


if __name__ == "__main__":
    unittest.main()
