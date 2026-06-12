# deploy/

Production deployment materials. Full step-by-step is in
[`../docs/部署手册.md`](../docs/部署手册.md).

| Path | Files | Use when |
|------|-------|----------|
| **Single VM** | [`docker-compose.prod.yml`](docker-compose.prod.yml) | one box, lowest ops; app containers only, managed DB/Redis/OSS |
| **Kubernetes** | [`k8s/`](k8s/) | multi-instance / autoscaling / rolling deploys |

Both expect **managed** Postgres / Redis / object storage (not self-hosted containers) and pull config from [`../.env.production.example`](../.env.production.example). Secrets go to a KMS / k8s Secret created out-of-band — never commit filled values.

Migrations run as a one-shot step (`api --migrate` service / `k8s/20-migrate-job.yaml`) so the server need not self-migrate.

## Kustomize base / overlays

The k8s manifests are now structured as a kustomize base + per-env overlays:

```
deploy/k8s/
├── base/                 # raw manifests + base kustomization
└── overlays/
    ├── staging/          # namespace marketplace-staging, stg- prefix, 1 replica
    └── prod/             # namespace marketplace, 2 replicas
```

Deploy:

```bash
kubectl apply -k deploy/k8s/overlays/staging   # or .../prod
```

Image tags: the base pins placeholder `REGISTRY/ai-data-marketplace-*:TAG`;
the `release.yml` CD workflow rewrites them to the freshly-built
`ghcr.io/<repo>/{backend,frontend}:<tag>` before applying. For manual deploys,
`sed` the same placeholders or `kustomize edit set image` in the overlay.

## CD pipeline (`.github/workflows/release.yml`)

Triggered on `v*.*.*` tags: build+push images to GHCR → deploy staging
(rollout + `/api/v1/ping` smoke + 5xx-rate auto-rollback) → deploy prod
(manual-approval `production` environment). Requires repo secrets/Environments:
`GHCR` is automatic via `GITHUB_TOKEN`; configure `staging`/`production`
Environments with a `KUBE_CONFIG` and required reviewers before first run.
