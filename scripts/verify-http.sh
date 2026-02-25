#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${ROOT_DIR}/rexd"
CFG="$(mktemp)"
PORT="${1:-8080}"

if [[ ! -x "${BIN}" ]]; then
  echo "Missing binary: ${BIN}"
  echo "Build first: go build -o rexd ./cmd/rexd"
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required"
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required for JSON parsing"
  exit 1
fi

cat > "${CFG}" <<EOF
[server]
stdio = false
http_listen = ":${PORT}"
http_path = "/rpc"
ws_path = "/ws"
log_level = "info"

[limits]
default_timeout_ms = 30000
hard_timeout_ms = 300000
max_output_bytes = 1048576
max_file_read_bytes = 1048576
max_processes_per_session = 8
max_concurrent_sessions = 16

[security]
allow_shell = true

[[security.allowed_roots]]
path = "${ROOT_DIR}"

[audit]
enabled = false
path = ""
EOF

"${BIN}" --http ":${PORT}" --config "${CFG}" >/tmp/rexd-http.log 2>&1 &
PID=$!
trap 'kill "${PID}" 2>/dev/null || true; rm -f "${CFG}"' EXIT
sleep 0.4

open_payload="$(cat <<EOF
{"jsonrpc":"2.0","id":1,"method":"session.open","params":{"client_name":"verify-http","workspace_roots":["${ROOT_DIR}"]}}
EOF
)"
open_resp="$(curl -sS -X POST "http://127.0.0.1:${PORT}/rpc" -H "content-type: application/json" -d "${open_payload}")"
sid="$(printf '%s' "${open_resp}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["session_id"])')"
echo "session.open ok: ${sid}"

stat_payload="$(cat <<EOF
{"jsonrpc":"2.0","id":2,"method":"fs.stat","params":{"session_id":"${sid}","path":"${ROOT_DIR}/README.md"}}
EOF
)"
stat_resp="$(curl -sS -X POST "http://127.0.0.1:${PORT}/rpc" -H "content-type: application/json" -d "${stat_payload}")"
exists="$(printf '%s' "${stat_resp}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["exists"])')"
if [[ "${exists}" != "True" ]]; then
  echo "fs.stat failed: ${stat_resp}"
  exit 1
fi
echo "fs.stat ok"

bad_payload="$(cat <<EOF
{"jsonrpc":"2.0","id":3,"method":"fs.read","params":{"session_id":"${sid}","path":"/etc/passwd"}}
EOF
)"
bad_resp="$(curl -sS -X POST "http://127.0.0.1:${PORT}/rpc" -H "content-type: application/json" -d "${bad_payload}")"
code="$(printf '%s' "${bad_resp}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["error"]["code"])')"
if [[ "${code}" != "-32002" ]]; then
  echo "Expected forbidden path error (-32002), got: ${bad_resp}"
  exit 1
fi
echo "forbidden path enforcement ok"

echo "HTTP verification passed"
