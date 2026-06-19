# Deploying the `paperguard` C2D algorithm on a live Oasis

The image is built and pushed to the local registry, **digest-pinned**:

```
127.0.0.1:5000/vo-paperguard@sha256:46ca9a23e080ca2bdf4ba010b400341ecc30b587f3b72810196f7c2ed4692eb3
```

Rebuild + repush any time with the generic publisher:

```sh
algorithms/publish-local.sh paperguard "PaperGuard 数据完整性筛查"
```

## 1) Register (as ops) — one command

```sh
OPS_TOKEN=<ops-jwt> API=http://localhost:8080/api/v1 \
  algorithms/publish-local.sh paperguard "PaperGuard 数据完整性筛查"
```

…which POSTs `/admin/compute/algorithms` (digest-pinned) then
`/admin/compute/algorithms/{id}/review {"status":"approved","trusted":true}`.
Trust is REQUIRED because `output_kind=model` is an L1 output (design §4/§7.3).

Manual payload:

```json
POST /admin/compute/algorithms      (ops bearer)
{
  "name": "PaperGuard 数据完整性筛查",
  "runtime": "python-sklearn",
  "image": "127.0.0.1:5000/vo-paperguard",
  "image_digest": "sha256:46ca9a23e080ca2bdf4ba010b400341ecc30b587f3b72810196f7c2ed4692eb3",
  "source_ref": "github.com/exergyleizhou-ux/ai-data-marketplace/tree/main/algorithms/paperguard",
  "output_kind": "model",
  "version": 1
}
POST /admin/compute/algorithms/{id}/review  {"status":"approved","trusted":true}
```

## 2) End-to-end → certificate

Mirror `scripts/demo-kmeans-up.sh` (the proven kmeans loop), swapping the algo id:

1. **ops**: register + approve + trust (above).
2. **seller**: add the algorithm id to the tabular dataset's compute offer `allowed_algorithm_ids`.
3. **buyer**: grant/purchase entitlement → `POST /compute/jobs` with this algo over the dataset.
4. job runs in the real `--network=none --read-only` sandbox → `output_reviewing`.
5. **ops**: `POST /admin/compute/jobs/{id}/release` → **certificate `VO-…`** (public lookup `/verify/{cert}`).

**Prereq** (the only thing missing today): a seeded ops/seller/buyer + a published
tabular dataset. Re-seed per the live-Oasis runbook — demo-ops/seller/buyer
@ `oasis.test`, pw `Oasis1234!`; a published dataset whose CSV sits under
`backend/data/storage`. The algorithm itself is already verified: it runs under
the exact production DockerRunner contract (`--network=none --read-only --user
65534`; see README "Test"), emitting aggregates only.

## params (optional, per offer/job)

```json
{"detectors": ["A2", "A7"]}
```
A subset of `A1/A2/A3/A5/A6/A7/D1/D2` (default = all 8 offline tabular detectors).
