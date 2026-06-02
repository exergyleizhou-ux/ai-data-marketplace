# dp_stats — differentially-private descriptive statistics (C2D sandbox)

A **trusted whitelist aggregate algorithm**: returns a DP count and a DP mean per
numeric column — only noised aggregates, never raw rows (design §8, §20).

- **Epsilon is platform-injected** (from `offer.dp_epsilon` → `job.dp_epsilon` →
  the runner writes it into `/params.json` as `_epsilon`). The buyer cannot turn
  the noise off. Output kind: `aggregate` (the platform records ε in the DP
  budget ledger).
- Laplace mechanism per query; total ε split evenly across `count + per-column
  means` (sequential composition).
- Sensitivity bounded by **clamping** each column to public bounds (`params.bounds`).
  Without supplied bounds it falls back to observed min/max — data-dependent, so
  NOT a formal DP guarantee; this is reported in `bounds_source`.
- **Not seeded**: DP needs fresh randomness; results differ run-to-run by design.

Params: `{ "_epsilon": <platform>, "columns": ["age", ...]?, "bounds": {"age":[0,120]}? }`.
Output: a zip containing `metrics.json`.

Register as `output_kind=aggregate`, `trusted=true`, with the offer setting
`dp_epsilon` (per-job) and `dp_epsilon_total` (per-buyer ceiling). Test locally:
`~/sdk/sidecar-venv/bin/python algorithms/dp_stats/test_stats.py`.
