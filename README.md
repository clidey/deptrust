# DepTrust

DepTrust is a local MCP server and CLI that lets an AI agent ask whether a
package version has known vulnerabilities before installing or updating it.

It is intentionally not a hosted service. The executable runs locally, calls
public registry and vulnerability APIs directly, and returns an agent-friendly
recommendation.

## Scope

Supported ecosystems:

- npm
- PyPI
- Cargo / crates.io

Current data sources:

- OSV for known vulnerabilities
- npm, PyPI, and crates.io metadata APIs for version validation and `latest`

Current non-goals:

- no hosted backend
- no database or persistent cache
- no package tarball downloads
- no lockfile scanning
- no malware or obfuscation heuristics yet

## Build

```bash
go build -o deptrust ./cmd/deptrust
```

If your Go build cache is outside the writable environment, use a local cache:

```bash
mkdir -p .cache/go-build
GOCACHE="$PWD/.cache/go-build" go build -o deptrust ./cmd/deptrust
```

## CLI

Check an exact version:

```bash
./deptrust check npm lodash 4.17.20
```

Check the latest version:

```bash
./deptrust check pypi requests latest
```

Emit stable JSON:

```bash
./deptrust check --json cargo serde latest
```

Suggest the latest version only when no known vulnerabilities are found:

```bash
./deptrust suggest --json npm lodash
```

## MCP

Run the MCP stdio server:

```bash
./deptrust mcp
```

Use this command in any MCP client that supports stdio servers:

```json
{
  "command": "/absolute/path/to/deptrust",
  "args": ["mcp"]
}
```

For clients that use the common `mcpServers` object:

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

## MCP Tools

### `check_package`

Input:

```json
{
  "ecosystem": "npm",
  "package": "lodash",
  "version": "4.17.20"
}
```

`version` may be omitted or set to `latest`. If an exact version does not
exist, DepTrust returns an error and includes the latest version in the message.

### `suggest_safe_version`

Input:

```json
{
  "ecosystem": "npm",
  "package": "lodash"
}
```

This checks the latest version and suggests it only if no known vulnerabilities
are found.

## Recommendation Policy

DepTrust v1 only evaluates known vulnerabilities.

| Highest known severity | Risk score | Recommendation |
| --- | ---: | --- |
| critical | 95 | block |
| high | 80 | block |
| medium / unknown | 50 / 40 | review |
| low | 20 | allow |
| none found | 0 | allow |

`allow` means "no blocking known vulnerability was found by the configured
providers." It does not prove the package is safe.
