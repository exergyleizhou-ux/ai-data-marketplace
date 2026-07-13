# Known limitations and promotion blockers

## Deliberately outside this local Definition of Done

- No remote push, merge to `main`, production traffic switch or public
  deployment was performed or authorized.
- Public-host Caddy/TLS, DNS, firewall and port exposure require staging/host
  acceptance. Docker Desktop on the verification Mac wedged when publishing
  the Caddy host port during Phase 9, so proxy semantics were instead exercised
  through the isolated loopback staging path.
- External S3 credentials/versioning, backup restore, SMTP/payment providers,
  metrics scraping and alert delivery require environment-owned credentials and
  operator verification before promotion.
- Schema migrations 36-39 remove Workbench state on down. Production policy is
  forward-fix unless writers are stopped, a snapshot exists, and loss has been
  explicitly accepted.

## Product scope intentionally deferred

- Provider-profile import, arbitrary provider switching, checkpoint/rewind,
  deterministic workflow scripting and automatic worktree management were
  ideas in the original planning set, not requirements of the finalized shared
  runtime. They remain out of scope until a validated revenue/user need exists.
- Full OpenClaudeScience/CSSwitch surface parity is not a goal. The accepted
  product absorbs projects, files, Runs, approvals, artifacts, evidence,
  bounded status and recovery while preserving Oasis/Lumen security boundaries.
- Billing integration beyond durable integer usage/cost/quota accounting and
  the existing marketplace payment paths is not introduced by this branch.

## Dependency posture

The exact current production dependency audit result is recorded in
`TEST_EVIDENCE.md`. Any remaining moderate transitive advisory must be reviewed
again at promotion; forced incompatible downgrades are prohibited. Any new high
or critical finding blocks release.
