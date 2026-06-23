#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if ! command -v golangci-lint >/dev/null 2>&1; then
	echo "lint.sh: golangci-lint is not installed" >&2
	echo "lint.sh: install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2" >&2
	exit 1
fi

if [ "$#" -eq 0 ]; then
	exec golangci-lint run --config "${SCRIPT_DIR}/.golangci.yml" ./...
fi

exec golangci-lint "$@"
