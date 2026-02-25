# REXD v1 Protocol Spec

## Purpose

A tiny, runtime-agnostic **remote execution plane** for agentic runtimes.

It provides one stable backend protocol for:
- command execution
- file operations
- process streaming
- optional PTY sessions

It is **not** tied to any single agent framework. MCP is an adapter, not the core.

---

## Design Goals

1. **Runtime-agnostic**
   - Works with any agent runtime (OpenCode, custom runners, OpenAI/Anthropic tool wrappers, CLI agents, etc.).

2. **Low remote footprint**
   - Install a single binary (like `vim`), no heavy runtime.
   - Supports **no-daemon mode** over SSH stdio.

3. **No tool duplication / context bloat**
   - The LLM sees one logical toolset (`exec`, `read`, `write`, etc.).
   - Local runtime handles routing (`local` vs `server-a`).

4. **Safe by default**
   - Workspace allowlists, timeouts, output limits, audit logs.

5. **Transport-agnostic**
   - Core protocol: JSON-RPC 2.0 over a byte stream.
   - Primary transport: stdio (including SSH stdio).
   - Optional transport: HTTP(S)/WebSocket.

---

## Non-Goals (v1)

- Full container orchestration
- SSH key management
- Remote package deployment / provisioning
- Distributed job scheduling
- Cross-host transaction semantics
- Rich file sync protocol

This is a **thin execution/file plane**, not a DevOps platform.

---

## Mental Model

- `rexd` runs on the remote host.
- A client connects to it via:
  - local process stdio (`rexd --stdio`)
  - or SSH stdio (`ssh host rexd --stdio`)
  - or optional HTTPS/WebSocket later
- Agent runtimes call a local adapter/client library.
- The runtime chooses the target host/session.
- The model does **not** see host-specific duplicate tools.

---

## Architecture

### Components

1. **rexd (remote binary)**
   - Single binary installed on remote host.
   - Exposes JSON-RPC methods over stdio (v1 required).

2. **rex client (library/CLI)**
   - Connects to `rexd`.
   - Manages transport, request IDs, streaming, reconnects (optional).

3. **Runtime adapter**
   - Converts agent tool calls (`bash`, `read`, `write`, etc.) into REXD RPC calls.
   - Maintains current `target` outside the prompt.

4. **Optional compatibility adapters**
   - MCP server exposing local logical tools backed by REXD
   - OpenAI/Anthropic tool wrapper
   - OpenCode plugin

---

## Core Protocol

### Encoding

- **Protocol:** JSON-RPC 2.0
- **Framing (stdio):** newline-delimited JSON (**NDJSON**) for v1 simplicity
- **Character encoding:** UTF-8

> v2 can switch to LSP-style `Content-Length` framing if needed. NDJSON is enough for v1 and easy to debug.

### Message Types

- **Request**: JSON-RPC request with `id`
- **Response**: JSON-RPC result/error with matching `id`
- **Event**: JSON-RPC notification (no `id`) used for streaming output and process state

### Protocol Version

Every `session.open` response includes:
- `protocol: "rexd/1"`
- `server_version`
- `capabilities`

---

## Session Model

A session is a scoped logical connection with policy and cwd state.

### Method: `session.open`

Creates a session and returns a `session_id`.

**Request params**
- `client_name` (string)
- `client_version` (string, optional)
- `workspace_roots` (array of strings, optional preferred roots)
- `requested_capabilities` (array, optional)

**Response**
- `session_id`
- `protocol`
- `server_version`
- `capabilities`
- `limits`
- `workspace_roots`

### Method: `session.close`

Closes a session and terminates any attached child processes unless detached.

### Method: `session.info`

Returns session state (cwd, running processes, limits, etc.).

---

## Standard Methods (v1)

### 1) `exec.start`

Start a command (non-PTY) and stream output via events.

#### Request params
- `session_id` (string, required)
- `argv` (array of strings, required)
  Preferred safe mode. Example: `["git", "status", "--short"]`
- `cwd` (string, optional)
- `env` (object string->string, optional)
- `stdin` (string, optional; small payload only)
- `timeout_ms` (integer, optional)
- `max_output_bytes` (integer, optional)
- `shell` (boolean, optional, default `false`)
  If `true`, interpret `command` via shell.
- `command` (string, optional; required only when `shell=true`)
- `detach` (boolean, optional, default `false`)

