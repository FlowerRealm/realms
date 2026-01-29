#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

version="${1:-}"
out_dir="${2:-dist}"

if [ -z "$version" ]; then
  echo "Usage: $0 <version> [out_dir]"
  exit 2
fi

build_date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
ldflags="-s -w -X realms/internal/version.Version=$version -X realms/internal/version.Date=$build_date"

mkdir -p "$out_dir"
rm -f "$out_dir/SHA256SUMS"

build_linux() {
  local arch="$1"
  local goarch="$2"
  local dir="$out_dir/realms_${version}_linux_${arch}"
  mkdir -p "$dir"
  CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go build -ldflags "$ldflags" -o "$dir/realms" ./cmd/realms
  cp ".env.example" "$dir/.env.example"
  cp "README.md" "$dir/README.md"
  tar -C "$out_dir" -czf "$out_dir/realms_${version}_linux_${arch}.tar.gz" "realms_${version}_linux_${arch}"
  rm -r "$dir"
}

build_darwin() {
  local arch="$1"
  local goarch="$2"
  local dir="$out_dir/realms_${version}_darwin_${arch}"
  mkdir -p "$dir"
  CGO_ENABLED=0 GOOS=darwin GOARCH="$goarch" go build -ldflags "$ldflags" -o "$dir/realms" ./cmd/realms
  cp ".env.example" "$dir/.env.example"
  cp "README.md" "$dir/README.md"
  tar -C "$out_dir" -czf "$out_dir/realms_${version}_darwin_${arch}.tar.gz" "realms_${version}_darwin_${arch}"
  rm -r "$dir"
}

build_windows_amd64() {
  local dir="$out_dir/realms_${version}_windows_amd64"
  mkdir -p "$dir"
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$ldflags" -o "$dir/realms.exe" ./cmd/realms
  cp ".env.example" "$dir/.env.example"
  cp "README.md" "$dir/README.md"
  (cd "$out_dir" && zip -q -r "realms_${version}_windows_amd64.zip" "realms_${version}_windows_amd64")
  rm -r "$dir"
}

build_linux amd64 amd64
build_linux arm64 arm64
build_darwin amd64 amd64
build_darwin arm64 arm64
build_windows_amd64

bash "./scripts/build-deb.sh" "$version" amd64 "$out_dir"
bash "./scripts/build-deb.sh" "$version" arm64 "$out_dir"

echo "[OK] release artifacts -> $out_dir/"
