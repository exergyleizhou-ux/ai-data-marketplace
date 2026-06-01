# deploy/

Production deployment materials. Full step-by-step is in
[`../docs/部署手册.md`](../docs/部署手册.md).

| Path | Files | Use when |
|------|-------|----------|
| **Single VM** | [`docker-compose.prod.yml`](docker-compose.prod.yml) | one box, lowest ops; app containers only, managed DB/Redis/OSS |
| **Kubernetes** | [`k8s/`](k8s/) | multi-instance / autoscaling / rolling deploys |

Both expect **managed** Postgres / Redis / object storage (not self-hosted containers) and pull config from [`../.env.production.example`](../.env.production.example). Secrets go to a KMS / k8s Secret created out-of-band — never commit filled values.

Migrations run as a one-shot step (`api --migrate` service / `k8s/20-migrate-job.yaml`) so the server need not self-migrate.
