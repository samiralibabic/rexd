# rexd

`rexd` is a lightweight remote execution and filesystem plane implementing the REXD v1 JSON-RPC protocol.

Protocol spec: `docs/spec/rexd_v1_protocol.md`

## Features

- JSON-RPC 2.0 over NDJSON stdio (`rexd --stdio`)
- Optional HTTP + WebSocket transport (`--http :8080`)
- Session lifecycle (`session.open`, `session.info`, `session.close`)
- Process lifecycle (`exec.start`, `exec.wait`, `exec.kill`, `exec.input`)
- Filesystem surface (`fs.read`, `fs.write`, `fs.list`, `fs.glob`, `fs.stat`)
- PTY extension (`pty.open`, `pty.input`, `pty.resize`, `pty.close`)
- Event streaming (`exec.stdout`, `exec.stderr`, `exec.exit`, `pty.output`, `pty.exit`)
- Security guardrails (allowlisted roots, configurable limits, audit logging)

## Build

```bash
go build ./cmd/rexd
```

Or with Make:

```bash
make build
```

## Install

Latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/samiralibabic/rexd/main/scripts/install.sh | bash
```

Pinned version:

```bash
curl -fsSL https://raw.githubusercontent.com/samiralibabic/rexd/main/scripts/install.sh | REXD_VERSION=v0.1.2 bash
```

Custom install dir:

```bash
curl -fsSL https://raw.githubusercontent.com/samiralibabic/rexd/main/scripts/install.sh | REXD_INSTALL_DIR="$HOME/.local/bin" bash
```

## Run (stdio)

```bash
./rexd --stdio --config ./rexd.example.toml
```

Over SSH:

```bash
ssh deploy@server-a /usr/local/bin/rexd --stdio
```

## Run (HTTP + WebSocket)

```bash
./rexd --http :8080 --config ./rexd.example.toml
```

- HTTP JSON-RPC endpoint defaults to `/rpc`
- WS JSON-RPC endpoint defaults to `/ws`

## Sample requests

Open session:

```json
{"jsonrpc":"2.0","id":1,"method":"session.open","params":{"client_name":"my-agent","workspace_roots":["/srv/myapp"]}}
```

Start command:

```json
{"jsonrpc":"2.0","id":2,"method":"exec.start","params":{"session_id":"s_1","argv":["git","status","--short"],"cwd":"/srv/myapp"}}
```

Read file:

```json
{"jsonrpc":"2.0","id":3,"method":"fs.read","params":{"session_id":"s_1","path":"/srv/myapp/README.md"}}
```

## Config

Use the example file at `rexd.example.toml` as a template. In production, place config at `/etc/rexd/config.toml`.

## Quick verification scripts

After `go build -o rexd ./cmd/rexd`, run:

```bash
bash ./scripts/verify-all.sh
```

Or run individual checks:

```bash
bash ./scripts/verify-stdio.sh
bash ./scripts/verify-http.sh
bash ./scripts/verify-ws.sh
```

Notes:
- The scripts generate a temporary config with the repo root as the allowed root.
- `verify-ws.sh` requires `websocat` (`brew install websocat`).

## Common developer commands

```bash
make tidy
make test
make verify
```

## Open-source docs

- License: `LICENSE`
- Contributing guide: `CONTRIBUTING.md`
- Code of conduct: `CODE_OF_CONDUCT.md`
- Security policy: `SECURITY.md`
