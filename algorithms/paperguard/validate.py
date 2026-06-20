#!/usr/bin/env python3
"""Differential operating-characteristic validation for the PaperGuard screen.

Measures whether the PaperGuard data-integrity screen (algorithms/paperguard/
screen.py) RESPONDS to injected, labelled statistical anomalies — the families
its offline tabular detectors target. For each anomaly type we build N PAIRED
datasets: a synthetic base, and the SAME base with one anomaly injected. We run
the exact production screen on both and ask whether the targeted detector's
signal STRENGTHENS with the injection (higher severity, more findings, or a
smaller min p-value). The paired design controls for the synthetic baseline, so
the result is a valid causal statement: "the screen detects this anomaly above
its own noise floor."

WHY PAIRED / WHY NOT A RAW FPR — read before quoting numbers. A raw
false-positive rate needs a valid NEGATIVE CONTROL: real clean data. Pure
numpy-RNG data is NOT a valid negative control for digit-distribution tests
(A1 terminal-digit, A7 last-digit-0/5): they fire on synthetic floats because the
RNG's digits don't satisfy their null model. So an absolute specificity/FPR
measured on synthetic negatives would be meaningless (and dishonest). We instead
report (1) per-anomaly SENSITIVITY (does the screen's binary verdict flag the
injected dataset) and (2) the paired DIFFERENTIAL RESPONSE (does the targeted
detector strengthen vs its own clean baseline). A real-world FPR is a Part-B item
gated on a real clean-data partner. Deterministic (fixed seed) → reproducible.

Run inside the production image (paperguard 2.17.0 + py3.11):

    docker run --rm --network=none -v "$PWD/algorithms/paperguard:/app" -w /app \
        --entrypoint python vo-paperguard:dev validate.py
"""
import json
import sys

import numpy as np
import pandas as pd

from screen import TABLE_DETECTORS, screen

N_PAIRS = 30
ROWS = 200
BASE_SEED = 20260620
SEV = {"PASS": 0, "NOTE": 1, "CONCERN": 2, "SUSPICIOUS": 3, "CRITICAL": 4}


def clean_df(rng):
    """A synthetic base: natural continuous measurements (NOT a clean control —
    see the module docstring; it is the shared baseline for the paired design)."""
    return pd.DataFrame(
        {
            "conc_mg_l": np.round(rng.lognormal(3.0, 1.1, ROWS), 3),
            "score": np.round(rng.normal(50, 14, ROWS), 2),
            "duration_s": np.round(rng.exponential(18, ROWS), 2),
            "mass_g": np.round(rng.lognormal(2.0, 0.9, ROWS), 3),
        }
    )


# --- injections, each matched to its target detector's actual trigger ---
def inj_digit_heaping(df, rng):  # A1 / A7: round most values to end in 0/5
    col = df["conc_mg_l"].to_numpy(copy=True)
    idx = rng.choice(len(col), int(0.7 * len(col)), replace=False)
    col[idx] = np.round(col[idx] / 5.0) * 5.0
    df["conc_mg_l"] = col
    return df


def inj_benford(df, rng):  # A2: leading digits forced away from Benford (8/9)
    df["mass_g"] = np.round(rng.uniform(8.0, 9.99, ROWS) * 10 ** rng.integers(0, 3, ROWS), 3)
    return df


def inj_implausible(df, rng):  # A6: a percentage column with impossible values
    pct = rng.uniform(0, 100, ROWS)
    bad = rng.choice(ROWS, int(0.15 * ROWS), replace=False)
    pct[bad] = rng.choice([150.0, 250.0, -20.0, 999.0], size=len(bad))
    df["percentage"] = np.round(pct, 1)
    return df


def inj_decimal_overconsistency(df, rng):  # A5: a dominant fractional part (".50")
    base = df["score"].to_numpy(copy=True)
    out = np.floor(base) + 0.50
    keep = rng.choice(len(out), int(0.25 * len(out)), replace=False)  # leave some varied
    out[keep] = np.round(base[keep], 2)
    df["score"] = out
    return df


