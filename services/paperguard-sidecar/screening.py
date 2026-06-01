"""Verdant Oasis · PaperGuard authenticity screening core.

Mirrors PaperGuard's own table-data detector flow (cli._run_detectors_on_file):
load the sheet(s), run the numeric detectors A1/A2/A3/A5/A6/A7/D1/D2, and map
their Findings into the sidecar contract. The paper-specific detectors (image,
full-text, citation-graph, retraction/PubPeer cross-checks) are intentionally
NOT run — only the numeric-forensics subset that doubles as data-quality signal.

PaperGuard imports are lazy (inside functions) so this module — and its pure
scoring/mapping helpers — import and unit-test without PaperGuard installed.
"""
from __future__ import annotations

from pathlib import Path
from typing import Any

# The numeric table-data detectors, exactly as PaperGuard's CLI selects them.
TABLE_DETECTORS = ("A1", "A2", "A3", "A5", "A6", "A7", "D1", "D2")
FDR_ALPHA = 0.05
SEED = 42

# Our severity buckets and their score weights — kept identical to the Go
# baseline (backend quality/authenticity.go) so a 0-100 score means the same
# thing whether the sidecar or the Go fallback produced it.
SEVERITY_WEIGHT = {"info": 0, "low": 4, "medium": 10, "high": 20}

# PaperGuard Severity IntEnum: PASS=0 NOTE=1 CONCERN=2 SUSPICIOUS=3 CRITICAL=4.
_PG_SEVERITY_TO_BUCKET = {4: "high", 3: "medium", 2: "low", 1: "info", 0: "info"}


def bucket_for(pg_severity: int) -> str:
    """Map a PaperGuard severity level to our score bucket."""
    return _PG_SEVERITY_TO_BUCKET.get(int(pg_severity), "info")


def is_significant(finding: Any) -> bool:
    """A finding counts toward the score if FDR-significant, or — when it has no
    p-value — at CONCERN severity or above."""
    padj = getattr(finding, "p_value_adjusted", None)
    if padj is not None:
        return padj < FDR_ALPHA
    return int(getattr(finding, "severity", 0)) >= 2


def map_finding(finding: Any, sheet: str) -> dict:
    """Map a PaperGuard Finding to the sidecar contract. Every signal keeps its
    academic reference and innocent explanations — never a verdict."""
    evidence = getattr(finding, "evidence", {}) or {}
    return {
        "detector": getattr(finding, "detector_id", ""),
        "detector_name": getattr(finding, "detector_name", ""),
        "sheet": sheet,
        "column": evidence.get("column", evidence.get("col", "")),
        "reference": getattr(finding, "academic_reference", ""),
        "summary": getattr(finding, "summary", ""),
        "severity": bucket_for(int(getattr(finding, "severity", 0))),
        "p_value": getattr(finding, "p_value", None),
        "p_value_adjusted": getattr(finding, "p_value_adjusted", None),
        "statistic": getattr(finding, "test_statistic", None),
        "test_name": getattr(finding, "test_name", ""),
        "significant": is_significant(finding),
        "innocent_explanations": list(getattr(finding, "innocent_explanations", []) or []),
    }


def score_findings(findings: list[dict]) -> tuple[int, str]:
    """0-100 authenticity score + clean/review/suspect band, matching the Go
    baseline's weighting and thresholds."""
    score = 100
    for f in findings:
        if f.get("significant"):
            score -= SEVERITY_WEIGHT.get(f.get("severity", "info"), 0)
    score = max(0, min(100, score))
    if score >= 85:
        band = "clean"
    elif score >= 60:
        band = "review"
    else:
        band = "suspect"
    return score, band


def paperguard_version() -> str:
    try:
        from paperguard import __version__

        return __version__
    except Exception:
        return "unknown"


def screen_file(path: Path, suffix: str = ".csv") -> dict:
    """Run the numeric table detectors over a tabular file and return the
    sidecar contract dict. CSV/TSV/XLSX go through PaperGuard's own loader;
    Parquet is read with pandas (PaperGuard's loader doesn't handle it) and fed
    to the same detectors. Lazily imports PaperGuard."""
    from paperguard.core.registry import DetectorRegistry

    registry = DetectorRegistry().register_default(load_plugins=False)
    if suffix == ".parquet":
        import pandas as pd

        sheets = {"data": pd.read_parquet(Path(path))}
    else:
        from paperguard.extractor.excel import parse_data_file

        sheets = dict(parse_data_file(Path(path)))

    findings: list[dict] = []
    errors: list[dict] = []
    rows = 0
    columns: set[str] = set()
    for sheet_name, df in sheets.items():
        rows += len(df)
        for c in df.columns:
            columns.add(str(c))
        for d_id in TABLE_DETECTORS:
            detector = registry.get(d_id)
            if detector is None:
                continue
            try:
                result = detector.detect(df, seed=SEED)
            except Exception as exc:  # one detector failing must not sink the rest
                errors.append({"detector": d_id, "sheet": sheet_name, "error": str(exc)})
                continue
            for f in getattr(result, "findings", []) or []:
                findings.append(map_finding(f, sheet_name))

    score, band = score_findings(findings)
    return {
        "schema_version": "1.0",
        "engine": {"paperguard_version": paperguard_version(), "detectors_run": len(TABLE_DETECTORS)},
        "summary": {
            "authenticity_score": score,
            "band": band,
            "n_findings": len(findings),
            "columns_screened": len(columns),
            "rows": rows,
            "truncated": False,
        },
        "findings": findings,
        "errors": errors,
    }
