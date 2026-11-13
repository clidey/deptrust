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

HTTP requests retry `429`, `502`, `503`, and `504` responses up to three total attempts. Retries use short exponential delays and honor `Retry-After` values up to two seconds; longer server-requested waits fail fast so the CLI does not hang. Exhausted advisory retries make the result incomplete and prevent an `allow` recommendation.

### GitHub API authentication

GitHub Advisory Database and GitHub Actions API requests can use a short-lived, least-privilege GitHub App token. In CI, pass it through `DEPTRUST_GITHUB_TOKEN`:

```bash
DEPTRUST_GITHUB_TOKEN="$GITHUB_APP_TOKEN" deptrust check npm lodash 4.17.20
```

The credential precedence is `DEPTRUST_GITHUB_TOKEN`, `GITHUB_TOKEN`, then `GH_TOKEN`. For local use, the optional GitHub CLI fallback is enabled explicitly with `DEPTRUST_GITHUB_AUTH=gh deptrust check ...`; it runs `gh auth token` without prompting. If no credential is available, DepTrust continues unauthenticated. A GitHub API rate-limit or permission failure produces `unknown` with diagnostics and is never treated as an OSV-only success.

DepTrust never stores, bundles, caches, logs, telemeters, or emits GitHub tokens. Authentication headers are sent only to `https://api.github.com`.

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

The default installer is guided. It installs the binary, asks which agent integrations to configure, prints the user-level destinations before changing anything, and asks for confirmation. The guided installer defaults to enabling MCP, the skill fallback, and dependency safety hooks for Codex and Claude Code. Add `--yes` to install without prompts or enabling new integrations, or pass explicit integration flags. Existing registrations are refreshed during installation.

Rerunning the installer is safe. It silently leaves unchanged MCP, skill, and hook configuration alone. If an integration points at an old deptrust binary or its managed configuration has changed, the installer updates it automatically; users do not need to manually remove and re-add MCP servers. Customized skills are backed up before replacement.

To upgrade the binary and refresh your existing agent setup, run one command:

```bash
npx @clidey/deptrust@latest upgrade
```

No setup questions or integration flags are needed. Existing local MCP and hook
registrations are updated in place, installed skills are refreshed with backups,
and integrations you haven't enabled stay disabled. The installer verifies the
downloaded binary's version before replacing the old executable at the same
location. If you originally chose a custom `--bin-dir`, pass that same directory
when upgrading. Updating the npm wrapper alone does not update the Go binary;
the command above handles both. Restart active agent sessions to reload any
already-running MCP server.

The npm installer and `scripts/install.sh` automatically refresh integrations
you already enabled. They do not ask again or enable new integrations. If an
automatic refresh fails, the binary installation still succeeds and prints
the exact `setup --refresh` repair command; existing configuration errors are
reported instead of silently discarded.

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

Homebrew prints a reminder after installation. To run the guided Codex and Claude Code setup using the Homebrew binary itself (MCP registrations and dependency safety hooks):

```bash
deptrust setup
```

The guided setup asks before enabling MCP and dependency safety hooks. It leaves registrations that already use the current binary alone, and reconciles existing DepTrust registrations that point to an older npm, Homebrew, or source-install path.

Homebrew setup uses the stable `opt/deptrust/bin/deptrust` path when available,
so subsequent `brew upgrade deptrust` runs do not need another guided setup. Existing
registrations that still contain a versioned Cellar path can be migrated once
with `deptrust setup --refresh`. This non-interactive command only refreshes
existing local user registrations; remote MCP servers and custom wrappers are
left alone. It preserves MCP environment settings, hook options, and unrelated
configuration.

After upgrading through Homebrew, Go, Nix, a manual download, or another package
manager, `deptrust setup --refresh` also updates any existing package-check skill
from the binary itself. It needs neither npm nor a source checkout. Changed
skills are backed up in full before replacement; unchanged skills are left
alone. It never enables an absent skill. For a first-time skill installation,
run `deptrust setup --skill` explicitly.

Homebrew's isolated post-install environment cannot reliably access user agent
settings, so the formula prints the refresh command instead of attempting it
inside post-install. Direct `go install`, Nix profile updates, and manual binary
replacement also require running `deptrust setup --refresh` explicitly when you
want to refresh installed skill files or migrate registrations. No background
updater or automatic agent restart is installed.