def inj_smooth_residual(df, rng):  # D1: a too-smooth fabricated linear relation
    x = np.linspace(1, 100, ROWS)
    df["x_dose"] = np.round(x, 3)
    df["y_response"] = np.round(2.0 * x + 3.0 + rng.normal(0, 0.01, ROWS), 3)
    return df


def inj_uniform_variance(df, rng):  # D2: "too clean" — >=5 cols with near-identical std
    cols = {}
    for j in range(6):
        x = rng.normal(0, 1, ROWS)
        x = (x - x.mean()) / x.std(ddof=1) * 10.0 + 100.0  # std ~ 10 for every column
        cols[f"m{j + 1}"] = np.round(x, 3)
    return pd.DataFrame(cols)  # replaces the base with suspiciously-uniform-variance data


FRAUD = {
    "digit_heaping": (inj_digit_heaping, {"A1", "A7"}),
    "benford_violation": (inj_benford, {"A2"}),
    "implausible_values": (inj_implausible, {"A6"}),
    "decimal_overconsistency": (inj_decimal_overconsistency, {"A5"}),
    "smooth_residual": (inj_smooth_residual, {"D1"}),
    "uniform_variance": (inj_uniform_variance, {"D2"}),
}


def signals(df):
    """Run the production screen; return (verdict_positive, per-detector signal)."""
    m = screen({"sheet": df}, TABLE_DETECTORS)
    pos = m["overall"]["verdict"] == "anomalies_flagged"
    per = {}
    for d in m["detectors"]:
        worst = max((SEV.get(s, 0) for s in d["severity_counts"]), default=0)
        per[d["detector_id"]] = (worst, d["finding_count"], d["min_p_value"])
    return pos, per


def strengthened(base, inj, targets):
    for d in targets:
        bw, bc, bp = base.get(d, (0, 0, None))
        aw, ac, ap = inj.get(d, (0, 0, None))
        if aw > bw or ac > bc:
            return True
        if ap is not None and (bp is None or ap < bp):
            return True
    return False


def main():
    per_type = {}
    for t_i, (name, (fn, targets)) in enumerate(FRAUD.items()):
        flagged = response = 0
        for i in range(N_PAIRS):
            seed = BASE_SEED + 1000 * (t_i + 1) + i
            base_pos, base_sig = signals(clean_df(np.random.default_rng(seed)))
            inj_pos, inj_sig = signals(fn(clean_df(np.random.default_rng(seed)), np.random.default_rng(seed)))
            if inj_pos:
                flagged += 1
            if strengthened(base_sig, inj_sig, targets):
                response += 1
        per_type[name] = {
            "target_detectors": sorted(targets),
            "sensitivity": round(flagged / N_PAIRS, 3),
            "differential_response_rate": round(response / N_PAIRS, 3),
        }

    overall_sens = round(sum(v["sensitivity"] for v in per_type.values()) / len(per_type), 3)
    overall_resp = round(sum(v["differential_response_rate"] for v in per_type.values()) / len(per_type), 3)
    result = {
        "method": "paired (base vs base+injected-anomaly), production screen(); differential response of the targeted detector",
        "n_pairs_per_type": N_PAIRS,
        "rows_per_dataset": ROWS,
        "seed": BASE_SEED,
        "per_type": per_type,
        "overall_sensitivity": overall_sens,
        "overall_differential_response_rate": overall_resp,
        "fpr_specificity": "NOT measured — synthetic RNG data is not a valid negative control for the "
        "digit-distribution detectors (A1/A7); a real-world FPR requires a real clean-data partner (Part B).",
        "interpretation": "The screen detects each injected anomaly family above its own synthetic baseline. "
        "The binary verdict is conservative ('warrants review'), not a fraud determination.",
    }
    json.dump(result, sys.stdout, indent=2)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
