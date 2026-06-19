# lca-footprint — Compute-to-Data GHG footprint (GWP)

A Verdant Oasis C2D sandbox algorithm that computes the **greenhouse-gas
footprint** (GWP, kg CO2e) for a dataset of process runs (each row = one run with
activity quantities like electricity, transport, substrate) **inside the
sandbox**, returning only the aggregate footprint. A buyer gets the carbon
footprint of an operation **without seeing the confidential per-run energy/logistics
data**.

The dataset-analyzer reframing of bos-platform's LCA engine
(`GWP = sum(activity × emission_factor)`). numpy/pandas only. Emission factors are
public constants supplied via params (or matched against built-in defaults).

## Output (aggregates only)

```json
{"format":"lca-footprint-1",
 "design":{"activities":{"electricity_kwh":0.5,"transport_km":0.12},"product":"product_kg","impact":"GWP (kg CO2e)","method":"..."},
 "gwp_total_kgco2e": 11200.0,
 "gwp_per_run":{"mean":56.0,"std":...},
 "contribution_by_activity_kgco2e":{"electricity_kwh":...,"transport_km":...},
 "gwp_per_product_unit": 5.6}
```
Only aggregate footprint figures — never the per-run activity data.

## params (`/params.json`)

```json
{"activities":{"electricity_kwh":0.5,"transport_km":0.12},"product":"product_kg"}
```
`activities` maps each activity column to its emission factor. If omitted, numeric
columns are matched against built-in default factors (electricity/transport/etc.).

## Contract & build

Reads only `/data` + `/params.json`; writes only `/out/output.bin`; no network;
deterministic. `PIP_INDEX_URL` build-arg points at a mirror.

```sh
docker build -t vo-lca-footprint:dev .
docker run --rm -i --entrypoint python vo-lca-footprint:dev - < test_lca.py
```
