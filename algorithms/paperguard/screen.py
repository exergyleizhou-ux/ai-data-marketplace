#!/usr/bin/env python3
"""PaperGuard data-integrity screen — a Verdant Oasis Compute-to-Data algorithm.

Runs PaperGuard's tabular statistical-anomaly detectors (terminal-digit, Benford,
inter-column arithmetic, decimal consistency, implausible values, last-digit
0/5, ...) on the seller's dataset INSIDE the C2D sandbox and emits an AGGREGATE
integrity verdict — detector-level statistics and severity counts ONLY, never raw
rows, flagged cells, or the detectors' free-text evidence. A third party
(buyer / journal / funder) thereby learns whether a dataset shows statistical
anomalies WITHOUT ever seeing the data: compute-to-data research-integrity
screening.

Container contract (design §7.3, L1 security posture). KEEP THESE:
  * Read ONLY /data; write ONLY /out/output.bin (a zip of model.json +
    metrics.json). No network — the sandbox enforces --network=none, but the
    audited code is the real boundary, so we only use the OFFLINE table detectors
    (the paper-metadata detectors that cross-check OpenAlex/CrossRef/Retraction
    Watch are excluded; they would need network).
  * Return AGGREGATES ONLY — detector-level counts/statistics. NEVER a Finding's
    `evidence`/`detail`/`summary` (those can quote raw flagged values) and never
    per-row output. That is the whole privacy point.
  * Be DETERMINISTIC (fixed seed) so a dispute can be re-computed (§3 / §21).

Wraps the published `paperguard` package (PyPI) as a black box — no PaperGuard
internals are modified.
"""
import io
import json
import os
import sys
import zipfile
from pathlib import Path

DATA_DIR = os.environ.get("VO_DATA_DIR", "/data")
OUT_DIR = os.environ.get("VO_OUT_DIR", "/out")
PARAMS_FILE = os.environ.get("VO_PARAMS", "/params.json")

# Offline statistical-forensics detectors that operate on tabular sheets. These
# are pure-numeric (no network), matching PaperGuard's own per-sheet table flow.
TABLE_DETECTORS = ["A1", "A2", "A3", "A5", "A6", "A7", "D1", "D2"]
SEED = 42  # fixed → deterministic / reproducible for dispute re-computation
# Severity >= CONCERN (PaperGuard: p<0.01 single-detector) counts as a real flag.
CONCERN = 2


def log(stage, **kw):
    print(json.dumps({"stage": stage, **kw}), flush=True)


def die(reason, code=2):
    log("error", reason=reason)
    sys.exit(code)


def load_params():
    if os.path.exists(PARAMS_FILE):
        try:
            with open(PARAMS_FILE) as f:
                return json.load(f) or {}
        except (OSError, ValueError):
            return {}
    return {}


def find_input():
    if not os.path.isdir(DATA_DIR):
        die("no_data_dir")
    names = sorted(os.listdir(DATA_DIR))
    for n in names:
        if n.lower().endswith((".csv", ".tsv", ".xlsx", ".xlsm")):
            return os.path.join(DATA_DIR, n)
    if names:
        return os.path.join(DATA_DIR, names[0])
    die("no_input_file")


def _sev_name(sev):
    return getattr(sev, "name", str(sev))


def _sev_int(sev):
    v = getattr(sev, "value", sev)
    try:
        return int(v)
    except (TypeError, ValueError):
        return 0


