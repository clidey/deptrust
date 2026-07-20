#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bin_dir="${DEPTRUST_BIN_DIR:-$HOME/.local/bin}"
install_codex_mcp=false
install_codex_skill=false
install_claude_code_mcp=false
run_check=false

usage() {
  cat <<'USAGE'
Usage:
  ./scripts/install.sh [options]

Options:
  --bin-dir DIR          Install deptrust into DIR. Default: ~/.local/bin
  --codex-mcp           Register deptrust as a Codex MCP server
  --codex-skill         Install the Codex CLI skill fallback
  --claude-code-mcp     Register deptrust as a Claude Code MCP server
  --all                 Install binary plus Codex MCP, Codex skill, and Claude Code MCP
  --check               Verify the binary was installed at the expected path
  -h, --help            Show this help

Examples:
  ./scripts/install.sh
  ./scripts/install.sh --check
  ./scripts/install.sh --codex-mcp --codex-skill
  ./scripts/install.sh --all
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bin-dir)
      if [[ $# -lt 2 ]]; then
        echo "--bin-dir requires a directory" >&2
        exit 2
      fi
      bin_dir="$2"
      shift 2
      ;;
    --codex-mcp)
      install_codex_mcp=true
      shift
      ;;
    --codex-skill)
      install_codex_skill=true
      shift
      ;;
    --claude-code-mcp)
      install_claude_code_mcp=true
      shift
      ;;
    --all)
      install_codex_mcp=true
      install_codex_skill=true
      install_claude_code_mcp=true
      shift
      ;;
    --check)
      run_check=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

mkdir -p "$bin_dir"

tmp_bin="$(mktemp "${TMPDIR:-/tmp}/deptrust.XXXXXX")"
cleanup() {
  rm -f "$tmp_bin"
}
trap cleanup EXIT

echo "Building deptrust from source..."
version="$(cd "$repo_root" && git describe --tags --always --dirty 2>/dev/null || printf 'dev')"
commit="$(cd "$repo_root" && git rev-parse --short HEAD 2>/dev/null || printf 'unknown')"
date="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
ldflags="-s -w -X github.com/clidey/deptrust/internal/buildinfo.Version=$version -X github.com/clidey/deptrust/internal/buildinfo.Commit=$commit -X github.com/clidey/deptrust/internal/buildinfo.Date=$date"
(cd "$repo_root" && go build -ldflags "$ldflags" -o "$tmp_bin" ./cmd/deptrust)

if [[ "$(uname -s)" == "Darwin" ]]; then
  codesign --force --sign - "$tmp_bin"
  codesign --verify --strict "$tmp_bin"
fi

install_path="$bin_dir/deptrust"
install -m 0755 "$tmp_bin" "$install_path"
echo "Installed deptrust to $install_path."

if $install_codex_skill; then
  "$repo_root/scripts/install-codex-skill.sh"
fi

setup_args=()
if $install_codex_mcp; then
  setup_args+=(--codex-mcp)
fi
if $install_claude_code_mcp; then
  setup_args+=(--claude-code-mcp)
fi
if [[ ${#setup_args[@]} -gt 0 ]]; then
  "$install_path" setup "${setup_args[@]}"
fi

if $run_check; then
  if [[ ! -x "$install_path" ]]; then
    echo "Something's off: $install_path is missing or isn't executable." >&2
    exit 1
  fi
  echo "Looks good: $install_path is in place and executable."
fi

cat <<EOF

You're all set. A couple of things to try:
  $install_path check npm lodash latest
  $install_path mcp

If you'd rather wire up MCP by hand, point your client at:
  command = "$install_path"
  args = ["mcp"]
EOF

case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *)
    cat <<EOF

One heads-up: $bin_dir isn't on your PATH yet.
To run deptrust by name, add this to your shell config:
  export PATH="$bin_dir:\$PATH"
EOF
    ;;
esac
