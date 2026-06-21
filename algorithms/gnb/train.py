#!/usr/bin/env python3
"""Gaussian Naive Bayes classifier for the Verdant Oasis Compute-to-Data sandbox.

A TRUSTED whitelist algorithm (design §2 / §7.3): on an L1 offer the audited
code — not the sandbox — is what keeps raw rows from leaking. Mirrors the
security posture of the kmeans/logreg algorithms:

  * Reads ONLY from the data dir; writes ONLY to the out dir. No network.
  * Emits ONLY structured progress logs (counts/metrics) — never raw rows (§7.4).
  * Output is a JSON model, NOT a pickle, so it can never be a deserialization-RCE
    vector for the buyer (§7.4).
  * Returns AGGREGATES ONLY — per-class per-feature statistics. It NEVER emits
    per-row predictions (which would be high-fidelity per-row leakage, §7.3).
  * Deterministic — closed-form MLE, no randomness — so a job can be reproduced
    for dispute re-computation (§3 / §21).

Container contract: read /data (the dataset, read-only), write /out/output.bin
(a zip of model.json + metrics.json). Paths overridable via env for testing.
Optional /params.json: { "label": "y", "features": ["col1", "col2"] }.
"""
import csv
import io
import json
import math
import os
import sys
import zipfile

DATA_DIR = os.environ.get("VO_DATA_DIR", "/data")
OUT_DIR = os.environ.get("VO_OUT_DIR", "/out")
PARAMS_FILE = os.environ.get("VO_PARAMS", "/params.json")


def log(stage, **kw):
    """Structured progress log — counts/metrics only, never raw data (§7.4)."""
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
        if n.lower().endswith((".csv", ".tsv")):
            return os.path.join(DATA_DIR, n)
    if names:
        return os.path.join(DATA_DIR, names[0])
    die("no_input_file")


def load_data(path):
    """Load CSV/TSV, return (header, list_of_rows_as_lists)."""
    sep = "\t" if path.lower().endswith(".tsv") else ","
    with open(path, newline="") as f:
        reader = csv.reader(f, delimiter=sep)
        rows = list(reader)
    if not rows:
        die("empty_file")
    return rows[0], rows[1:]


def is_float(s):
    try:
        float(s)
        return True
    except (ValueError, TypeError):
        return False


def logpdf_normal(x, mu, var):
    """Log of the univariate normal PDF at x given mu and variance var."""
    return -0.5 * math.log(2.0 * math.pi * var) - (x - mu) ** 2 / (2.0 * var)


