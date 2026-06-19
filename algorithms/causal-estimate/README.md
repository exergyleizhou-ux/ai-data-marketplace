# causal-estimate — Compute-to-Data average treatment effect (ATE)

A Verdant Oasis C2D sandbox algorithm that estimates the **average treatment
effect** of T on Y (adjusting for covariates) **inside the sandbox** by two
methods — linear OLS and **cross-fitted double-ML** (partialling-out with a
quadratic nuisance basis) — returning only the aggregate estimates. A buyer gets
the causal effect on someone's dataset **without seeing the data**.

The foundation of the causal suite (`mediation` → `sensitivity` → `refutation`
build on it). numpy only — no DoWhy/EconML/sklearn (the DML nuisance is OLS on a
quadratic basis, k-fold cross-fitted; the p-value uses an inline normal CDF).

## Output (aggregates only)

```json
{"format":"causal-estimate-1",
 "design":{"treatment":"dose","outcome":"yield","covariates":["temp"],"method":"..."},
 "estimate":{"ate_ols":1.5,"se":0.02,"t_stat":...,"p_value":0.0,"ci95":[...],"dof":...,
             "ate_dml":1.5,"ate_dml_ci95":[...]},
 "treatment":{"treatment_type":"continuous"}}
```
Binary treatments also report `n_treated`/`n_control`/`unadjusted_diff`. Only the
effect estimates + scalar statistics are emitted — never per-row outputs.

## params (`/params.json`)

```json
{"treatment":"dose","outcome":"yield","covariates":["temp"]}
```
If omitted, the first numeric column is T, the second Y, the rest covariates.

## Contract & build

Reads only `/data` + `/params.json`; writes only `/out/output.bin`; no network;
deterministic. `PIP_INDEX_URL` build-arg points at a mirror.

```sh
docker build -t vo-causal-estimate:dev .
docker run --rm -v "$PWD/test_estimate.py:/app/test_estimate.py:ro" \
    --entrypoint python vo-causal-estimate:dev /app/test_estimate.py
```