Go users can install directly:

```bash
go install github.com/clidey/deptrust/cmd/deptrust@latest
```

### Nix

The project provides optional Nix flake outputs for users who already use Nix. The flake wraps the prebuilt release binary.

```bash
# Run without installing
nix run github:clidey/deptrust

# Install into your profile
nix profile install github:clidey/deptrust
```

The normal [release workflow](.github/workflows/release.yml) generates the Nix
hashes from the same archives it publishes, evaluates the flake before
publishing, then builds and runs it against the published assets before updating
the default branch. `github:clidey/deptrust` may briefly lag while that workflow
is running. Release tags point to the source commit from before the generated
flake update and may still reference the previous binary; pin a commit whose
`flake.nix` contains the version you need when reproducibility matters.

### Devbox

For reproducible development environments, use Devbox:

```bash
# Install Devbox first (if not already installed)
curl -fsSL https://get.jetify.dev/devbox | bash

# Initialize the environment
devbox shell

# Build the project
devbox run build
```

`devbox.json` constrains the toolchain version and the committed `devbox.lock`
pins the exact package versions and nixpkgs revisions. Run `devbox update` when
you intentionally want to refresh those pins.

Or install Devbox via Homebrew:

```bash
brew install jetify-com/devbox/devbox
```

## Agent Setup

To install deptrust and register everything the installer can configure without the guided prompts:

```bash
npx @clidey/deptrust install --all
pnpx @clidey/deptrust@latest install --all
```

`--all` installs the binary, registers Codex MCP when the `codex` CLI is available, installs the Codex skill fallback, registers Claude Code MCP when the `claude` CLI is available, and installs the Codex and Claude Code dependency safety hooks.

The hooks are `PreToolUse` hooks. They check package install commands before they run, and they also check GitHub Actions added to workflow files through agent file-edit tools. A hook blocks the tool call when deptrust returns `review`, `block`, or `unknown`. The installer writes user-level hook config only: `~/.codex/hooks.json` for Codex and `~/.claude/settings.json` for Claude Code.

Install inspection accepts a deliberately simple format: one standalone command
with literal package names and versions, such as `pnpm add @scope/pkg@1.2.3`,
`pip install requests==2.32.3`, or
`dotnet add package Example --version 1.0.0`. Multiple literal packages are
supported where the package manager allows them. An omitted version means
`latest`. The hook does not attempt to interpret shell programs.

If an install-like command contains redirects (including `2>&1`), command
chaining, quotes, variables, substitutions, ranges, local paths, wrappers, or
unsupported options, the hook denies that attempt before package lookups and asks
the agent to retry with simpler input. It never checks a partial package list and
treats that as complete. Directory changes should be separate tool calls. Only
the version options shown above (`--version`, or `-v` for Ruby tools) and Maven's
`dependency:get -Dartifact=group:artifact:version` form are accepted as options;
other necessary options and source settings require an explicit source-aware
check, not silently removing their meaning from the install.

This is inspection of recognized explicit package installs, not a shell security
boundary or a complete detector of arbitrary scripts. Install commands without
explicit package names do not audit the manifest or lockfile. Unknown tools,
aliases and dynamically constructed command names are outside this hook's
coverage; use explicit checks for their dependencies.

When the `gh` CLI is available, guided setup also offers to use its existing local login for hook checks. This writes only `DEPTRUST_GITHUB_AUTH=gh`, never a GitHub token, so hook subprocesses can avoid unauthenticated GitHub API rate limits.

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

For CI, configure a short-lived least-privilege GitHub App token as `DEPTRUST_GITHUB_TOKEN` for the process running DepTrust. For local GitHub CLI authentication, use `DEPTRUST_GITHUB_AUTH=gh deptrust check ...`. DepTrust never stores tokens.

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

When GitHub advisory access is rate-limited or unavailable, MCP returns `unknown`. The agent should proactively offer to configure a token and retry, skip or defer the version, or proceed only after the user explicitly accepts the unresolved GitHub coverage risk for that exact version. That exception must remain clearly labeled as user-accepted uncertainty; it must not be reported as `allow` or as proof that the version is safe.

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

### Private Go modules

