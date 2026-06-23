# deptrust

deptrust is a CLI that checks package versions for known vulnerabilities across npm, PyPI, crates.io, and more.

It is a standalone executable that can be run locally.

There is also an MCP server that can be used by tools like Claude Code and Codex to run deptrust and check dependencies before suggesting/installing them.

This tool was born out of the frustration that is AI agents constantly using old versions.

## Scope

Supported ecosystems:

- npm
- PyPI
- Cargo / crates.io

Current data sources:

- OSV for known vulnerabilities
- npm, PyPI, and crates.io metadata APIs for version validation and `latest`

## Build

```bash
go build -o deptrust ./cmd/deptrust
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

Return as JSON:

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

DepTrust evaluates known vulnerabilities.

| Highest known severity | Risk score | Recommendation |
| --- | ---: | --- |
| critical | 95 | block |
| high | 80 | block |
| medium / unknown | 50 / 40 | review |
| low | 20 | allow |
| none found | 0 | allow |

`allow` means "no blocking known vulnerability was found and publicly disclosed." It does not prove the package is safe. Exercise caution as usual.
