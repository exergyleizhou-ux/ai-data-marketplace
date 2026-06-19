# causal-sensitivity — Compute-to-Data robustness value (Cinelli-Hazlett)

A Verdant Oasis C2D sandbox algorithm that quantifies **how robust a linear
treatment effect is to unobserved confounding** — inside the sandbox, returning
only the aggregate sensitivity statistics. A buyer learns *how trustworthy a
finding is* on someone's dataset **without ever seeing the data**.

This is the **C2D-portable core of [bos-platform](https://github.com/exergyleizhou-ux/bos-platform)'s
`"linear"` sensitivity branch**: the closed-form **Cinelli & Hazlett (2020)
robustness value + partial R²** from a single OLS fit (`Y ~ T + covariates`).
numpy only — no DoWhy/statsmodels/scipy (the normal critical value uses an
inline Acklam inverse-CDF), so the audited image stays tiny.

```
partial_r2_yd = t² / (t² + dof)
RV_q          = ½(√(f_q⁴ + 4f_q²) − f_q²),   f_q = q·|t|/√dof
```
`RV_q` is the partial R² an unobserved confounder must share with **both**
treatment and outcome to reduce the estimate by `100·q`% (q=1 → to zero).
Conventionally **robust** when `RV_q > 0.10`.

## Output (aggregates only)

`/out/output.bin` = zip(`model.json`, `metrics.json`):

```json
{"format":"causal-sensitivity-1",
 "design":{"treatment":"dose","outcome":"yield_mg","covariates":[],"q":1.0,"alpha":0.05,
           "method":"Cinelli-Hazlett 2020 robustness value (linear)"},
 "estimate":{"coef":..., "se":..., "t_stat":..., "dof":..., "partial_r2_treatment_outcome":...},
 "sensitivity":{"robustness_value":..., "robustness_value_ci":..., "robust_threshold":0.10, "robust":true},
 "interpretation":"An unobserved confounder would need to explain more than X% ..."}
```
Only coefficients + sensitivity statistics — never per-row outputs or raw values.

## params (`/params.json`)

```json
{"treatment":"dose","outcome":"yield_mg","covariates":["temp"],"q":1.0,"alpha":0.05}
```
If omitted, the first numeric column is T, the second is Y, the rest are covariates.

## Contract & build

Reads only `/data` + `/params.json`; writes only `/out/output.bin`; no network;
deterministic. `PIP_INDEX_URL` build-arg points at a mirror.

```sh
docker build -t vo-causal-sensitivity:dev .
docker run --rm -v "$PWD/test_sensitivity.py:/app/test_sensitivity.py:ro" \
    --entrypoint python vo-causal-sensitivity:dev /app/test_sensitivity.py
```
