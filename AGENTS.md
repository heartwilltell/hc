# AGENTS.md

## Cursor Cloud specific instructions

This repo is the `github.com/heartwilltell/hc` Go **library** for concurrent health checks. There is no server, database, binary, or UI to run — "running the app" means building and exercising the package via tests.

### Toolchain
- The module requires **Go 1.24** (`go.mod`). The base image's system Go may be older (1.22), so Go 1.24 is installed at `/usr/local/go` and symlinked to `/usr/local/bin/go` (which precedes `/usr/bin` on `PATH`). `go version` should report `go1.24.x`.
- Lint uses **golangci-lint v2** (`.golangci.yml` has `version: "2"`); v1 will not parse the config. It is installed in `$(go env GOPATH)/bin` and symlinked to `/usr/local/bin/golangci-lint`.

### Common commands (from repo root)
- Build: `go build ./...`
- Test (matches CI): `go test -race -cover ./...`
- Lint: `golangci-lint run --timeout=3m`

### Notes
- `golangci-lint run` currently reports one pre-existing finding on `master` (`hc.go:147` gocritic `deprecatedComment`); it is unrelated to environment setup.
