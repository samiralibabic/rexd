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

if ! command -v websocat >/dev/null 2>&1; then
  echo "websocat is required (brew install websocat)"
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

"${BIN}" --http ":${PORT}" --config "${CFG}" >/tmp/rexd-ws.log 2>&1 &
PID=$!
WS_IN="$(mktemp -u)"
WS_OUT="$(mktemp -u)"
mkfifo "${WS_IN}" "${WS_OUT}"
websocat "ws://127.0.0.1:${PORT}/ws" <"${WS_IN}" >"${WS_OUT}" &
WS_PID=$!
exec 5>"${WS_IN}"
exec 6<"${WS_OUT}"
trap 'kill "${WS_PID}" 2>/dev/null || true; wait "${WS_PID}" 2>/dev/null || true; kill "${PID}" 2>/dev/null || true; wait "${PID}" 2>/dev/null || true; exec 5>&- 6<&-; rm -f "${CFG}" "${WS_IN}" "${WS_OUT}"' EXIT
sleep 0.4

open_req="$(cat <<EOF
{"jsonrpc":"2.0","id":1,"method":"session.open","params":{"client_name":"verify-ws","workspace_roots":["${ROOT_DIR}"]}}
EOF
)"
echo "${open_req}" >&5
IFS= read -r open_resp <&6
sid="$(printf '%s' "${open_resp}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["session_id"])')"
echo "session.open ok: ${sid}"

exec_req="$(cat <<EOF
{"jsonrpc":"2.0","id":2,"method":"exec.start","params":{"session_id":"${sid}","argv":["sh","-lc","printf ws-ok"],"cwd":"${ROOT_DIR}"}}
EOF
)"
echo "${exec_req}" >&5
IFS= read -r _ <&6

got_stdout=0
got_exit=0
for _i in {1..8}; do
  IFS= read -r evt <&6
  method="$(printf '%s' "${evt}" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("method",""))')"
  if [[ "${method}" == "exec.stdout" ]]; then
    got_stdout=1
  fi
  if [[ "${method}" == "exec.exit" ]]; then
    got_exit=1
    break
  fi
done

if [[ "${got_stdout}" -ne 1 || "${got_exit}" -ne 1 ]]; then
  echo "Missing ws events (stdout=${got_stdout}, exit=${got_exit})"
  exit 1
fi

echo "WS verification passed"