#### Response
- `process_id` (string)
- `started_at` (timestamp)

#### Events emitted
- `exec.stdout`
- `exec.stderr`
- `exec.exit`
- `exec.error` (if process launch failed)

#### Notes
- `argv` and `shell=false` should be the default for safety.
- `shell=true` is explicit and auditable.

---

### 2) `exec.wait`

Wait for process completion (optional helper if client ignores stream events).

#### Request params
- `session_id`
- `process_id`
- `timeout_ms` (optional)

#### Response
- `status` (`running` | `exited` | `killed` | `timed_out`)
- `exit_code` (nullable int)
- `signal` (nullable string)
- `bytes_stdout`
- `bytes_stderr`

---

### 3) `exec.kill`

Terminate a process.

#### Request params
- `session_id`
- `process_id`
- `signal` (optional; default `TERM`)

#### Response
- `ok` (boolean)

---

### 4) `exec.input` (optional in v1, recommended)

Send stdin to a running non-PTY process.

#### Request params
- `session_id`
- `process_id`
- `data` (string)
- `eof` (boolean, optional)

#### Response
- `accepted_bytes` (int)

---

### 5) `fs.read`

Read a file.

#### Request params
- `session_id`
- `path`
- `offset` (optional, int)
- `length` (optional, int)
- `encoding` (`utf8` | `base64`, default `utf8`)

#### Response
- `path`
- `size`
- `mtime`
- `encoding`
- `content`
- `truncated` (boolean)

---

### 6) `fs.write`

Write a file (replace or create).

#### Request params
- `session_id`
- `path`
- `content`
- `encoding` (`utf8` | `base64`, default `utf8`)
- `mode` (`create` | `replace` | `append`, default `replace`)
- `mkdir_parents` (boolean, default `false`)
- `atomic` (boolean, default `true`)
- `expected_mtime` (optional; optimistic concurrency)

#### Response
- `path`
- `bytes_written`
- `mtime`
- `created` (boolean)

---

### 7) `fs.list`

List directory entries.

#### Request params
- `session_id`
- `path`
- `recursive` (boolean, default `false`)
- `max_entries` (optional)

#### Response
- `path`
- `entries` array of:
  - `name`
  - `path`
  - `type` (`file` | `dir` | `symlink` | `other`)
  - `size` (nullable)
  - `mtime` (nullable)

---

### 8) `fs.glob`

Glob files within allowed roots.

#### Request params
- `session_id`
- `pattern`
- `cwd` (optional)
- `max_matches` (optional)

#### Response
- `matches` (array of paths)

---

### 9) `fs.stat`

Stat a file or directory.

#### Request params
- `session_id`
- `path`

#### Response
- `path`
- `exists` (boolean)
- `type`
- `size`
- `mtime`
- `mode`
- `uid` (optional)
- `gid` (optional)
- `symlink_target` (optional)

---

## PTY Support (Optional v1 Extension)

Needed for interactive programs (`vim`, `top`, installers, shells).

### `pty.open`
Open a PTY-backed process.

**Request params**
- `session_id`
- `argv` or `command` (same rules as `exec.start`)
- `cwd`, `env`
- `cols`, `rows`

**Response**
- `pty_id`
- `process_id`

### `pty.input`
Send keystrokes / bytes.

### `pty.resize`
Resize terminal.

### `pty.close`
Close PTY (and optionally process).

### PTY Events
- `pty.output`
- `pty.exit`

> If you want to stay ultra-lean, you can defer PTY to v1.1 and ship only non-PTY exec + file ops first.

---

## Event Streaming

Events are JSON-RPC notifications (no `id`) emitted asynchronously.

### Event: `exec.stdout`
```json
{
  "jsonrpc": "2.0",
  "method": "exec.stdout",
  "params": {
    "session_id": "s_123",
    "process_id": "p_456",
    "seq": 1,
    "data": "line 1\n",
    "encoding": "utf8"
  }
}
```

### Event: `exec.stderr`
Same shape as stdout.

### Event: `exec.exit`
```json
{
  "jsonrpc": "2.0",
  "method": "exec.exit",
  "params": {
    "session_id": "s_123",
    "process_id": "p_456",
    "exit_code": 0,
    "signal": null,
    "timed_out": false,
    "duration_ms": 328,
    "bytes_stdout": 120,
    "bytes_stderr": 0
  }
}
```

