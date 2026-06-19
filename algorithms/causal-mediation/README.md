# causal-mediation — Compute-to-Data Pearl NDE/NIE mediation

A Verdant Oasis C2D sandbox algorithm that estimates a **causal mediation
decomposition** — natural direct effect (NDE), natural indirect effect (NIE),
total effect (ATE) and proportion-mediated for a single-mediator linear model
T → M → Y — **inside the sandbox**, returning only the aggregate effect estimates
(with bootstrap CIs). A buyer learns *how an effect is mediated* in someone's
dataset **without ever seeing the data**.

This is the **C2D-portable core of the [bos-platform](https://github.com/exergyleizhou-ux/bos-platform)
causal layer**: bos uses DoWhy's `mediation.two_stage_regression`, which for the
linear single-mediator case reduces to exactly this (Pearl natural effects:
`NDE = β_T`, `NIE = α_T·β_M`, with `NDE + NIE == ATE`). Implemented with
numpy/pandas only, so the sandbox image stays tiny and the audited surface small
— no DoWhy / EconML / PyMC.

## Output (aggregates only)

`/out/output.bin` = zip(`model.json`, `metrics.json`). `model.json`:

```json
{
  "format": "causal-mediation-1",
  "design": {"treatment":"T","mediator":"M","outcome":"Y","covariates":[],
             "method":"linear two-stage regression (Pearl NDE/NIE)"},
  "effects": {"nde":3.0,"nie":8.0,"ate":11.0,"proportion_mediated":0.727,
              "nde_ci95":[...],"nie_ci95":[...],"ate_ci95":[...],"proportion_mediated_ci95":[...]},
  "pearl_invariant_residual": 0.0
}
```

Only effect estimates + bootstrap CIs are emitted — never per-row outputs or raw
values. `pearl_invariant_residual` (= |ATE − (NDE+NIE)|, ~0 by construction)
demonstrates the Pearl identity holds.

## params (`/params.json`)

```json
{"treatment": "dose", "mediator": "od600", "outcome": "yield_mg", "covariates": ["temp"]}
```
If omitted, the first three numeric columns are used as T, M, Y (runs out of box).

## Contract & build

Reads only `/data` (first `.csv`/`.tsv`) + `/params.json`; writes only
`/out/output.bin`; no network; deterministic (fixed seed). Build & pin by digest
like `algorithms/logreg`; `PIP_INDEX_URL` build-arg points at a mirror.

```sh
docker build -t vo-causal-mediation:dev .
docker run --rm -v "$PWD/test_mediate.py:/app/test_mediate.py:ro" \
    --entrypoint python vo-causal-mediation:dev /app/test_mediate.py
```
