# redteam — §19 adversarial sandbox test suite

A deliberately **malicious** payload that proves the L1 sandbox contains a
hostile algorithm (design §7, §19). **Never register this as an approved/trusted
algorithm** — it exists only to attack the sandbox in tests.

Two layers, together the §19 suite:

1. **Docker-level containment** — `run-redteam.sh` (run on a host WITH Docker):
   builds the payload and runs each attack under the platform's hardening flags
   (mirror of `dockerRunArgs` in `backend/internal/modules/compute/runner_docker.go`),
   asserting egress is blocked, the rootfs is read-only, a 4 GiB allocation is
   OOM-killed, and a long sleep is killed by the runtime budget.
   ```
   REDTEAM_IMAGE=vo-redteam:test ./run-redteam.sh
   ```
2. **Platform-level gates** — covered by Go tests in CI (no docker needed):
   oversize output → rejected, algorithm stdout never surfaced to the buyer
   (`compute/engine_integration_test.go`, design §7.4).

`test_attack.py` (runs locally + in CI) only checks the probes are GENUINE
attempts (they succeed with no sandbox), so the docker harness flipping them to
"blocked" is meaningful.

Attack modes (env `VO_ATTACK` or params `_attack`): `egress`, `read_host`,
`leak_stdout`, `oversize`, `timeout`, `oom`.
