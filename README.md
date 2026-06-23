# deptrust

deptrust is a CLI that checks package versions for known vulnerabilities across npm, PyPI, crates.io, and more.

It runs locally as a CLI and as an MCP server. It calls public package registry and OSV APIs directly; there is no hosted deptrust service to trust or configure.

This tool was born out of the frustration that is AI agents constantly using old versions.

## Contents

- [Scope](#scope)
- [CLI Usage](#cli-usage)
- [Install](#install)
- [Agent Setup](#agent-setup)
- [Manual MCP Setup](#manual-mcp-setup)
- [MCP Tools](#mcp-tools)
- [Skill-Only Use](#skill-only-use)
- [Troubleshooting](#troubleshooting)

## Scope

Supported ecosystems:

- npm, including scoped packages like `@clidey/ux`
- PyPI
- Cargo / crates.io

deptrust currently reports known vulnerabilities and gives a simple recommendation:

| Highest known severity | Recommendation |
| --- | --- |
| critical | block |
| high | block |
| medium / unknown | review |
| low | allow |
| none found | allow |

`allow` means no blocking known vulnerability was found in the public data sources. It does not prove that a package is safe.

deptrust also emits risk signals that are not CVEs. For example, a version published in the last 72 hours is marked for review so an agent does not blindly install a brand-new release.

Advisory providers are queried in parallel:

- OSV
- GitHub Advisory Database, including reviewed advisories and malware advisories

## CLI Usage

Check an exact version:

```bash
deptrust check npm lodash 4.17.20
```

Example normal response:

```text
npm lodash@4.17.20: 2 known vulnerabilities found
recommendation: block
risk_score: 80
```

Check the latest version:

```bash
deptrust check pypi requests latest
```

Return JSON:

```bash
deptrust check --json cargo serde latest
```

Example JSON response:

```json
{
  "ecosystem": "npm",
  "package": "lodash",
  "version": "4.17.20",
  "latest_version": "4.17.21",
  "known_vulnerabilities_found": true,
  "safe_to_use": false,
  "should_install": false,
  "risk_score": 80,
  "recommendation": "block",
  "classification": "vulnerable",
  "reason": "Found 2 known vulnerability records.",
  "next_action": "do_not_install; use suggest_safe_version or compare_versions to choose a safer version",
  "summary": "lodash 4.17.20 has 2 known vulnerabilities, including high severity. Block this exact version and prefer a fixed release.",
  "signals": [],
  "vulnerabilities": [
    {
      "id": "GHSA-35jh-r3h4-6jhm",
      "aliases": [
        "CVE-2021-23337"
      ],
      "cve_ids": [
        "CVE-2021-23337"
      ],
      "ghsa_ids": [
        "GHSA-35jh-r3h4-6jhm"
      ],
      "summary": "Command Injection in lodash",
      "severity": "high",
      "source": "OSV",
      "advisory_url": "https://github.com/advisories/GHSA-35jh-r3h4-6jhm",
      "affected_ranges": [
        "SEMVER: introduced 0, fixed 4.17.21"
      ],
      "fixed_versions": [
        "4.17.21"
      ],
      "references": [
        {
          "type": "ADVISORY",
          "url": "https://github.com/advisories/GHSA-35jh-r3h4-6jhm"
        }
      ]
    }
  ],
  "provider_errors": []
}
```

Suggest the latest version only when no known vulnerabilities are found:

```bash
deptrust suggest npm lodash
```

If the latest version is not allowed, `suggest` checks older known versions and returns the newest version with an `allow` recommendation.

Compare two versions:

```bash
deptrust compare npm lodash 4.17.20 4.17.21
```

Example compare response:

```text
lodash 4.17.20 -> 4.17.21 improves risk: score 80 to 0.
recommendation: allow
next_action: upgrade_to_target
```

Show the installed version:

```bash
deptrust version
```

## Install

The easiest install path is `npx` or `pnpx`:

```bash
pnpx @clidey/deptrust install --check
```

Go users can install directly:

```bash
go install github.com/clidey/deptrust/cmd/deptrust@latest
```

## Agent Setup

To install deptrust and register everything the installer can configure from your terminal:

```bash
npx @clidey/deptrust install --all
```

`--all` installs the binary, registers Codex MCP when the `codex` CLI is available, installs the Codex skill fallback, and registers Claude Code MCP when the `claude` CLI is available.

Use narrower installs when preferred:

```bash
npx @clidey/deptrust install --codex-mcp
npx @clidey/deptrust install --claude-code-mcp
npx @clidey/deptrust skills install
```

After MCP setup, ask your agent to use deptrust before dependency changes:

```text
Before installing or updating dependencies, check the exact package version with deptrust.
```

## Manual MCP Setup

If your client supports stdio MCP servers, configure it to run:

```bash
/absolute/path/to/deptrust mcp
```

Many clients use this JSON shape:

```json
{
  "mcpServers": {
    "deptrust": {
      "command": "/absolute/path/to/deptrust",
      "args": ["mcp"]
    }
  }
}
```

For Codex, you can also add it with:

```bash
codex mcp add deptrust -- /absolute/path/to/deptrust mcp
```

For Claude Code:

```bash
claude mcp add --transport stdio deptrust -- /absolute/path/to/deptrust mcp
```

## MCP Tools

### `check_package`

Checks a package version and returns known vulnerabilities plus a recommendation.

```json
{
  "ecosystem": "npm",
  "package": "lodash",
  "version": "4.17.20"
}
```

`version` may be omitted or set to `latest`. If an exact version does not exist, deptrust returns an error and suggests the latest explicit version.

### `suggest_safe_version`

Checks the latest version first. If latest is not allowed, checks older known versions and suggests the newest version with an `allow` recommendation.

```json
{
  "ecosystem": "npm",
  "package": "lodash"
}
```

### `compare_versions`

Compares a current version and target version, including resolved and added vulnerabilities.

```json
{
  "ecosystem": "npm",
  "package": "lodash",
  "from_version": "4.17.20",
  "to_version": "4.17.21"
}
```

## Skill-Only Use

If you do not want MCP, install the bundled Codex skill:

```bash
npx @clidey/deptrust skills install
```

The skill tells Codex to call the `deptrust` CLI before installing, updating, or recommending npm, PyPI, and Cargo packages.

## Troubleshooting

If `deptrust` is not found:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

If an MCP client cannot start the server, find the full path:

```bash
which deptrust
```

Then put that absolute path in the MCP config.

If a package check returns `unknown`, do not treat the package as safe. It usually means deptrust could not get a complete answer from a provider.
