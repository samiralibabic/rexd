# rexd

`rexd` is a lightweight remote execution and filesystem plane implementing the REXD v1 JSON-RPC protocol.

Protocol spec: `docs/spec/rexd_v1_protocol.md`

## Available plugins

- [opencode-rexd-target](https://github.com/samiralibabic/opencode-rexd-target): OpenCode plugin for routing tools to remote REXD targets.

## Features

- JSON-RPC 2.0 over NDJSON stdio (`rexd --stdio`)
- Optional HTTP + WebSocket transport (`--http :8080`)
- Session lifecycle (`session.open`, `session.info`, `session.close`)
- Process lifecycle (`exec.start`, `exec.wait`, `exec.kill`, `exec.input`)
- Filesystem surface (`fs.read`, `fs.write`, `fs.list`, `fs.glob`, `fs.stat`, `fs.edit`, `fs.patch`)
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
curl -fsSL https://raw.githubusercontent.com/samiralibabic/rexd/main/scripts/install.sh | REXD_VERSION=v0.1.3 bash
```

Custom install dir:

```bash
curl -fsSL https://raw.githubusercontent.com/samiralibabic/rexd/main/scripts/install.sh | REXD_INSTALL_DIR="$HOME/.local/bin" bash
```

## Post-install setup (required)

`rexd` requires a config file with at least one `security.allowed_roots` entry. If this is missing, clients will fail to access files/commands as expected.

Default config path is `/etc/rexd/config.toml`.

```bash
sudo mkdir -p /etc/rexd
sudo curl -fsSL https://raw.githubusercontent.com/samiralibabic/rexd/main/rexd.example.toml -o /etc/rexd/config.toml
sudo $EDITOR /etc/rexd/config.toml
```

At minimum, update the `[[security.allowed_roots]]` paths to match the real directories you want to expose.

## Updating

Update the binary by rerunning the installer:

```bash
curl -fsSL https://raw.githubusercontent.com/samiralibabic/rexd/main/scripts/install.sh | bash
```

If `rexd` is managed as a service, restart it after update (example):

```bash
sudo systemctl restart rexd
```

If you also use `opencode-rexd-target`, update `rexd` on remote hosts first, then update the plugin.

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

Shell mode notes:

- Non-PTY `exec.start` with `shell=true` defaults to non-login shell behavior for predictable automation.
- Use `login=true` only for compatibility with legacy environments that require login-shell startup files.

Read file:

```json
{"jsonrpc":"2.0","id":3,"method":"fs.read","params":{"session_id":"s_1","path":"/srv/myapp/README.md"}}
```

Edit file:

```json
{"jsonrpc":"2.0","id":4,"method":"fs.edit","params":{"session_id":"s_1","path":"/srv/myapp/README.md","old_string":"Hello","new_string":"REXD","replace_all":false}}
```

Apply patch:

```json
{"jsonrpc":"2.0","id":5,"method":"fs.patch","params":{"session_id":"s_1","cwd":"/srv/myapp","patch_text":"*** Begin Patch\n*** Update File: README.md\n@@\n-Hello\n+REXD\n*** End Patch"}}
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
