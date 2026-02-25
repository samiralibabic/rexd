# Contributing

Thanks for contributing to `rexd`.

## Getting started

1. Fork and clone the repository.
2. Install Go (version from `go.mod`).
3. Run:
   ```bash
   make tidy
   make test
   make verify
   ```

## Pull request checklist

- Keep changes focused and small.
- Add or update tests when behavior changes.
- Ensure `go test ./...` passes.
- Run `make verify` before opening a PR.
- Update docs when adding/changing protocol behavior.

## Commit style

- Use clear, imperative commit messages.
- Explain the "why" in PR descriptions, not just the "what".

## Reporting issues

- Include repro steps, expected behavior, and actual behavior.
- If possible, include config snippets and request/response examples.