def main():
    params = load_params()
    inp = find_input()
    header, raw_rows = load_data(inp)
    log("loaded", rows=len(raw_rows), cols=len(header))

    col_count = len(header)
    label_col = params.get("label", "label")

    # Locate the label column index.
    if label_col not in header:
        die("label_column_not_found")
    label_idx = header.index(label_col)

    # Feature columns: explicit list from params, or auto-detect.
    want_features = params.get("features")
    if isinstance(want_features, list) and want_features:
        # Validate requested features exist and are not the label.
        feat_indices = []
        feat_names = []
        for f in want_features:
            if f == label_col:
                continue
            if f in header:
                feat_indices.append(header.index(f))
                feat_names.append(f)
        if not feat_indices:
            die("no_valid_features")
    else:
        # Auto-detect: every column whose name != label_col and whose cells
        # all parse as float (across all rows).
        feat_indices = []
        feat_names = []
        for ci, col_name in enumerate(header):
            if ci == label_idx:
                continue
            # Check all rows for this column parse as float.
            all_float = True
            for r in raw_rows:
                if ci >= len(r) or not is_float(r[ci]):
                    all_float = False
                    break
            if all_float:
                feat_indices.append(ci)
                feat_names.append(col_name)

    if not feat_indices:
        die("no_numeric_features")

    n_features = len(feat_indices)
    log("features", count=n_features, names=feat_names)

    # Parse rows: keep label (as string) and feature values (as float).
    # Skip rows where any feature value is missing or non-numeric.
    X = []  # list of list[float]
    y = []  # list of str labels
    skipped = 0
    for r in raw_rows:
        # Label must exist.
        if label_idx >= len(r) or r[label_idx] == "":
            skipped += 1
            continue
        label = r[label_idx]
        feat_vals = []
        ok = True
        for fi in feat_indices:
            if fi >= len(r):
                ok = False
                break
            val = r[fi]
            if val == "" or not is_float(val):
                ok = False
                break
            feat_vals.append(float(val))
        if not ok:
            skipped += 1
            continue
        X.append(feat_vals)
        y.append(label)

    if skipped:
        log("skipped_rows", count=skipped)

    n_train = len(y)
    if n_train < 1:
        die("no_complete_rows")

    # Determine classes (sorted ascending as strings).
    classes = sorted(set(y))
    n_classes = len(classes)
    # Privacy guard (L1: the audited code is the boundary). A classification label
    # must be low-cardinality; if `label` points at an id/email/free-text column,
    # every distinct value would otherwise leak into the model as a "class". Refuse
    # rather than emit per-row identifiers.
    max_classes = int(params.get("max_classes", 100))
    if n_classes > max_classes or n_classes * 2 > n_train:
        die("too_many_classes")
    log("classes", count=n_classes)  # count only — never echo the class values

    # Group feature values by class.
    # class_values[c][j] = list of feature j values for class c.
    class_indices = {c: [] for c in classes}
    for i, label in enumerate(y):
        class_indices[label].append(i)

    # Privacy guard (L1: the audited code IS the boundary). A class's per-class mean
    # `theta` over a SINGLE row IS that row, verbatim — a direct raw-record leak. The
    # earlier guard bounds the class *count*; this bounds each class's *size*. Default
    # floor 2 blocks the verbatim singleton leak while still allowing legitimately
    # small research classes (a mean over ≥2 rows is an average, not a record); raise
    # `min_class_count` for stronger k-anonymity on sensitive data.
    min_class_count = int(params.get("min_class_count", 2))
    if min(len(ix) for ix in class_indices.values()) < min_class_count:
        die("class_too_small")

    # Compute theta (mean), var (population variance + epsilon), priors.
    theta = {}
    var = {}
    priors = {}
    class_counts = {}

    for c in classes:
        indices = class_indices[c]
        count = len(indices)
        class_counts[c] = count
        priors[c] = count / n_train

        theta_c = []
        var_c = []
        for j in range(n_features):
            vals = [X[i][j] for i in indices]
            mean_j = sum(vals) / count
            # Population variance (ddof=0): sum of squared deviations / count.
            var_j = sum((v - mean_j) ** 2 for v in vals) / count
            theta_c.append(mean_j)
            var_c.append(var_j)
        theta[c] = theta_c
        var[c] = var_c

    # Training accuracy: predict each training row.
    n_correct = 0
    for i in range(n_train):
        x_i = X[i]
        best_log_prob = -float("inf")
        best_class = classes[0]
        for c in classes:
            log_prob = math.log(priors[c])
            for j in range(n_features):
                v = var[c][j] + 1e-9  # apply epsilon during prediction
                log_prob += logpdf_normal(x_i[j], theta[c][j], v)
            if log_prob > best_log_prob:
                best_log_prob = log_prob
                best_class = c
            # Tie-break: first class in sorted order wins (already handled
            # because we only update on strictly greater).
        if best_class == y[i]:
            n_correct += 1

    accuracy = n_correct / n_train if n_train > 0 else 0.0

    # Apply var_epsilon to stored variance.
    var_with_eps = {}
    for c in classes:
        var_with_eps[c] = [vj + 1e-9 for vj in var[c]]

    model = {
        "format": "vo-gnb-1",
        "features": feat_names,
        "classes": classes,
        "priors": {c: priors[c] for c in classes},
        "theta": {c: theta[c] for c in classes},
        "var": {c: var_with_eps[c] for c in classes},
        "var_epsilon": 1e-9,
    }
    metrics = {
        "format": "vo-gnb-1",
        "n_train": n_train,
        "n_features": n_features,
        "classes": classes,
        "class_counts": {c: class_counts[c] for c in classes},
        "accuracy": accuracy,
        "n_correct": n_correct,
    }

    log("fitted", n_train=n_train, n_features=n_features, accuracy=round(accuracy, 6))

    # Write output.
    os.makedirs(OUT_DIR, exist_ok=True)
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("model.json", json.dumps(model))
        z.writestr("metrics.json", json.dumps(metrics))
    with open(os.path.join(OUT_DIR, "output.bin"), "wb") as f:
        f.write(buf.getvalue())

    log("done", n_train=n_train, accuracy=round(accuracy, 6))


if __name__ == "__main__":
    main()
