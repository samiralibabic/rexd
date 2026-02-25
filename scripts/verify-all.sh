#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "[1/3] STDIO verification"
bash "${ROOT_DIR}/scripts/verify-stdio.sh"

echo "[2/3] HTTP verification"
bash "${ROOT_DIR}/scripts/verify-http.sh"

if command -v websocat >/dev/null 2>&1; then
  echo "[3/3] WS verification"
  bash "${ROOT_DIR}/scripts/verify-ws.sh"
else
  echo "[3/3] WS verification skipped (websocat not installed)"
fi

echo "All requested verifications completed."
