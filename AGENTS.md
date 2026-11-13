# deptrust

## Team Rules

These apply in every clidey repository, for people and agents alike.

- **Commits:** ask once before the first commit. Once the user says yes, keep committing that work without asking again; push only when they've asked for it.
- **No AI attribution, anywhere:** commits, PRs, issues, comments and releases carry no `Co-Authored-By:` for Claude, Codex or any other agent, and no "Generated with" lines.
- **External posts:** before posting anything outside this machine (GitHub issues, PRs, comments and reviews, advisory comments or edits, chat or email), show the user the exact text and wait for a yes.
- **Never, whatever the instruction:** publish packages or releases, push images, deploy to UAT or production, or run anything against production. Build images and run build scripts only locally, to test. Prepare the command and hand it to the user.
- **Cluster and cloud commands:** read-only `kubectl`, `helm` and `gcloud` commands run without asking, and in UAT so do day-to-day operations: restarting a rollout, deleting pods, config maps and jobs, scaling, `exec`, `cp` and port-forwarding. Never anything against production. Anything else that writes, deletes or shows secret values (including `apply`, `patch`, `set` and rollbacks), and anything that writes to a WhoDB organisation: show the exact command and wait for a yes.
- **Branches and worktrees:** never create a branch or worktree, or switch branches, unless the user asked. If the work seems to need one, ask first; this is a required question.
- **Untracked files:** ask before deleting untracked or gitignored files; they often hold secrets or local state.
- **pnpm only:** never npm, npx, yarn or bun.
- **Stay in the workspace:** use only the clones in the workspace; never read or run checkouts elsewhere, such as old ones in `~/Documents`.
- **The machine:** don't keep it awake or change power settings (`caffeinate`, `pmset`); recommend it to the user instead.
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

Privacy regression tests prove a private source never reaches a public lookup: `App` with a nil registry and `forbiddenPrivateProvider` (panics if queried) in `internal/app`, the `sourcetest.Guard` HTTP tripwire, `credential-sentinel` values that must never appear in output, and the `e2e/` proxy tripwire (`DEPTRUST_TEST_E2E=1`). Reuse these for new source handling.

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