def screen(sheets, detector_ids):
    """Run the offline table detectors over every sheet; return aggregates only.

    NEVER emit raw rows, flagged values, evidence dicts, or free-text detail —
    only detector-level statistics + severity counts (L1 §7.3).
    """
    from paperguard.core.registry import DetectorRegistry
    from paperguard.detectors.a1_terminal_digit import A1TerminalDigitDetector
    from paperguard.detectors.a2_benford import A2BenfordDetector
    from paperguard.detectors.a3_arithmetic import A3ArithmeticRelationDetector
    from paperguard.detectors.a5_decimal_consistency import A5DecimalConsistencyDetector
    from paperguard.detectors.a6_implausible_values import A6ImplausibleValueDetector
    from paperguard.detectors.a7_last_digit_five_zero import A7LastDigitFiveZeroDetector
    from paperguard.detectors.d1_residual_smoothness import D1ResidualSmoothnessDetector
    from paperguard.detectors.d2_missing_pattern import D2MissingPatternDetector

    # Register ONLY the 8 offline tabular detectors — NOT register_default(), which
    # hard-imports all 41 detectors (opencv/pymupdf image detectors + their heavy
    # deps) and could fail on a missing optional dependency.
    registry = DetectorRegistry()
    for det_cls in (
        A1TerminalDigitDetector,
        A2BenfordDetector,
        A3ArithmeticRelationDetector,
        A5DecimalConsistencyDetector,
        A6ImplausibleValueDetector,
        A7LastDigitFiveZeroDetector,
        D1ResidualSmoothnessDetector,
        D2MissingPatternDetector,
    ):
        registry.register(det_cls())

    per_detector = {}
    total_findings = 0
    worst = 0
    for _sheet_name, df in sheets.items():
        for did in detector_ids:
            det = registry.get(did)
            if det is None:
                continue
            result = det.detect(df, seed=SEED)
            agg = per_detector.setdefault(
                did,
                {
                    "detector_id": did,
                    "detector_name": getattr(det, "name", did),
                    "applicable": False,
                    "finding_count": 0,
                    "severity_counts": {},
                    "min_p_value": None,
                    "min_p_value_adjusted": None,
                    "test_names": [],
                },
            )
            if getattr(result, "applicable", False):
                agg["applicable"] = True
            for fnd in getattr(result, "findings", []):
                total_findings += 1
                agg["finding_count"] += 1
                sname = _sev_name(fnd.severity)
                agg["severity_counts"][sname] = agg["severity_counts"].get(sname, 0) + 1
                worst = max(worst, _sev_int(fnd.severity))
                if fnd.p_value is not None:
                    agg["min_p_value"] = (
                        fnd.p_value
                        if agg["min_p_value"] is None
                        else min(agg["min_p_value"], fnd.p_value)
                    )
                if fnd.p_value_adjusted is not None:
                    agg["min_p_value_adjusted"] = (
                        fnd.p_value_adjusted
                        if agg["min_p_value_adjusted"] is None
                        else min(agg["min_p_value_adjusted"], fnd.p_value_adjusted)
                    )
                if fnd.test_name and fnd.test_name not in agg["test_names"]:
                    agg["test_names"].append(fnd.test_name)

    detectors = [per_detector[d] for d in detector_ids if d in per_detector]
    n_applicable = sum(1 for d in detectors if d["applicable"])
    n_flagged = sum(1 for d in detectors if d["finding_count"] > 0)
    model = {
        "format": "paperguard-screen-1",
        "detectors": detectors,
        "overall": {
            "n_detectors_run": len(detectors),
            "n_applicable": n_applicable,
            "n_flagged_detectors": n_flagged,
            "total_findings": total_findings,
            "worst_severity": _worst_name(worst) if total_findings else "PASS",
            "verdict": "anomalies_flagged" if worst >= CONCERN else "clean",
        },
    }
    return model


def _worst_name(worst_int):
    names = {0: "PASS", 1: "NOTE", 2: "CONCERN", 3: "SUSPICIOUS", 4: "CRITICAL"}
    return names.get(worst_int, str(worst_int))


def main():
    params = load_params()
    requested = params.get("detectors") or TABLE_DETECTORS
    # Defense-in-depth: only the known OFFLINE table detectors may run, even if a
    # job's params ask for something else (a network detector would hang/fail).
    detector_ids = [d for d in requested if d in TABLE_DETECTORS] or TABLE_DETECTORS

    inp = find_input()
    from paperguard.extractor.excel import parse_data_file

    try:
        sheets = dict(parse_data_file(Path(inp)))
    except Exception:  # noqa: BLE001 — any parse failure is just a bad input
        die("unreadable_input")
    if not sheets:
        die("no_tables_parsed")

    n_rows = sum(int(df.shape[0]) for df in sheets.values())
    n_cols = max((int(df.shape[1]) for df in sheets.values()), default=0)
    log("loaded", sheets=len(sheets), rows=n_rows, cols=n_cols)
    if n_rows < 1:
        die("empty_input")

    model = screen(sheets, detector_ids)
    metrics = {
        "n_sheets": len(sheets),
        "n_rows": n_rows,
        "n_cols": n_cols,
        "n_detectors_run": model["overall"]["n_detectors_run"],
        "n_flagged_detectors": model["overall"]["n_flagged_detectors"],
        "total_findings": model["overall"]["total_findings"],
        "worst_severity": model["overall"]["worst_severity"],
        "verdict": model["overall"]["verdict"],
    }

    os.makedirs(OUT_DIR, exist_ok=True)
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("model.json", json.dumps(model))
        z.writestr("metrics.json", json.dumps(metrics))
    with open(os.path.join(OUT_DIR, "output.bin"), "wb") as f:
        f.write(buf.getvalue())
    log("done", **metrics)


if __name__ == "__main__":
    main()
