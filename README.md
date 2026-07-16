# deptrust

```text
     __           __                  __
 ___/ /___  ___  / /________  _______/ /_
/ _  / __ \/ _ \/ __/ ___/ / / / ___/ __/
/  __/ /_/ /  __/ /_/ /  / /_/ (__  ) /_
\__,_/\____/ .___/\__/_/   \__,_/____/\__/
           /_/
```

deptrust is a CLI that checks package versions for known vulnerabilities across npm, PyPI, crates.io, Go modules, RubyGems, NuGet, Maven, Packagist, pub.dev, CocoaPods, Hex.pm, Hackage, GitHub Actions, and more.

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
- Go modules
- RubyGems
- NuGet
- Maven, using `groupId:artifactId` package names
- Packagist / Composer, using `vendor/package` package names
- pub.dev
- CocoaPods
- Hex.pm
- Hackage
- GitHub Actions, using `owner/repo` package names and tags, branch refs, or commit SHAs as versions

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

Provider coverage varies by ecosystem. If deptrust can resolve registry metadata but no configured vulnerability provider supports that ecosystem, it returns `unknown` instead of treating the package as safe.

Provider coverage:

| Ecosystem | Registry metadata | OSV | GitHub Advisory DB |
| --- | --- | --- | --- |
| npm | yes | yes | yes |
| PyPI | yes | yes | yes |
| Cargo / crates.io | yes | yes | yes |
| Go modules | yes | yes | yes |
| RubyGems | yes | yes | yes |
| NuGet | yes | yes | yes |
| Maven | yes | yes | yes |
| Packagist / Composer | yes | yes | yes |
| pub.dev | yes | yes | yes |
| CocoaPods | yes | no | yes |
| Hex.pm | yes | yes | yes |
| Hackage | yes | yes | no |
| GitHub Actions | yes | yes | yes |

The JSON output includes advisory coverage fields:

- `checked_providers`: vulnerability providers deptrust actually queried
- `skipped_providers`: configured providers skipped because the ecosystem is unsupported
- `advisory_coverage`: `full`, `partial`, `none`, or `error`
- `advisory_coverage_reason`: short explanation for the coverage value
- `registry_verification`: `verified` when registry metadata confirmed the version, or `unverified` when an exact-version check continued after a transient registry failure
- `registry_verification_reason`: the registry error when verification was unavailable

An exact-version check still queries advisory providers when registry verification is temporarily unavailable. That result is always non-installable and never receives an `allow` recommendation. Checks for `latest`, unknown packages, and definitively nonexistent versions still require successful registry resolution.

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

Check a Go module:

```bash
deptrust check go golang.org/x/crypto latest
```

Check RubyGems, NuGet, or Maven:

```bash
deptrust check rubygems rails latest
deptrust check nuget Newtonsoft.Json latest
deptrust check maven org.apache.logging.log4j:log4j-core latest
```

Check Packagist, pub.dev, CocoaPods, Hex.pm, Hackage, or GitHub Actions:

```bash
deptrust check packagist monolog/monolog latest
deptrust check pub http latest
deptrust check cocoapods AFNetworking latest
deptrust check hex plug latest
deptrust check hackage aeson latest
deptrust check github-actions actions/checkout v7.0.0
deptrust check github-actions actions/checkout main
```

For GitHub Actions, full commit SHAs are treated as pinned. Full semver tags such as `v4.2.2` are accepted without an extra pinning signal. Major-only tags such as `v4` and branch refs such as `main` are valid refs, but deptrust adds a review signal because they can move.

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
  "checked_providers": [
    "OSV",
    "GitHub Advisory DB"
  ],
  "skipped_providers": [],
  "advisory_coverage": "full",
  "advisory_coverage_reason": "all configured vulnerability providers were checked",
  "registry_verification": "verified",
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

