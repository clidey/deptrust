#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source_dir="$repo_root/.agents/skills/deptrust-package-check"
target_root="${1:-${CODEX_SKILLS_DIR:-$HOME/.agents/skills}}"
target_dir="$target_root/deptrust-package-check"

if [[ ! -f "$source_dir/SKILL.md" ]]; then
  echo "deptrust skill not found at $source_dir" >&2
  exit 1
fi

mkdir -p "$target_root"
if [[ -e "$target_dir" ]] && ! cmp -s "$source_dir/SKILL.md" "$target_dir/SKILL.md"; then
  backup="$target_dir.bak-$(date +%s)"
  mv "$target_dir" "$backup"
  echo "Found a customized skill at $target_dir, so we saved it to $backup before installing ours." >&2
else
  rm -rf "$target_dir"
fi
cp -R "$source_dir" "$target_dir"

echo "Installed the deptrust-package-check skill to $target_dir."
echo "If it doesn't show up right away, give Codex a restart."

