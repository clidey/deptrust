# deptrust

## Team Rules

These apply in every clidey repository, for people and agents alike.

- **Commits and pushes:** never commit or push without the user explicitly asking in that turn ("please commit this for me" or unambiguously equivalent). "Implement it" or "merge it into main locally" means change the files, not commit. If a commit seems necessary, stop and ask.
- **No AI attribution:** commit messages and PR descriptions carry no AI attribution: no `Co-Authored-By:` trailers for Claude, Codex, or any other agent, and no "Generated with" lines.
- **Untracked files:** never delete untracked or gitignored files without asking first; they often hold secrets or local state.
- **Agent replies:** lead with the answer or result. No preamble, no restating the question, no recap unless asked. Keep it short and explain only what isn't obvious; if you need something from the user, ask in one line.
- **Clarify first:** before starting, ask about anything where a different answer would change the work: scope, behaviour, which repo or environment, or anything hard to undo. Ask all your questions at once, then wait. Don't ask what the code or docs already answer; for small choices that are easy to change later, pick one and say which in your reply.

## What this is

deptrust is a Go CLI and stdio MCP server that checks package versions for known vulnerabilities across the ecosystems enumerated in `internal/models/models.go`. It calls public registry and advisory APIs (OSV, GitHub Advisory DB) directly — there is no hosted backend. It also ships as an npm package (`@clidey/deptrust`) whose `bin` is a Node installer that downloads the Go binary and registers MCP/skill integrations.

## Commands

```bash
go build -o deptrust ./cmd/deptrust   # build the binary
go test ./...                         # all tests — NOTE: integration_test.go hits live registries/network
./lint.sh                             # golangci-lint with repo config (requires golangci-lint v2.14.0)
gofmt -l .                            # CI fails if this prints any file
```

CI (`.github/workflows/ci.yml`) runs `./lint.sh`, a `gofmt -l` check, `go test ./...`, `go build`, and `npm pack --dry-run`. There is no `-short` gating — `internal/registry/integration_test.go` makes real network calls and runs as part of `go test ./...`.

## Architecture

The flow is **CLI/MCP entrypoint → `app.App` orchestrator → registry resolver + vulnerability providers → risk scorer**.

- `cmd/deptrust/main.go` — CLI argument parsing only. Subcommands: `check`, `suggest`, `compare`, `mcp`, `setup` (registers MCP and hooks, `setup_hooks.go`), `hook tool` (reads agent tool-hook JSON on stdin and vets install commands and GitHub Actions `uses:` lines via `internal/hook`), `version`. Maps errors to exit codes by string-matching the message (`provider`→2, `not found`→3, `block`→10), so keep those words in error messages.
- `internal/app/app.go` — the orchestrator (`App`). `CheckPackage` resolves the version via the registry, fans out to all vulnerability providers **in parallel** (goroutines + channel), dedupes advisories by GHSA/CVE id, computes non-CVE signals (recent-release, mutable GitHub Actions tags), then calls `risk.Score`. `SuggestSafeVersion` and `CompareVersions` build on `checkResolved`. `App` holds an injectable `now func() time.Time` for deterministic signal tests.
- `internal/registry/` — one file per ecosystem (`npm.go`, `pypi.go`, …), all dispatched from `registry.go`'s `Resolve` switch. Each returns a `VersionInfo` (resolved version, latest, full version list, publish timestamps). `models.LatestVersion` is the sentinel for "latest". Shared helpers in `common.go`/`http.go`/`errors.go`.
- `internal/osv/` and `internal/github/` — vulnerability providers. Both implement the `vulnerabilityClient` interface (`Name()`, `Query()`) defined in `app.go`. Providers may also implement `ecosystemAwareProvider` (`Supports(ecosystem)`); unsupported ecosystems are reported as `skipped_providers` and drive `advisory_coverage` (`full`/`partial`/`none`/`error`). When no provider covers an ecosystem, the result is `unknown` — never treated as safe.
- `internal/risk/risk.go` — pure scoring. Maps highest severity to a recommendation: critical/high→`block`, medium/unknown→`review`, low/none→`allow`. Recommendation constants live here and are referenced across packages.
- `internal/models/models.go` — shared types, the `Ecosystem` enum, `NormalizeEcosystem` (string→enum), and `OSVEcosystem` (enum→OSV name).
- `internal/mcp/server.go` — hand-rolled JSON-RPC stdio MCP server (no SDK). Exposes `check_package`, `suggest_safe_version`, `compare_versions`. `compact.go` produces a slimmed-down response (counts + highest severity, no advisory bodies) to keep agent context small; the full response is available via the `full_response_command` field. `serverInstructions` is the prompt sent to agents on `initialize` — keep it in sync with the README's MCP section.

### Adding an ecosystem

1. Add the enum constant + `NormalizeEcosystem` + `OSVEcosystem` cases in `internal/models/models.go`.
2. Add `resolveX` in `internal/registry/<ecosystem>.go` and a case in `registry.go`'s `Resolve` switch.
3. Update provider `Supports()` in `internal/osv` / `internal/github` if coverage differs.
4. Update the ecosystem lists in `cmd/deptrust/main.go` (`printUsage`), `internal/mcp/server.go` (`ecosystemDescription`), and the README coverage table; `TestEcosystemListsNameEveryEcosystem` (`internal/models/ecosystem_lists_test.go`) fails until all three match `models.go`.

## Distribution

- `scripts/npm-install.mjs` — the `npm`/`npx` entrypoint (`install`, `uninstall`, `skills install`, integration flags). Downloads the platform binary and registers Codex/Claude Code MCP + skills.
- Release tooling: `scripts/build-release.sh`, `scripts/stamp-release-version.mjs`, `scripts/release-notes.sh`; the version is stamped into `internal/buildinfo` via ldflags as the `v`-prefixed tag.
- Homebrew: `packaging/homebrew/deptrust.rb.tmpl`, filled by `scripts/generate-homebrew-formula.sh <tag> <checksums.txt> [out]`; the release workflow pushes the formula to the tap (needs `HOMEBREW_TAP_TOKEN`). Because the binary reports the `v`-prefixed tag, the formula test asserts `deptrust v#{version}`.
- `.agents/skills/deptrust-package-check/` — bundled Codex skill (non-MCP fallback).
