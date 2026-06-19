# paperguard — Compute-to-Data research-data integrity screen

A Verdant Oasis C2D sandbox algorithm that runs **[PaperGuard](https://github.com/exergyleizhou-ux/PaperGuard)**'s
tabular statistical-anomaly detectors on a seller's dataset **inside the sandbox**
and returns an **aggregate integrity verdict** — never the raw rows.

A buyer / journal / funder thereby learns *whether a dataset shows statistical
anomalies* (Benford deviation, terminal-digit irregularity, arithmetic
inconsistency, implausible values, decimal/last-digit patterns, residual
smoothness, missing-data structure) **without ever seeing the data** — research
integrity screening done compute-to-data, with a signed result certificate.

## What it computes

Runs PaperGuard's 8 **offline** tabular detectors — `A1` terminal-digit,
`A2` Benford, `A3` inter-column arithmetic, `A5` decimal consistency,
`A6` implausible values, `A7` last-digit 0/5, `D1` residual smoothness,
`D2` missing-pattern — over every sheet of the dataset.

It deliberately **excludes** PaperGuard's paper-metadata detectors (which
cross-check OpenAlex / CrossRef / Retraction Watch) because they need network,
and the sandbox runs `--network=none`.

## Output (aggregates only — the privacy guarantee)

`/out/output.bin` is a zip of `model.json` + `metrics.json`. `model.json`:

```json
{
  "format": "paperguard-screen-1",
  "detectors": [
    {"detector_id": "A2", "detector_name": "...", "applicable": true,
     "finding_count": 1, "severity_counts": {"CONCERN": 1},
     "min_p_value": 0.004, "min_p_value_adjusted": 0.012, "test_names": ["chi2"]}
  ],
  "overall": {"n_detectors_run": 8, "n_applicable": 5, "n_flagged_detectors": 1,
              "total_findings": 1, "worst_severity": "CONCERN", "verdict": "anomalies_flagged"}
}
```

Only **detector-level statistics + severity counts** are emitted. A `Finding`'s
`evidence` / `detail` / `summary` (which can quote raw flagged values) and any
per-row output are **never** included — that is the whole point of running it
compute-to-data. A unit test (`test_screen.py`) plants a sentinel value and
asserts it never appears in the output.

## Contract & posture (design §7.3, L1)

- Reads only `/data` (first `.csv`/`.tsv`/`.xlsx`), optional `/params.json`;
  writes only `/out/output.bin`. No network.
- Deterministic (fixed seed) so a dispute can be re-computed.
- Wraps the **published** `paperguard` PyPI package as a black box — no PaperGuard
  internals are modified. Only its light core deps (pydantic/numpy/scipy/pandas/
  openpyxl) are installed; the image/PDF detectors and their heavy deps are never
  imported.

### params (optional, `/params.json`)

- `detectors`: subset of `["A1","A2","A3","A5","A6","A7","D1","D2"]` to run
  (defaults to all 8; anything outside this offline set is ignored).

## Build & pin by digest

```sh
docker build -t <registry>/vo-paperguard:1 .
docker push <registry>/vo-paperguard:1
# register on Oasis digest-pinned, exactly like algorithms/logreg — see that README
# and algorithms/publish.sh.
```

## Test

```sh
docker build -t vo-paperguard:dev .
docker run --rm -v "$PWD/test_screen.py:/app/test_screen.py:ro" \
    --entrypoint python vo-paperguard:dev /app/test_screen.py
```
