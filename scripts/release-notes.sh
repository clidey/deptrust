#!/usr/bin/env bash
set -euo pipefail

tag="${1:?tag is required, for example v0.1.0}"
out="${2:-dist/release-notes.md}"

mkdir -p "$(dirname "$out")"

if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  target_ref="$tag"
else
  target_ref="HEAD"
fi

previous_tag="$(git describe --tags --abbrev=0 "${target_ref}^" 2>/dev/null || true)"
if [[ -n "$previous_tag" ]]; then
  range="$previous_tag..$target_ref"
  heading="Changes since $previous_tag"
else
  range="$target_ref"
  heading="Changes"
fi

{
  printf '# deptrust %s\n\n' "$tag"
  printf '## %s\n\n' "$heading"
  git log --no-merges --pretty=format:'- %s (%h)' "$range"
  printf '\n'
} > "$out"
