#!/usr/bin/env bash
set -euo pipefail

version="${1:?version is required, for example v0.1.0}"
commit="${2:-$(git rev-parse --short HEAD 2>/dev/null || printf 'unknown')}"
date="${3:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"
out_dir="${4:-dist}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
package="github.com/clidey/deptrust/internal/buildinfo"
ldflags="-s -w -X $package.Version=$version -X $package.Commit=$commit -X $package.Date=$date"

if [[ "$out_dir" = /* ]]; then
  out_path="$out_dir"
else
  out_path="$repo_root/$out_dir"
fi

targets=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
)

rm -rf "$out_path"
mkdir -p "$out_path/checksums"

for target in "${targets[@]}"; do
  goos="${target%/*}"
  goarch="${target#*/}"
  name="deptrust_${version}_${goos}_${goarch}"
  build_dir="$out_path/$name"
  binary="deptrust"
  archive="$name.tar.gz"

  if [[ "$goos" == "windows" ]]; then
    binary="deptrust.exe"
    archive="$name.zip"
  fi

  mkdir -p "$build_dir"
  (
    cd "$repo_root"
    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build \
      -trimpath \
      -ldflags "$ldflags" \
      -o "$build_dir/$binary" \
      ./cmd/deptrust
  )

  cp "$repo_root/README.md" "$build_dir/README.md"
  cp "$repo_root/LICENSE" "$build_dir/LICENSE"

  (
    cd "$out_path"
    if [[ "$goos" == "windows" ]]; then
      zip -qr "$archive" "$name"
    else
      tar -czf "$archive" "$name"
    fi
    rm -rf "$name"
  )
done

(
  cd "$out_path"
  shasum -a 256 deptrust_* > checksums.txt
)
