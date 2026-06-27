## What this is

deptrust is a Go CLI and stdio MCP server that checks package versions for known vulnerabilities across 13 ecosystems (npm, pypi, cargo, go, rubygems, nuget, maven, packagist, pub, cocoapods, hex, hackage, github-actions). It calls public registry and advisory APIs (OSV, GitHub Advisory DB) directly — there is no hosted backend. It also ships as an npm package (`@clidey/deptrust`) whose `bin` is a Node installer that downloads the Go binary and registers MCP/skill integrations.

## Commands

```bash
go build -o deptrust ./cmd/deptrust   # build the binary
go test ./...                         # all tests — NOTE: integration_test.go hits live registries/network
go test ./internal/risk/...           # one package
go test ./internal/app/ -run TestName # one test
./lint.sh                             # golangci-lint with repo config (requires golangci-lint v2.12.2)
gofmt -l .                            # CI fails if this prints any file
```

CI (`.github/workflows/ci.yml`) runs `gofmt -l`, `go test ./...`, `go build`, and `npm pack --dry-run`. There is no `-short` gating — `internal/registry/integration_test.go` makes real network calls and runs as part of `go test ./...`.

## Architecture

The flow is **CLI/MCP entrypoint → `app.App` orchestrator → registry resolver + vulnerability providers → risk scorer**.

- `cmd/deptrust/main.go` — CLI argument parsing only. Subcommands: `check`, `suggest`, `compare`, `mcp`, `version`. Maps errors to exit codes by string-matching the message (`provider`→2, `not found`→3, `block`→10).
- `internal/app/app.go` — the orchestrator (`App`). `CheckPackage` resolves the version via the registry, fans out to all vulnerability providers **in parallel** (goroutines + channel), dedupes advisories by GHSA/CVE id, computes non-CVE signals (recent-release, mutable GitHub Actions tags), then calls `risk.Score`. `SuggestSafeVersion` and `CompareVersions` build on `checkResolved`. `App` holds an injectable `now func() time.Time` for deterministic signal tests.
- `internal/registry/` — one file per ecosystem (`npm.go`, `pypi.go`, …), all dispatched from `registry.go`'s `Resolve` switch. Each returns a `VersionInfo` (resolved version, latest, full version list, publish timestamps). `models.LatestVersion` is the sentinel for "latest". Shared helpers in `common.go`/`http.go`/`errors.go`.
- `internal/osv/` and `internal/github/` — vulnerability providers. Both implement the `vulnerabilityClient` interface (`Name()`, `Query()`) defined in `app.go`. Providers may also implement `ecosystemAwareProvider` (`Supports(ecosystem)`); unsupported ecosystems are reported as `skipped_providers` and drive `advisory_coverage` (`full`/`partial`/`none`/`error`). When no provider covers an ecosystem, the result is `unknown` — never treated as safe.
- `internal/risk/risk.go` — pure scoring. Maps highest severity to a recommendation: critical/high→`block`, medium/unknown→`review`, low/none→`allow`. Recommendation constants live here and are referenced across packages.
- `internal/models/models.go` — shared types, the `Ecosystem` enum, `NormalizeEcosystem` (string→enum), and `OSVEcosystem` (enum→OSV name).
- `internal/mcp/server.go` — hand-rolled JSON-RPC stdio MCP server (no SDK). Exposes `check_package`, `suggest_safe_version`, `compare_versions`. `compact.go` produces a slimmed-down response (counts + highest severity, no advisory bodies) to keep agent context small; the full response is available via the `full_response_command` field. `serverInstructions` is the prompt sent to agents on `initialize` — keep it in sync with the README's MCP section.

### Adding an ecosystem

1. Add the enum constant + `NormalizeEcosystem` + `OSVEcosystem` cases in `internal/models/models.go`.
2. Add `resolveX` in `internal/registry/x.go` and a case in `registry.go`'s `Resolve` switch.
3. Update provider `Supports()` in `internal/osv` / `internal/github` if coverage differs.
4. Update the ecosystem lists in `cmd/deptrust/main.go` (`printUsage`), `internal/mcp/server.go` (`ecosystemDescription`), and the README coverage table.

## Distribution

- `scripts/npm-install.mjs` — the `npm`/`npx` entrypoint (`install`, `uninstall`, `skills install`, integration flags). Downloads the platform binary and registers Codex/Claude Code MCP + skills.
- `scripts/build-release.sh`, `stamp-release-version.mjs`, `release-notes.sh` — release tooling; version is stamped into `internal/buildinfo` at build time via ldflags.
- `packaging/homebrew/` — Homebrew formula.
- `.agents/skills/` — bundled Codex skill (non-MCP fallback).
