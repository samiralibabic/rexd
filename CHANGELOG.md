# Changelog

## v0.1.4 - 2026-03-19

- Change non-PTY `exec.start` shell default to non-login mode (`sh -c`) for predictable automation.
- Add optional `login` flag to `exec.start`; `login=true` uses login-shell mode (`sh -lc`) for legacy compatibility.
- Add integration tests for default non-login shell behavior and explicit login-shell compatibility.
- Document the new default and compatibility behavior in protocol and README docs.
