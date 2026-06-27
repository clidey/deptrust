#!/usr/bin/env bash
set -euo pipefail

# Generates a Homebrew formula from packaging/homebrew/deptrust.rb.tmpl by
# filling in the version and the per-archive sha256 sums from a checksums.txt
# (the one produced by scripts/build-release.sh and attached to each release).
#
# Usage:
#   scripts/generate-homebrew-formula.sh <tag> <checksums-file> [out-file]
#
# Example:
#   scripts/generate-homebrew-formula.sh v0.6.0 dist/checksums.txt dist/deptrust.rb

tag="${1:?tag is required, for example v0.6.0}"
checksums="${2:?path to checksums.txt is required}"
out="${3:-dist/deptrust.rb}"

version="${tag#v}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
template="$repo_root/packaging/homebrew/deptrust.rb.tmpl"

if [[ ! -f "$template" ]]; then
  echo "generate-homebrew-formula.sh: template not found at $template" >&2
  exit 1
fi
if [[ ! -f "$checksums" ]]; then
  echo "generate-homebrew-formula.sh: checksums file not found at $checksums" >&2
  exit 1
fi

# Look up the sha256 for an archive name from the checksums file.
# checksums.txt lines look like: "<sha256>  deptrust_v0.6.0_darwin_arm64.tar.gz"
sha_for() {
  local archive="$1"
  local sum
  sum="$(awk -v a="$archive" '$2 == a {print $1}' "$checksums")"
  if [[ -z "$sum" ]]; then
    echo "generate-homebrew-formula.sh: no sha256 for $archive in $checksums" >&2
    exit 1
  fi
  printf '%s' "$sum"
}

sha_darwin_arm64="$(sha_for "deptrust_${tag}_darwin_arm64.tar.gz")"
sha_darwin_amd64="$(sha_for "deptrust_${tag}_darwin_amd64.tar.gz")"
sha_linux_arm64="$(sha_for "deptrust_${tag}_linux_arm64.tar.gz")"
sha_linux_amd64="$(sha_for "deptrust_${tag}_linux_amd64.tar.gz")"

mkdir -p "$(dirname "$out")"

sed \
  -e "s|__VERSION__|${version}|g" \
  -e "s|__SHA_DARWIN_ARM64__|${sha_darwin_arm64}|g" \
  -e "s|__SHA_DARWIN_AMD64__|${sha_darwin_amd64}|g" \
  -e "s|__SHA_LINUX_ARM64__|${sha_linux_arm64}|g" \
  -e "s|__SHA_LINUX_AMD64__|${sha_linux_amd64}|g" \
  "$template" > "$out"

echo "Wrote $out for $tag"