### Event ordering
- `seq` must be monotonic per `(process_id, stream)`.
- `exec.exit` is terminal for a process.

---

## Errors

Use standard JSON-RPC errors with a `code`, `message`, and structured `data`.

### Standard error codes (v1)
- `-32602` invalid params
- `-32601` method not found
- `-32001` unauthorized
- `-32002` forbidden_path
- `-32003` timeout
- `-32004` output_limit_exceeded
- `-32005` process_not_found
- `-32006` concurrency_conflict
- `-32007` unsupported_capability
- `-32008` resource_limit

### Example
```json
{
  "jsonrpc": "2.0",
  "id": 7,
  "error": {
    "code": -32002,
    "message": "Path is outside allowed roots",
    "data": {
      "path": "/etc/passwd",
      "allowed_roots": ["/srv/app", "/home/deploy"]
    }
  }
}
```

---

## Security Model (v1)

### 1) Allowlisted workspace roots (required)
`rexd` only permits file ops and `cwd` within configured roots.

Example:
- `/srv/myapp`
- `/home/deploy/projects`

### 2) Command execution policy
- Default user is the OS user running `rexd`
- `argv` mode preferred (no shell)
- Shell mode explicit (`shell=true`)
- Optional command allowlist / denylist (configurable)

### 3) Limits
Configurable per server and overridable downward per session:
- `max_output_bytes`
- `max_file_read_bytes`
- `max_processes_per_session`
- `default_timeout_ms`
- `hard_timeout_ms`
- `max_concurrent_sessions`

### 4) Audit log (recommended)
Each request should emit structured audit entries:
- timestamp
- session_id
- client_name
- method
- normalized params (redacted where needed)
- exit_code / result summary

### 5) Transport security
#### SSH stdio mode (recommended default)
- Reuse SSH auth, host keys, encryption
- No open port needed

#### HTTP mode (optional)
- mTLS or token auth required
- Bind private interface only by default
- Reverse proxy allowed

---

## Transport Profiles

### Profile A: SSH stdio (primary)
Client launches:
```bash
ssh deploy@server-a /usr/local/bin/rexd --stdio
```

Pros:
- no daemon
- no extra port
- easy auth
- "install binary and go"

### Profile B: Local stdio (for local target)
Client launches:
```bash
rexd --stdio
```

### Profile C: HTTP(S) daemon (optional)
For lower latency / shared service mode.

Endpoints can carry JSON-RPC payloads directly (POST) or use WebSocket for streaming.

---

## Runtime Routing Model (Important)

This solves the **tool duplication / context bloat** issue.

### Principle
The model sees one logical tool surface. The runtime stores target routing state outside the prompt.

### Example session state (runtime side)
```json
{
  "target": "server-a",
  "cwd": "/srv/myapp",
  "transport": "ssh-stdio"
}
```

### Runtime behavior
- Model calls logical `bash` tool
- Runtime adapter translates to `exec.start` against current target
- Model calls `read` tool
- Runtime adapter translates to `fs.read` against same target

No duplicate tools like:
- `bash_server_a`
- `bash_server_b`
- `read_remote`

### How target is selected (deterministic)
Choose one or combine:
- explicit user command (`/target server-a`)
- project config (repo -> host mapping)
- path prefix mapping
- session carry-over

Keep routing logic out of model context whenever possible.

---

## Minimal API Examples

### 1) Open session
**Request**
```json
{"jsonrpc":"2.0","id":1,"method":"session.open","params":{"client_name":"my-agent","workspace_roots":["/srv/myapp"]}}
```

**Response**
```json
{"jsonrpc":"2.0","id":1,"result":{"session_id":"s_1","protocol":"rexd/1","server_version":"0.1.0","capabilities":["exec","fs","events"],"limits":{"default_timeout_ms":30000,"max_output_bytes":1048576},"workspace_roots":["/srv/myapp"]}}
```

### 2) Start command
**Request**
```json
{"jsonrpc":"2.0","id":2,"method":"exec.start","params":{"session_id":"s_1","argv":["git","status","--short"],"cwd":"/srv/myapp"}}
```

**Response**
```json
{"jsonrpc":"2.0","id":2,"result":{"process_id":"p_1","started_at":"2026-02-25T10:00:00Z"}}
```

