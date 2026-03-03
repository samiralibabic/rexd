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
trap 'kill "${REXD_PID}" 2>/dev/null || true; exec 3>&- 4<&-; rm -f "${CFG}" "${IN_FIFO}" "${OUT_FIFO}" "${ROOT_DIR}/rexd_verify.txt" "${ROOT_DIR}/rexd_patch_added.txt" "${ROOT_DIR}/rexd_patch_delete.txt"' EXIT

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

edit_req="$(cat <<EOF
{"jsonrpc":"2.0","id":4,"method":"fs.edit","params":{"session_id":"${sid}","path":"${ROOT_DIR}/rexd_verify.txt","old_string":"verify\n","new_string":"verified\n"}}
EOF
)"
echo "${edit_req}" >&3
IFS= read -r _ <&4
echo "fs.edit ok"

read_after_edit_req="$(cat <<EOF
{"jsonrpc":"2.0","id":5,"method":"fs.read","params":{"session_id":"${sid}","path":"${ROOT_DIR}/rexd_verify.txt"}}
EOF
)"
echo "${read_after_edit_req}" >&3
IFS= read -r read_after_edit_resp <&4
content_after_edit="$(printf '%s' "${read_after_edit_resp}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["content"])')"
if [[ "${content_after_edit}" != "verified" ]]; then
  echo "fs.edit produced unexpected content: ${content_after_edit}"
  exit 1
fi

create_delete_target_req="$(cat <<EOF
{"jsonrpc":"2.0","id":6,"method":"fs.write","params":{"session_id":"${sid}","path":"${ROOT_DIR}/rexd_patch_delete.txt","content":"delete me\n","encoding":"utf8","mode":"replace","mkdir_parents":true}}
EOF
)"
echo "${create_delete_target_req}" >&3
IFS= read -r _ <&4

patch_body="$(cat <<EOF
*** Begin Patch
*** Update File: ${ROOT_DIR}/rexd_verify.txt
@@
-verified
+patched
*** Add File: ${ROOT_DIR}/rexd_patch_added.txt
+added from patch
*** Delete File: ${ROOT_DIR}/rexd_patch_delete.txt
*** End Patch
EOF
)"
patch_req="$(SID="${sid}" PATCH_BODY="${patch_body}" python3 - <<'PY'
import json
import os
print(json.dumps({
    "jsonrpc": "2.0",
    "id": 7,
    "method": "fs.patch",
    "params": {
        "session_id": os.environ["SID"],
        "patch_text": os.environ["PATCH_BODY"],
    },
}))
PY
)"
echo "${patch_req}" >&3
IFS= read -r _ <&4
echo "fs.patch ok"

read_after_patch_req="$(cat <<EOF
{"jsonrpc":"2.0","id":8,"method":"fs.read","params":{"session_id":"${sid}","path":"${ROOT_DIR}/rexd_verify.txt"}}
EOF
)"
echo "${read_after_patch_req}" >&3
IFS= read -r read_after_patch_resp <&4
content_after_patch="$(printf '%s' "${read_after_patch_resp}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["content"])')"
if [[ "${content_after_patch}" != "patched" ]]; then
  echo "fs.patch update produced unexpected content: ${content_after_patch}"
  exit 1
fi

added_stat_req="$(cat <<EOF
{"jsonrpc":"2.0","id":9,"method":"fs.stat","params":{"session_id":"${sid}","path":"${ROOT_DIR}/rexd_patch_added.txt"}}
EOF
)"
echo "${added_stat_req}" >&3
IFS= read -r added_stat_resp <&4
added_exists="$(printf '%s' "${added_stat_resp}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["exists"])')"
if [[ "${added_exists}" != "True" ]]; then
  echo "fs.patch add failed"
  exit 1
fi

deleted_stat_req="$(cat <<EOF
{"jsonrpc":"2.0","id":10,"method":"fs.stat","params":{"session_id":"${sid}","path":"${ROOT_DIR}/rexd_patch_delete.txt"}}
EOF
)"
echo "${deleted_stat_req}" >&3
IFS= read -r deleted_stat_resp <&4
deleted_exists="$(printf '%s' "${deleted_stat_resp}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["exists"])')"
if [[ "${deleted_exists}" != "False" ]]; then
  echo "fs.patch delete failed"
  exit 1
fi

exec_req="$(cat <<EOF
{"jsonrpc":"2.0","id":11,"method":"exec.start","params":{"session_id":"${sid}","argv":["sh","-lc","printf rexd-stdio-ok"],"cwd":"${ROOT_DIR}"}}
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

if [[ "${got_exit}" -ne 1 ]]; then
  echo "Missing expected exec.exit event (stdout=${got_stdout}, exit=${got_exit})"
  exit 1
fi

echo "STDIO verification passed"
