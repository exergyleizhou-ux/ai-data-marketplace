# causal-refutation — Compute-to-Data effect validity checks

A Verdant Oasis C2D sandbox algorithm that **stress-tests a linear treatment
effect** with three standard refuters — inside the sandbox, returning only the
aggregate refutation verdict. A buyer learns whether an effect **survives
validity checks** on someone's dataset **without ever seeing the data**.

The **C2D-portable core of [bos-platform](https://github.com/exergyleizhou-ux/bos-platform)'s
`causal_refute_engine`** (which wraps DoWhy's mandatory refuters). bos calls a
finding *validated* when the refuters pass; this port reproduces the same three
checks with numpy OLS — no DoWhy:

| refuter | what it does | passes when |
| --- | --- | --- |
| **placebo treatment** | permute the treatment, re-estimate | effect collapses to ~0 |
| **random common cause** | add a random covariate, re-estimate | effect stays stable |
| **data subset** | re-estimate on random 80% subsets | effect stays stable |

`evidence_level = "validated"` iff all three pass.

## Output (aggregates only)

`/out/output.bin` = zip(`model.json`, `metrics.json`):

```json
{"format":"causal-refutation-1",
 "design":{"treatment":"dose","outcome":"yield","covariates":[], "method":"..."},
 "original_effect": 2.0,
 "refuters":[
   {"name":"placebo_treatment","placebo_effect_mean":0.001,"permutation_p_value":0.0,"passed":true},
   {"name":"random_common_cause","new_effect_mean":2.0,"relative_change":0.0008,"passed":true},
   {"name":"data_subset","new_effect_mean":1.99,"new_effect_std":0.04,"relative_change":0.004,"passed":true}],
 "evidence_level":"validated"}
```
Only the effect estimate + per-refuter scalar results are emitted — never per-row
outputs or raw values.

## params (`/params.json`)

```json
{"treatment": "dose", "outcome": "yield", "covariates": ["temp"]}
```
If omitted, the first numeric column is T, the second is Y, the rest are covariates.

## Contract & build

Reads only `/data` + `/params.json`; writes only `/out/output.bin`; no network;
deterministic (fixed seed). `PIP_INDEX_URL` build-arg points at a mirror.

```sh
docker build -t vo-causal-refutation:dev .
docker run --rm -v "$PWD/test_refute.py:/app/test_refute.py:ro" \
    --entrypoint python vo-causal-refutation:dev /app/test_refute.py
```