**Stream events (notifications)**
```json
{"jsonrpc":"2.0","method":"exec.stdout","params":{"session_id":"s_1","process_id":"p_1","seq":1,"data":" M README.md\n","encoding":"utf8"}}
```
```json
{"jsonrpc":"2.0","method":"exec.exit","params":{"session_id":"s_1","process_id":"p_1","exit_code":0,"signal":null,"timed_out":false,"duration_ms":93,"bytes_stdout":12,"bytes_stderr":0}}
```

### 3) Read file
**Request**
```json
{"jsonrpc":"2.0","id":3,"method":"fs.read","params":{"session_id":"s_1","path":"/srv/myapp/README.md"}}
```

### 4) Write file (atomic replace)
**Request**
```json
{"jsonrpc":"2.0","id":4,"method":"fs.write","params":{"session_id":"s_1","path":"/srv/myapp/README.md","content":"# Hello\n","encoding":"utf8","mode":"replace","atomic":true}}
```

---

## Configuration (Remote `rexd`)

Suggested config file: `/etc/rexd/config.toml`

```toml
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
path = "/srv/myapp"

[[security.allowed_roots]]
path = "/home/deploy/projects"

[audit]
enabled = true
path = "/var/log/rexd/audit.log"
```

---

## Installation UX (Target = "like vim")

### Option 1: Copy binary
```bash
scp rexd deploy@server-a:/usr/local/bin/rexd
ssh deploy@server-a 'chmod +x /usr/local/bin/rexd'
```

### Option 2: Package (later)
- `.deb` / `.rpm`
- Homebrew/Linuxbrew formula

### Option 3: systemd (optional daemon mode)
- `rexd.service` or `rexd.socket`

---

## Adapter Strategy (How this stays generic)

### Core
- `rexd` protocol = source of truth

### Adapters (thin)
1. **MCP adapter**
   - Exposes one logical toolset backed by REXD
   - Runtime still handles target routing

2. **SDKs**
   - Go / TypeScript / Python client libraries
   - Used by custom agents and CLIs

3. **Runtime plugins**
   - OpenCode plugin
   - Claude Code wrapper
   - Custom shell/TUI runtimes

This keeps the protocol stable and the ecosystem flexible.

---

## v1 Implementation Plan (Lean)

### Phase 1 (must-have)
- `session.open/close/info`
- `exec.start`, `exec.wait`, `exec.kill`
- `fs.read`, `fs.write`, `fs.list`, `fs.glob`, `fs.stat`
- event notifications for stdout/stderr/exit
- allowlisted roots + limits + audit
- SSH stdio transport

### Phase 2 (nice-to-have)
- `exec.input`
- PTY methods
- optimistic concurrency (`expected_mtime`) enforcement
- HTTP/WebSocket transport

### Phase 3 (polish)
- resumable sessions
- compressed streaming
- advanced policy engine (sudo allowlist, command classes)
- metrics endpoint

---

## Opinionated Defaults (Recommended)

- **Transport:** SSH stdio
- **Protocol:** JSON-RPC 2.0 NDJSON
- **Exec mode:** `argv` only unless explicitly using shell
- **Root policy:** strict allowlist (no `/`)
- **Timeouts:** 30s default, 5m hard
- **Output cap:** 1MB per process by default
- **PTY:** optional, defer if speed matters

These defaults give a very usable v1 with minimal complexity.

---

## Open Questions (Decide before implementation)

1. **NDJSON vs Content-Length framing**
   - NDJSON is simpler; Content-Length is more robust for arbitrary binary.
   - v1 recommendation: NDJSON + base64 for binary payloads.

2. **Process lifetime across disconnects**
   - Should processes die on session close/disconnect?
   - v1 recommendation: yes by default, with `detach=true` opt-in.

3. **PTY in v1 or v1.1**
   - If your workflows need `vim`/interactive shell, include PTY now.
   - Otherwise ship non-PTY first.

4. **Policy granularity**
   - Root-only policy is enough for v1.
   - Command allowlists/sudo policy can come later unless required.

---

## Summary

REXD v1 is a **small remote execution/file protocol** that gives you a generic execution plane for agentic runtimes without duplicating tools in model context.

- one binary on remote
- SSH-first transport
- JSON-RPC core
- safe defaults
- runtime-controlled target routing
- MCP and others as adapters, not the core

That gets you the ergonomics you want: **"type English locally, execute anywhere"** without installing a full agent runtime on every box.
