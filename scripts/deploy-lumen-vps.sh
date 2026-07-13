#!/usr/bin/env bash
set -euo pipefail

cat >&2 <<'EOF'
ERROR: scripts/deploy-lumen-vps.sh is retired.

The old entry point bypassed host verification and directly mutated a public
machine. Use the immutable, non-interactive candidate procedure in
docs/production/DEPLOY.md instead. That procedure never pushes, merges, or
performs a public deployment without separate authorization.
EOF
exit 64
