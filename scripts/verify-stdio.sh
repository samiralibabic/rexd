#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${ROOT_DIR}/rexd"
CFG="$(mktemp)"

if [[ ! -x "${BIN}" ]]; then
  echo "Missing binary: ${BIN}"
  echo "Build first: go build -o rexd ./cmd/rexd"
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required for JSON parsing"
  exit 1
fi

cat > "${CFG}" <<EOF
[server]
stdio = true
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

IN_FIFO="$(mktemp -u)"
OUT_FIFO="$(mktemp -u)"
mkfifo "${IN_FIFO}" "${OUT_FIFO}"
"${BIN}" --stdio --config "${CFG}" <"${IN_FIFO}" >"${OUT_FIFO}" &
REXD_PID=$!
exec 3>"${IN_FIFO}"
exec 4<"${OUT_FIFO}"
trap 'kill "${REXD_PID}" 2>/dev/null || true; exec 3>&- 4<&-; rm -f "${CFG}" "${IN_FIFO}" "${OUT_FIFO}"' EXIT

open_req="$(cat <<EOF
{"jsonrpc":"2.0","id":1,"method":"session.open","params":{"client_name":"verify-stdio","workspace_roots":["${ROOT_DIR}"]}}
EOF
)"
echo "${open_req}" >&3
IFS= read -r open_resp <&4
sid="$(printf '%s' "${open_resp}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["session_id"])')"
echo "session.open ok: ${sid}"

write_req="$(cat <<EOF
{"jsonrpc":"2.0","id":2,"method":"fs.write","params":{"session_id":"${sid}","path":"${ROOT_DIR}/rexd_verify.txt","content":"verify\n","encoding":"utf8","mode":"replace","mkdir_parents":true}}
EOF
)"
echo "${write_req}" >&3
IFS= read -r _ <&4
echo "fs.write ok"

read_req="$(cat <<EOF
{"jsonrpc":"2.0","id":3,"method":"fs.read","params":{"session_id":"${sid}","path":"${ROOT_DIR}/rexd_verify.txt"}}
EOF
)"
echo "${read_req}" >&3
IFS= read -r read_resp <&4
content="$(printf '%s' "${read_resp}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["content"])')"
printf 'fs.read content: %q\n' "${content}"

exec_req="$(cat <<EOF
{"jsonrpc":"2.0","id":4,"method":"exec.start","params":{"session_id":"${sid}","argv":["sh","-lc","printf rexd-stdio-ok"],"cwd":"${ROOT_DIR}"}}
EOF
)"
echo "${exec_req}" >&3
IFS= read -r _ <&4

got_stdout=0
got_exit=0
for _i in {1..8}; do
  IFS= read -r evt <&4
  method="$(printf '%s' "${evt}" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("method",""))')"
  if [[ "${method}" == "exec.stdout" ]]; then
    echo "exec.stdout received"
    got_stdout=1
  fi
  if [[ "${method}" == "exec.exit" ]]; then
    echo "exec.exit received"
    got_exit=1
    break
  fi
done

if [[ "${got_stdout}" -ne 1 || "${got_exit}" -ne 1 ]]; then
  echo "Missing expected exec events (stdout=${got_stdout}, exit=${got_exit})"
  exit 1
fi

echo "STDIO verification passed"
