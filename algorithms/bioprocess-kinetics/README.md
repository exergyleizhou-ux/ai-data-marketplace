# bioprocess-kinetics — Compute-to-Data growth-curve fitting

A Verdant Oasis C2D sandbox algorithm that fits **microbial / larval growth-curve
models** (Logistic and modified Gompertz) to a biomass time-series — inside the
sandbox, returning only the aggregate **fitted parameters + goodness-of-fit**. A
buyer gets the growth kinetics of a fermentation / bioconversion run **without
seeing the raw measurements**.

The **C2D-portable core of [bos-platform](https://github.com/exergyleizhou-ux/bos-platform)'s
`kinetics_engine`** (which models insect / microbial growth via Monod / Logistic
/ Gompertz / Baranyi-Roberts). This port keeps the two closed-form sigmoids —
bos's exact Logistic `B(t)=K/(1+((K−B0)/B0)e^{−rt})` and the Zwietering modified
Gompertz `B(t)=A·exp(−exp((μ_m·e/A)(λ−t)+1))` — fit with `scipy` least squares,
so no ODE solver and a small audited surface.

## Output (aggregates only)

`/out/output.bin` = zip(`model.json`, `metrics.json`):

```json
{"format":"bioprocess-kinetics-1",
 "design":{"time":"hour","value":"biomass","models":["logistic","gompertz"], "method":"..."},
 "fits":[{"model":"logistic","converged":true,"params":[B0,K,r],"r2":0.999,"rmse":...},
         {"model":"gompertz","converged":true,"params":[A,mu_m,lam],"r2":0.998,"rmse":...}],
 "best_model":"logistic","best_r2":0.999,
 "derived":{"carrying_capacity":10.0,"growth_rate_r":0.8,"max_dB_dt":2.0,"doubling_time":0.866}}
```
Only fitted parameters + scalar fit statistics + derived growth indices are
emitted — never the per-row fitted curve or the raw measurements.

## params (`/params.json`)

```json
{"time": "hour", "value": "biomass"}
```
If omitted, the first numeric column is time, the second is the measurement.

## Contract & build

Reads only `/data` + `/params.json`; writes only `/out/output.bin`; no network;
deterministic. `PIP_INDEX_URL` build-arg points at a mirror.

```sh
docker build -t vo-bioprocess-kinetics:dev .
docker run --rm -v "$PWD/test_kinetics.py:/app/test_kinetics.py:ro" \
    --entrypoint python vo-bioprocess-kinetics:dev /app/test_kinetics.py
```
