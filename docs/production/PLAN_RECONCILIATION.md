# Original-plan reconciliation

The five files below remain untracked and unmodified in
`/Users/lei/ai-data-marketplace-seed`. They were planning inputs, not files to
copy blindly into the production branch. This reconciliation maps their intent
to authoritative implementation commits and records what was deliberately
replaced, incomplete or out of scope.

Status meanings: **absorbed** is implemented in the final architecture;
**replaced** reached the intent through a safer/current design; **incomplete**
needs environment or product work beyond this branch; **out** is deliberately
excluded from the finalized Definition of Done.

## `2026-07-06-oasis-lumen-productivity-uplift.md`

- Deployment source, Lab routing, Science fallback, unified Workbench, honest
  certificate states, responsive Lab and critical browser journeys: **absorbed
  or replaced** by `fb4635b`, `98d0742`, `8a1eee2`, `51fef34`, `5a69586`,
  `67339b9`, `517495e`, `f26db18`, `0b4be68`, `29e9e02` and `91a9187`.
- The old source-path guard and VPS scripts: **replaced** by immutable image
  tags, Compose migration ordering and explicit deploy/rollback runbooks.
- Live public route verification and broad visual redesign: **incomplete/out**;
  public deployment was not authorized, and visual restyling is not necessary
  to prove the runtime Definition of Done.

## `2026-07-06-oasis-workbench-v2-ultimate-production-plan.md`

- Unified shell/status, projects/files, Runs, approvals, artifact provenance,
  evidence, usage/quota accounting and go-live gates: **absorbed** by
  `38139a7`, `8cebe9c`, `2477527`, `51fef34`, `54b7e81`, `e5b7130`, `4060025`,
  `29e9e02`, `91a9187`, `0cf7e06` and `4016b89`.
- Provider profiles and one-click provider transactions: **out**; the production
  design uses configured providers and bounded service health rather than
  importing arbitrary third-party configuration into the trust boundary.
- C2D wizard/publish funnel and pricing-page experiments: **incomplete/out**;
  existing C2D/marketplace flows remain, but speculative monetization UI is not
  part of shared-runtime production safety.
- Checkpoint/rewind, goal mode, workflow scripts and worktree isolation:
  **out**, explicitly listed as post-revenue candidates by the source plan.

## `2026-07-08-oasis-lumen-ultimate-parity-master-plan.md`

- Locked navigation, Workbench contracts, service topology, Lab/Science
  integration, production evidence and fail-closed security: **absorbed or
  replaced** by the full `fb4635b..4016b89` branch series.
- Literal parity with reference repositories: **replaced** by contract-level
  parity. Oasis keeps tenant/auth/storage/approval authority and Lumen keeps
  execution authority instead of cloning reference architecture or UI.
- Public probes and commercial KPI validation: **incomplete** and required only
  at the environment promotion/product-validation stages.

## `_gen_ultimate_plan.py`

- The generator's task taxonomy and baseline inventory: **absorbed** as review
  input by the two generated planning documents.
- The script itself and generated bulk plan maintenance: **out**. Production
  truth now lives in implementation, tests, OpenAPI and these concise runbooks;
  the generator is not a runtime or release dependency.

## `2026-07-08-oasis-lumen-deep-parity-design.md`

- Lab project/file/run/artifact behavior, Science bridge reliability and one
  unified Workbench: **absorbed or replaced** by `98d0742`, `8a1eee2`,
  `38139a7`, `2477527`, `51fef34`, `5a69586`, `517495e` and `0b4be68`.
- Deep UI/feature parity with OpenClaudeScience and CSSwitch: **out**. Only the
  useful workflow contracts were adopted; unsafe configuration import and
  duplicated execution state were rejected.
- Public-host success criteria: **incomplete** pending authorized staging/host
  execution.

## Reconciliation conclusion

No original item is silently presumed complete. The accepted local production
candidate consists of the absorbed/replaced runtime and safety contracts proven
in `TEST_EVIDENCE.md`. Items above marked incomplete remain promotion or future
product work; items marked out are not hidden Definition-of-Done debt.