When advisories include fixed versions, `suggest` checks those provider-reported fixed versions first before walking back through the registry version list.

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
npx @clidey/deptrust install
pnpx @clidey/deptrust@latest install
```

The default installer is guided. It installs the binary, asks which agent integrations to configure, prints the user-level destinations before changing anything, and asks for confirmation. The guided installer defaults to enabling MCP, the skill fallback, and dependency safety hooks for Codex and Claude Code. Add `--yes` for non-interactive binary-only installs, or pass explicit integration flags.

To remove the user-level binary, skill, and MCP entries:

```bash
npx @clidey/deptrust uninstall
pnpx @clidey/deptrust@latest uninstall
```

Homebrew users can install from the Clidey tap:

```bash
brew install clidey/tap/deptrust
```

Or tap first, then install and upgrade as usual:

```bash
brew tap clidey/tap
brew install deptrust
brew upgrade deptrust
```

Go users can install directly:

```bash
go install github.com/clidey/deptrust/cmd/deptrust@latest
```

## Agent Setup

To install deptrust and register everything the installer can configure without the guided prompts:

```bash
npx @clidey/deptrust install --all
pnpx @clidey/deptrust@latest install --all
```

`--all` installs the binary, registers Codex MCP when the `codex` CLI is available, installs the Codex skill fallback, registers Claude Code MCP when the `claude` CLI is available, and installs the Codex and Claude Code dependency safety hooks.

The hooks are `PreToolUse` hooks. They check package install commands before they run, and they also check GitHub Actions added to workflow files through agent file-edit tools. A hook blocks the tool call when deptrust returns `review`, `block`, or `unknown`. The installer writes user-level hook config only: `~/.codex/hooks.json` for Codex and `~/.claude/settings.json` for Claude Code.

Use narrower installs when preferred:

```bash
npx @clidey/deptrust install --codex-mcp
npx @clidey/deptrust install --claude-code-mcp
npx @clidey/deptrust skills install
pnpx @clidey/deptrust@latest install --codex-mcp
pnpx @clidey/deptrust@latest install --claude-code-mcp
pnpx @clidey/deptrust@latest skills install
```

After MCP setup, agents will automatically check packages before recommending updates or changes. The MCP server sends instructions to vet all dependency versions — including answering questions like "what can I update" or "which dependencies are safe to upgrade" — before providing recommendations.

If using deptrust in a non-MCP context, remind your agent:

```text
Before listing, comparing, or recommending specific package versions, check them with deptrust. This includes answering "what can I update" — do not provide version recommendations until after checking for known vulnerabilities.
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

On `initialize`, the server returns MCP `instructions` telling the agent when to reach for these tools (before adding, bumping, or recommending a dependency, or when asked whether a version is safe to update). Clients that surface server instructions will apply this automatically, so the manual reminder above is optional rather than required.

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

MCP output is intentionally compact so agents can decide whether to install a dependency without pulling full advisory bodies into context. If the user asks to see full details, the agent can run the `full_response_command`.

Example compact MCP structured output:

```json
{
  "ecosystem": "npm",
  "package": "vite",
  "version": "7.0.0",
  "latest_version": "8.0.16",
  "known_vulnerabilities_found": true,
  "safe_to_use": false,
  "should_install": false,
  "risk_score": 80,
  "classification": "vulnerable",
  "recommendation": "block",
  "reason": "Found 7 known vulnerability records.",
  "next_action": "do_not_install; use suggest_safe_version or compare_versions to choose a safer version",
  "summary": "vite 7.0.0 has 7 known vulnerabilities, including high severity. Block this exact version and prefer a fixed release.",
  "vulnerability_count": 7,
  "vulnerability_counts": {
    "critical": 0,
    "high": 2,
    "medium": 3,
    "low": 2,
    "unknown": 0
  },
  "highest_severity": "high",
  "checked_providers": [
    "OSV",
    "GitHub Advisory DB"
  ],
  "skipped_providers": [],
  "advisory_coverage": "full",
  "advisory_coverage_reason": "all configured vulnerability providers were checked",
  "registry_verification": "verified",
  "full_response_command": "deptrust check --json npm vite 7.0.0"
}
```

The compact MCP response omits the vulnerability array, advisory `details`, and repeated `references`. Agents should use the counts, highest severity, provider coverage, recommendation, and next action by default. If the user asks for full advisory details, run the `full_response_command`.

### `suggest_safe_version`

Checks the latest version first. If latest is not allowed, checks provider-reported fixed versions first, then older known versions, and suggests the newest version with an `allow` recommendation.

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

The skill tells Codex to call the `deptrust` CLI before installing, updating, or recommending npm, PyPI, Cargo, Go module, RubyGems, NuGet, Maven, Packagist, pub.dev, CocoaPods, Hex.pm, Hackage, and GitHub Actions packages.

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

If a package check returns `unknown`, do not treat the package as safe. It means deptrust could not get a complete answer from an advisory provider or could not verify the exact version with its registry.
