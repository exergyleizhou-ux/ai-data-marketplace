# mass-balance — Compute-to-Data process mass closure

A Verdant Oasis C2D sandbox algorithm that checks **mass closure** for a dataset
of bioconversion / process runs (each row = one run, an input mass + its output
fractions) **inside the sandbox**, returning only aggregate closure statistics. A
buyer learns whether a process's mass balances on someone's run log **without
seeing the runs**.

The C2D-portable core of bos-platform's mass-balance engine (`epsilon` = closure
residual). numpy/pandas only.

```
closure_i  = sum(outputs_i) / input_i     # fraction of input accounted for
residual_i = 1 − closure_i  (= epsilon)   # unaccounted fraction
```

## Output (aggregates only)

```json
{"format":"mass-balance-1",
 "design":{"input":"feed_g","outputs":["product_g","residue_g","loss_g"],"tolerance":0.05,"method":"..."},
 "closure":{"mean":0.97,"std":0.01,"min":...,"max":...},
 "residual_epsilon":{"mean":0.03,...},
 "within_tolerance_fraction":0.95,
 "mean_output_fraction":{"product_g":0.44,"residue_g":0.39,"loss_g":0.15}}
```
Only aggregate closure/residual statistics + mean per-output fractions — never the
per-run masses.

## params (`/params.json`)

```json
{"input":"feed_g","outputs":["product_g","residue_g","loss_g"],"tolerance":0.05}
```
If omitted, the first numeric column is the input, the rest are outputs.

## Contract & build

Reads only `/data` + `/params.json`; writes only `/out/output.bin`; no network;
deterministic. `PIP_INDEX_URL` build-arg points at a mirror.

```sh
docker build -t vo-mass-balance:dev .
docker run --rm -v "$PWD/test_massbalance.py:/app/test_massbalance.py:ro" \
    --entrypoint python vo-mass-balance:dev /app/test_massbalance.py
```
