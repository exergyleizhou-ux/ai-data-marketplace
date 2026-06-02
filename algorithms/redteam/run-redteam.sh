#!/usr/bin/env bash
# §19 adversarial sandbox test — RUN ON A HOST WITH DOCKER.
#
# Builds the red-team payload and runs each attack under the SAME hardening flags
# the platform uses (mirror of backend/internal/modules/compute/runner_docker.go
# `dockerRunArgs`), asserting the docker-level containments hold:
#   - egress blocked        (--network=none)
#   - read-only rootfs       (--read-only: writing outside /out is blocked)
#   - OOM killed             (--memory=512m kills a 4 GiB allocation)
#   - runtime timeout killed (the runner's context timeout kills a long sleep)
#
# The PLATFORM-level gates — output-size rejection and "algorithm stdout is never
# surfaced to the buyer" — are covered by the Go tests
# (compute engine_integration_test: oversize -> rejected; design §7.4), which run
# in CI without docker. Together these are the §19 suite.
#
# Usage:  REDTEAM_IMAGE=vo-redteam:test ./run-redteam.sh
set -u
IMG="${REDTEAM_IMAGE:-vo-redteam:test}"
HERE="$(cd "$(dirname "$0")" && pwd)"

command -v docker >/dev/null || { echo "docker not found — run this on a host with Docker"; exit 2; }
echo "building $IMG ..."
docker build -q -t "$IMG" "$HERE" >/dev/null || { echo "image build failed"; exit 1; }

# Hardening flags — keep in sync with dockerRunArgs in runner_docker.go.
FLAGS=(--rm --network=none --read-only --security-opt=no-new-privileges
       --cap-drop=ALL --pids-limit=128 --memory=512m --cpus=1
       --tmpfs=/tmp:rw,size=64m,nodev,nosuid,noexec)

pass=0; fail=0
check() { if [ "$2" -eq 0 ]; then echo "PASS  $1"; pass=$((pass+1)); else echo "FAIL  $1"; fail=$((fail+1)); fi; }

# --- egress must be blocked ---
out="$(mktemp -d)"
docker run "${FLAGS[@]}" -v "$out:/out" -e VO_ATTACK=egress "$IMG" >/dev/null 2>&1
r="$(cat "$out/result.json" 2>/dev/null)"
echo "$r" | grep -q '"tcp": *"blocked'; check "egress: TCP connect blocked" $?
echo "$r" | grep -q '"dns": *"blocked'; check "egress: DNS resolution blocked" $?

# --- read-only rootfs: writing outside /out must be blocked ---
out="$(mktemp -d)"
docker run "${FLAGS[@]}" -v "$out:/out" -e VO_ATTACK=read_host "$IMG" >/dev/null 2>&1
r="$(cat "$out/result.json" 2>/dev/null)"
echo "$r" | grep -q '"rootfs_write": *"blocked'; check "rootfs: write to / blocked (read-only)" $?

# --- OOM: a 4 GiB allocation must be killed under --memory=512m ---
out="$(mktemp -d)"
docker run "${FLAGS[@]}" -v "$out:/out" -e VO_ATTACK=oom "$IMG" >/dev/null 2>&1
code=$?
# A successful allocation writes result.json with "allocated"; containment means
# the process was killed (non-zero exit, no "allocated" result).
[ "$code" -ne 0 ] && ! grep -q allocated "$out/result.json" 2>/dev/null; check "oom: 4GiB allocation killed by --memory" $?

# --- timeout: a long sleep must be killed by the runtime budget ---
# The platform kills via the runner's context timeout; here we emulate that with
# an outer `timeout`. Containment = the container did NOT run to completion.
out="$(mktemp -d)"
timeout 8 docker run "${FLAGS[@]}" -v "$out:/out" -e VO_ATTACK=timeout "$IMG" >/dev/null 2>&1
code=$?
[ "$code" -ne 0 ]; check "timeout: long-running job killed by the runtime budget" $?

echo "-----"
echo "PASS=$pass FAIL=$fail"
[ "$fail" -eq 0 ]
