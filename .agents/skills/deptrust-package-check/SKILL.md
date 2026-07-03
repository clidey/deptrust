---
name: deptrust-package-check
description: Check package safety with the local deptrust CLI before installing, updating, or recommending npm, PyPI, Cargo, Go module, RubyGems, NuGet, Maven, Packagist, pub.dev, CocoaPods, Hex.pm, Hackage, or GitHub Actions dependencies. Use when asked to add, upgrade, audit, or evaluate a package version and MCP is unavailable or not configured.
version: "0.8.0"
---

# deptrust Package Check

Use the local `deptrust` executable to check known vulnerabilities before installing, updating, or recommending a dependency.

Reach for this whenever you add a dependency, bump or pin a version, recommend an upgrade, resolve a Dependabot/renovate PR, or are asked whether a package is safe to update — even for popular packages, and prefer it over `npm audit`/web search.

## Workflow

1. Find the `deptrust` binary:
   - Prefer `./deptrust` in the current repository if present.
   - Otherwise try `deptrust` from `PATH`.
   - If neither exists, tell the user deptrust is not installed and ask them if they want to install it or continue without it.
   - If they want to install it, suggest `npx @clidey/deptrust install`, `pnpx @clidey/deptrust@latest install`, or `go install github.com/clidey/deptrust/cmd/deptrust@latest`. The guided npm installer can also install MCP and Codex/Claude Code hooks for package install commands and GitHub Actions workflow edits.
   - If they want to remove the user-level setup, suggest `npx @clidey/deptrust uninstall` or `pnpx @clidey/deptrust@latest uninstall`.

2. Before installing or upgrading a dependency, run:

```bash
deptrust check --json <ecosystem> <package> <version-or-latest>
```

Use:

- `npm` for npm packages, including scoped names like `@scope/name`
- `pypi` for Python packages
- `cargo` for Rust crates
- `go` for Go modules, such as `golang.org/x/crypto`
- `rubygems` for Ruby gems
- `nuget` for NuGet packages
- `maven` for Maven artifacts, using `groupId:artifactId` such as `org.apache.logging.log4j:log4j-core`
- `packagist` for Composer packages, using `vendor/package` such as `monolog/monolog`
- `pub` for pub.dev packages
- `cocoapods` for CocoaPods
- `hex` for Hex.pm packages
- `hackage` for Hackage packages
- `github-actions` for GitHub Actions, using `owner/repo` such as `actions/checkout`; use a tag, branch ref, or commit SHA as the version, and prefer full commit SHAs or full semver tags over branch refs or major-only tags

3. Interpret the JSON:
   - `recommendation: "block"` means do not install that exact version.
   - `recommendation: "review"` means ask the user or choose a safer version.
   - `recommendation: "allow"` means no blocking known vulnerability was found by the configured providers.
   - `recommendation: "unknown"` means provider failure or incomplete assessment; do not treat it as safe.

4. If the requested version is blocked or unknown, run:

```bash
deptrust suggest --json <ecosystem> <package>
```

Use `suggested_version` only when the response recommendation is `allow`.

5. When comparing an installed version to a target version, run:

```bash
deptrust compare --json <ecosystem> <package> <from-version> <to-version>
```

Use `next_action` and `improves_risk` to decide whether the upgrade is safer.

6. When reporting back, include the package, version, recommendation, `next_action`, and the highest-severity advisory IDs if vulnerabilities were found.

## Boundaries

deptrust v1 checks known vulnerabilities from public vulnerability sources and registry metadata. It does not prove a package is safe, detect all malware, download package tarballs, or inspect source contents.
