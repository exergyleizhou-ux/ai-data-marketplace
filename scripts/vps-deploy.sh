#!/usr/bin/env bash
set -euo pipefail

cat >&2 <<'EOF'
ERROR: scripts/vps-deploy.sh is retired.

The old bootstrap replaced local source state, installed unpinned software,
created unsafe credentials, and directly changed a public machine. Use
docs/production/DEPLOY.md with immutable images and an operator-owned secret
store. Public deployment still requires explicit authorization.
EOF
exit 64
