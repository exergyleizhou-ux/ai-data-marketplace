# process-economics — Compute-to-Data batch economics

A Verdant Oasis C2D sandbox algorithm that computes **batch economics** for a
dataset of production batches (each row = one batch, a product mass + its cost
components) **inside the sandbox**, returning only the aggregate indicators
(revenue, cost, margin, unit cost, profitable-batch fraction). A buyer gets the
economics of an operation **without seeing the confidential per-batch cost data**.

The dataset-analyzer reframing of bos-platform's TEA engine: instead of one
project NPV, it aggregates per-batch profitability across a run log. numpy/pandas
only.

## Output (aggregates only)

```json
{"format":"process-economics-1",
 "design":{"product":"product_kg","price_per_unit":2.5,"costs":["substrate_cost","energy_cost","labor_cost"],"method":"..."},
 "revenue":{"mean":25.0,"std":...,"total":...},
 "cost":{"mean":14.0,...}, "margin":{"mean":11.0,...},
 "unit_cost":{"mean":1.4,"std":...},
 "profitable_batch_fraction":1.0,
 "cost_breakdown_total":{"substrate_cost":...,"energy_cost":...,"labor_cost":...}}
```
Only aggregate economics — never per-batch figures.

## params (`/params.json`)

```json
{"product":"product_kg","price":2.5,"costs":["substrate_cost","energy_cost","labor_cost"]}
```
`price` (price per product unit) is required; `product` defaults to the first
numeric column, `costs` to the rest.

## Contract & build

Reads only `/data` + `/params.json`; writes only `/out/output.bin`; no network;
deterministic. `PIP_INDEX_URL` build-arg points at a mirror.

```sh
docker build -t vo-process-economics:dev .
docker run --rm -i --entrypoint python vo-process-economics:dev - < test_economics.py
```