Configure `GOPRIVATE` through the environment or `go env -w` to identify private
Go module paths:

```sh
export GOPRIVATE='github.com/your-org/*,git.corp.example.com'
deptrust check go github.com/your-org/internal-lib v1.2.3
```

DepTrust uses Go's comma-separated module-prefix glob semantics. Matching Go
modules skip all public registry and advisory requests and return `unknown`,
with registry verification `unverified` and advisory coverage `none`. Suggestions
and comparisons also skip these requests and cannot recommend a version.
This does not verify the module's existence, authenticate to its repository, or
scan its dependencies. Other ecosystems and nonmatching Go modules retain their
normal behavior.

A nonempty exported `GOPRIVATE` takes precedence. Otherwise DepTrust runs
`go env GOPRIVATE` to read the effective saved configuration, including `GOENV`
overrides and `GOENV=off`. This requires Go on PATH unless a nonempty `GOPRIVATE`
is exported. Failure to read the configuration stops before public requests.
Toolchain downloads and Go telemetry are disabled for this configuration read.
For MCP, configure the environment of the server process. `GONOPROXY` and `GONOSUMDB` do not override this privacy
policy. Configure patterns before checking private names: unmarked paths use
public services.

See the [official Go private-module documentation](https://go.dev/ref/mod#private-modules)
for the underlying Go configuration and pattern semantics.

### npm and NuGet package sources

Select configuration explicitly before checking packages that might come from
custom feeds. These options apply to `check`, `suggest`, and `compare`:

```sh
deptrust check --json --project-dir /absolute/path/to/project npm @company/library 1.2.3
deptrust check --json --nuget-config /absolute/path/to/nuget.config nuget Company.Library 1.2.3
```

For MCP, pass `project_dir` (npm) or `nuget_config` (NuGet) on each tool call.
Follow-up commands retain the selected configuration. No project directory is
implicitly taken from the MCP server's working directory. Without these options,
checks continue to use the public registries.

- **npm:** requires npm on PATH and reads its effective configuration in the
  selected directory using `npm config get`. Scope-specific registry mappings
  take precedence over the default registry. npm handles its normal project,
  user, global, and environment configuration. This does not infer privacy from
  a scope name or `package.json` publishing restrictions. Private packages hosted
  on registry.npmjs.org cannot be distinguished by registry selection alone.
- **NuGet:** reads the selected config file alone, like NuGet's explicit
  `--configfile` mode; machine, user, and parent-directory settings are not merged in this explicit-file mode.
  Use a consolidated config containing package sources and mappings. Exact package
  IDs take precedence over prefixes, longer prefixes over shorter ones, and `*`
  is the default. Matching is case-insensitive. Disabled sources are excluded.
  Ordered `add`, `remove`, and `clear` operations are applied to source/disabled
  collections; source mappings support collection clears. Duplicate sections and
  unsupported mapping directives are rejected before network access.

Only an exclusive selection of the supported public source (npmjs.org or the
NuGet v3 public index) proceeds to public checks. Custom, mixed, or unmapped
sources return `unknown` with registry verification `unverified` and advisory
coverage `none`. They are not labeled private: a custom feed may also be public
or mirror public packages. Configuration errors stop the operation before public
requests. DepTrust does not fetch from custom feeds or expose their URLs or
credentials in results. This is source selection support, not authenticated
private-package assessment or a dependency-graph scan.

Official references: [npm scope registry mappings](https://docs.npmjs.com/cli/v11/using-npm/scope/),
[npm configuration](https://docs.npmjs.com/cli/v11/using-npm/config/),
[NuGet source mapping and precedence](https://learn.microsoft.com/en-us/nuget/consume-packages/package-source-mapping),
[NuGet configuration selection](https://learn.microsoft.com/en-us/nuget/consume-packages/configuring-nuget-behavior).

### Cargo and Poetry package sources

The same explicit `--project-dir` option (MCP: `project_dir`) supports Cargo and
Poetry projects:

```sh
deptrust check --json --project-dir /absolute/path/to/rust-project cargo internal-crate 1.2.3
deptrust check --json --project-dir /absolute/path/to/python-project pypi internal-package 1.2.3
```

- **Cargo:** requires Cargo on PATH. Reads workspace dependency declarations with
  `cargo metadata --format-version 1 --no-deps --offline`. Alternate registries,
  Git/path dependencies, workspace packages, aliases queried instead of actual
  crate names, and names not declared as workspace dependencies return `unknown`.
  Use the actual crate name for renamed dependencies. Before permitting a public
  check, resolved sources are confirmed with offline, locked metadata so that
  local/Git patches cannot be mistaken for crates.io artifacts. This requires a
  current lockfile and cached metadata for the resolved graph; failures stop
  before public requests. This does not build code or fetch missing dependencies.
  It does not scan arbitrary transitive dependencies or authenticate to registries.
- **Poetry:** uses a native Go TOML parser; Python is not required. Reads `pyproject.toml` without executing
  project code or invoking installation. Supports explicit `source` constraints,
  dependency groups, optional dependencies, legacy dev-dependencies, and Poetry
  enrichment of PEP 621 dependencies. PyPI-bound dependencies can proceed; custom source constraints
  and direct Git/path/URL dependencies return `unknown`. Unconstrained packages
  also return `unknown` if custom primary/supplemental sources could supply them.
  Unused explicit sources do not affect other dependencies. Source constraints
  are not inherited by dependencies. Undeclared packages return `unknown`; this
  does not scan a lockfile or resolve transitive dependencies. Deprecated Poetry
  `default`/`secondary` priority settings are rejected rather than guessed.

Both options work for checks, suggestions, and comparisons, retain project
selection in MCP follow-up commands, and skip public registry/advisory requests
when source identity is unsupported or uncertain. Parser/tool errors stop before
public requests. Calls without project targeting retain public-registry behavior.

Official references: [Go environment configuration](https://pkg.go.dev/cmd/go#hdr-Environment_variables),
[Cargo metadata](https://doc.rust-lang.org/cargo/commands/cargo-metadata.html),
[Poetry package source constraints](https://python-poetry.org/docs/repositories/#package-source-constraint),
[Go TOML parser](https://github.com/pelletier/go-toml).

### Bundler locked gem sources

Select a resolved Bundler lockfile explicitly:

```sh
deptrust check --json --bundler-lock /absolute/path/to/Gemfile.lock rubygems internal_gem 1.2.3
```

Use `bundler_lock` in MCP calls. The option works for checks, suggestions, and
comparisons; follow-up commands preserve it. It cannot be combined with
`project_dir` or `nuget_config`.

DepTrust uses the installed Ruby/Bundler `LockfileParser` to read resolved gem
sources. This follows the source recorded after Bundler resolves per-gem `source:`
options and source blocks. Only gems exclusively associated with the HTTPS
RubyGems.org source proceed to public checks. Custom sources, mixed remotes,
missing gems, and source uncertainty return `unknown` without public requests.
Git, path, and plugin source objects are not constructed. Their locked gem names
remain unsupported, while unrelated RubyGems.org gems can still be assessed.
The same gem name appearing in both public and unsupported sources returns `unknown`. Credential URLs never appear in results.

This inspects the selected artifact only. It does not evaluate Gemfile, verify
that the lockfile matches current Gemfile declarations, apply local source
configuration overrides, install gems, run plugins, or update the lockfile.
Keep the selected lockfile current before relying on its source assignments.
A newer version suggested from a public source is not proof of compatibility with
Gemfile constraints. Calls without this option retain public-registry behavior.
Ruby and Bundler must be installed on PATH; unavailable tooling or an unreadable
lockfile stops the operation before public requests. `RUBYOPT` startup options
are disabled for the parser subprocess.

Official references: [Bundler per-gem sources and source blocks](https://guides.rubygems.org/gemfile/),
[resolved lockfile sources](https://guides.rubygems.org/gemfile-lock/).

Hook source checks use an absolute hook `cwd` for npm configuration. Install
commands with explicit registry/index/configuration/directory overrides stop
before public requests because the hook cannot reliably reconstruct those
sources. Use the explicit source-aware CLI/MCP interface for those assessments.

### Composer repository declarations

Use `--source-file /absolute/path/to/composer.json` (MCP: `source_file`) to assess
repository declarations in the selected root artifact. Custom repositories that
can supply the package return `unknown`, including noncanonical repositories;
`only` and `exclude` wildcard filters are respected. Disabling Packagist also
returns `unknown`. A VCS/dist URL in composer.lock is not treated as proof that a
package came from Packagist. No Composer scripts or plugins run.

This is a selected-file assessment: it does not merge Composer's global config,
environment overrides, or plugins. Use a configuration artifact representing the
intended sources. It does not prove the freshness or provenance of installed
packages. Source context options are mutually exclusive.

Official references: [Composer repositories](https://getcomposer.org/doc/05-repositories.md),
[repository priority and filters](https://getcomposer.org/doc/articles/repository-priorities.md).

### Dart/pub sources

`--source-file /absolute/path/to/pubspec.yaml` or `pubspec.lock` (MCP:
`source_file`) reads declared or locked Dart sources natively. Hosted pub.dev
identities can proceed; custom hosted URLs, Git/path/SDK dependencies, workspace
inheritance, missing packages, and unsupported identities return `unknown`.
`dependency_overrides` in the selected pubspec take precedence. A custom
`PUB_HOSTED_URL` always prevents public checks, even without a selected file.
The selected artifact is not merged with pubspec_overrides.yaml or workspace
configuration; supply the intended artifact and do not treat this as freshness
verification. No Dart commands, downloads, or project code are executed.

Official references: [Dart dependency sources](https://dart.dev/tools/pub/dependencies),
[pub environment](https://dart.dev/tools/pub/environment-variables).

### uv, Pipenv, and pip

For Python, `--source-file` additionally accepts `uv.lock`, uv `pyproject.toml`,
`Pipfile`, `Pipfile.lock`, `pip.conf`, or `pip.ini`. uv index constraints and locked
registry identities, Pipenv index assignments, and selected pip global/install
index settings are read natively. Custom, direct, ambiguous, or undeclared
identities return `unknown`. Explicit uv indexes unused by a dependency do not
restrict that dependency. Marker alternatives are considered together.

Custom index/find-links/no-index environment settings for pip and uv prevent
public requests, even without a selected artifact. `PIP_CONFIG_FILE` is checked
when set. A selected pip file is not a reconstruction of pip's complete global,
user, virtualenv, and command-line configuration hierarchy. Selected lockfiles
are snapshots; project configuration overrides and freshness are not inferred.
No Python tools or build backends run. Unsupported configuration stops the check.

References: [uv indexes](https://docs.astral.sh/uv/configuration/indexes/),
[Pipenv indexes](https://pipenv.pypa.io/en/stable/indexes.html),
[pip configuration](https://pip.pypa.io/en/stable/topics/configuration/).

### Yarn and pnpm

For npm ecosystem checks, `--source-file` accepts Yarn Classic `.yarnrc`, Yarn
Berry `.yarnrc.yml`, and pnpm `pnpm-workspace.yaml`/`config.yaml`. Registry and
scope mappings in the selected file determine whether public assessment applies.
Unknown/custom registry identities return `unknown`. Named pnpm registry routing
and URL-keyed registry declaration objects are unsupported and return `unknown`. Registry URL interpolation is not guessed.
This is a selected-file assessment, not a merge of all inherited package-manager
configuration, plugins, package overrides, aliases, or command-line options.

A selected `.npmrc` is parsed by npm itself as user configuration in its directory;
normal npm project/environment precedence still applies. Yarn Classic registry
lines support the documented simple quoted/unquoted form; other source overrides
need explicit context. A public npm scope is not evidence of privacy, and private
npmjs.org packages cannot be identified reliably from a scope name alone.

References: [Yarn settings](https://yarnpkg.com/configuration/yarnrc/),
[pnpm settings](https://pnpm.io/settings), [npm config](https://docs.npmjs.com/cli/v11/using-npm/config/).

### Effective Maven and Gradle sources

Use `--project-dir /absolute/path/to/project maven group:artifact 1.2.3` (MCP:
`project_dir`, ecosystem `maven`). The installed Maven/Gradle tool processes its
own effective configuration. A directory containing both POM and Gradle build
files is rejected instead of guessing which manager applies.

- **Maven:** requires Maven and a compatible JDK (`javac`/`jar`). A temporary
  extension uses Maven's public `afterProjectsRead` lifecycle callback after model
  inheritance, settings, and active profiles have been applied. Resolver's public
  mirror selector supplies the effective endpoint. The inspector aborts before
  build goals run. System-path and reactor-local identities remain `unknown`.
- **Gradle:** requires installed Gradle. An init script uses public repository and
  resolution APIs and runs a generated inspection task in the selected project's
  task context. Metadata is resolved offline to detect local/project or module
  substitutions. Requested/default project tasks are not scheduled by DepTrust.
  Missing cached metadata, unresolved identities, and composite builds remain
  `unknown`.

These are **trusted-project integrations**, not static parsers or sandboxes.
Maven model extensions and Gradle configuration/buildSrc/plugin code may execute;
build tools may write local caches. Both are invoked offline, using installed
executables rather than wrappers that might download a distribution. Prepare
required parent POMs, extensions, plugins, and metadata in advance. Select the
actual project/subproject directory whose declared direct dependency is being
checked. Arbitrary transitive queries and undeclared modules remain `unknown`.

Only exclusively Maven Central endpoints permit a public assessment. Custom,
mixed, Ivy/local, or uncertain source sets return `unknown`, including mixed
Gradle repositories where a content filter might exclude the private source for
this particular module. The implementation does not use internal filter APIs or
pretend it can prove that narrower routing. The context is the installed tool and
current environment (including MAVEN_ARGS/Gradle properties); flags supplied only
to a separate future invocation are not inferred. Hooks stop on explicit profile,
property, or settings overrides rather than losing that context.

A standalone Maven effective XML supplied via `source_file` still returns
`unknown`: one artifact alone cannot establish the full effective source set.
No private registry credentials are fetched or changed.

Runtime tests use isolated projects and local repository tripwires, including
settings profiles, parent inheritance, mirrors, system/reactor dependencies,
Gradle exclusive content, project substitution, and missing offline metadata.
Run with `DEPTRUST_TEST_JVM=1 go test ./internal/app -run 'TestEffectiveMaven|TestEffectiveGradle|TestMavenLocal'`.
The public Gradle fixture primes an isolated cache with the vetted public
commons-lang3 test module before testing the product's offline inspection.

References: [Maven lifecycle callback](https://maven.apache.org/ref/3.9.11/maven-core/apidocs/org/apache/maven/AbstractMavenLifecycleParticipant.html),
[Maven repository order](https://maven.apache.org/guides/mini/guide-multiple-repositories),
[Gradle public repository API](https://docs.gradle.org/current/javadoc/org/gradle/api/artifacts/repositories/MavenArtifactRepository.html),
[Gradle resolution results](https://docs.gradle.org/current/javadoc/org/gradle/api/artifacts/result/ResolutionResult.html),
[Gradle safe resolution lifecycle](https://docs.gradle.org/current/userguide/declaring_configurations.html).

### Cabal resolved plans

For Hackage, `source_file` accepts Cabal's `dist-newstyle/cache/plan.json`.
Only matching `repo-tar` packages whose recorded repository URI is HTTPS
hackage.haskell.org proceed to public checks. Local, source-repository, tarball,
custom-repository, missing-source, and unlisted packages return `unknown`.
The parser follows Cabal's official plan writer; fixtures cover those shapes.
The selected plan is not freshness/provenance proof; DepTrust does not generate
or update it.

References: [Cabal external plan artifact](https://cabal.readthedocs.io/en/stable/nix-local-build.html),
[official plan writer](https://github.com/haskell/cabal/blob/master/cabal-install/src/Distribution/Client/ProjectPlanOutput.hs).

### CocoaPods locked sources

For CocoaPods, `source_file` accepts `Podfile.lock`. `SPEC REPOS` associates root
pod names with trunk/the public Specs repository or custom repositories.
`EXTERNAL SOURCES` takes precedence and returns `unknown`; mixed or missing
source assignments do too. Query root pod names, not subspec names. The selected
lockfile does not verify Podfile freshness, local repository configuration, or
installed artifact provenance. No Podfile or Ruby project code is evaluated.

Reference: [official CocoaPods lockfile implementation](https://github.com/CocoaPods/Core/blob/master/lib/cocoapods-core/lockfile.rb).

### Hex repository metadata

For Hex, `source_file` accepts a selected official package API response JSON.
The package name, `repository: hexpm`, and hex.pm package URL must agree before
public assessment proceeds. Organization/custom repository metadata, mismatched
identities, and missing fields return `unknown`. Repository-qualified names also
skip public requests without a selected file. Metadata is a caller-selected
snapshot, not live authentication or provenance verification. DepTrust does not
fetch private metadata on your behalf or evaluate Mix files. `mix.lock` parsing is supported as described below; effective Mix runtime repository
overrides and authenticated private fetching remain separate.

References: [Hex private organizations](https://hex.pm/docs/private),
[Hex repository usage](https://hex.pm/docs/usage),
[official repository-qualified naming](https://github.com/hexpm/hex/blob/main/lib/hex/utils.ex).

### GitHub Actions visibility

For GitHub Actions, local paths and `docker://` actions return `unknown` before
public requests. For repository actions, `source_file` accepts the official
GitHub repository API response JSON. Matching `full_name`, `private: false`,
`visibility: public`, and github.com repository URL are required before public
checks. Private/internal, mismatched, or incomplete metadata returns `unknown`.
This snapshot is not live visibility verification and may become stale. Without
it, ordinary owner/repo checks retain public behavior; privacy cannot be inferred
from an owner/name. Fetching authenticated metadata and enterprise-host support
remain separate integrations. No repository visibility or permission is changed.

Reference: [GitHub repository REST metadata](https://docs.github.com/en/rest/repos/repos).

Hooks also accept optional `source_context` containing the same `project_dir`,
`nuget_config`, `bundler_lock`, or `source_file` selector used by CLI/MCP. The
wrapper supplying hook JSON must provide it explicitly; identity/version fields
cannot override parsed install requests. With an absolute `cwd`, hooks use npm or
Cargo project context; NuGet uses its effective .NET hierarchy, and Go uses
effective GOPRIVATE independently. Other project
installs require explicit context rather than guessing inherited repositories
from a nearby artifact. New dependencies not yet represented by selected
artifacts may return `unknown`. This does not turn command parsing into a complete shell
interpreter; unrecognized commands and source overrides remain documented limits.

A consolidated [source support matrix](docs/package-sources.md) lists the
implemented paths, verification boundaries, and deferred integrations.

### Effective NuGet project configuration

Use `--project-dir /absolute/path/to/solution-directory nuget Package.Name 1.2.3`
(MCP: `project_dir`) to apply inherited NuGet configuration. DepTrust invokes the
official `dotnet nuget config paths` command, reads its ordered machine/user/parent
file list, and applies collection clears/removals, source-key overrides, disabled
sources, mapping precedence, and `%ENVIRONMENT_VARIABLE%` source URLs. The selected
directory is the restore context; choose the solution or project directory used
by your actual restore. The installed .NET SDK must support that command.

No restore or project evaluation runs during source discovery. Tests compare
merge decisions against the NuGet.Configuration assemblies shipped with the SDK,
including replacement of a source's pattern list by a child config. Discovery
errors stop public requests. `--nuget-config` retains NuGet's standalone-file mode.
Hooks can use their absolute cwd for effective NuGet context.

Reference: [official NuGet configuration hierarchy](https://learn.microsoft.com/en-us/nuget/consume-packages/configuring-nuget-behavior/).

### Mix lockfile sources

Hex `source_file` also accepts `mix.lock`. DepTrust invokes Elixir's official
`Code.string_to_quoted!` parser and inspects the returned AST without evaluating
the lockfile or loading mix.exs. Public `:hex` tuples must identify the queried
package and the `hexpm` repository (including Hex's legacy default repository).
Organization/custom repositories, Git entries, aliases queried as package names,
unlisted packages, and executable expressions return `unknown`. Syntax/tool
errors stop before public requests. Elixir must be on PATH.

Tests run the same parser in a cached official Elixir container with networking
disabled, read-only fixtures, and an executable-lockfile marker that must never
be created. Set `DEPTRUST_TEST_ELIXIR_IMAGE` to a cached digest to run that optional
oracle when Elixir is not installed; normal product code does not invoke Docker.

References: [Elixir AST parsing](https://elixir.hexdocs.pm/Code.html#string_to_quoted/2),
[Hex lock tuple semantics](https://github.com/hexpm/hex/blob/main/lib/hex/utils.ex).
