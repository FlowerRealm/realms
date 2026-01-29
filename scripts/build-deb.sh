#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

version="${1:-}"
arch="${2:-}"
out_dir="${3:-dist}"

if [ -z "$version" ] || [ -z "$arch" ]; then
  echo "Usage: $0 <version> <arch: amd64|arm64> [out_dir]"
  exit 2
fi

goarch=""
case "$arch" in
  amd64) goarch="amd64" ;;
  arm64) goarch="arm64" ;;
  *)
    echo "[ERROR] unsupported arch: $arch (expected amd64 or arm64)"
    exit 2
    ;;
esac

build_date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
ldflags="-s -w -X realms/internal/version.Version=$version -X realms/internal/version.Date=$build_date"

deb_version="${version#v}"
pkg_name="realms_${version}_linux_${arch}.deb"

tmp_dir="$(mktemp -d)"
trap 'rm -r "$tmp_dir"' EXIT

pkg_root="$tmp_dir/pkg"
mkdir -p "$pkg_root/DEBIAN"
mkdir -p "$pkg_root/usr/bin"
mkdir -p "$pkg_root/etc/realms"
mkdir -p "$pkg_root/lib/systemd/system"
mkdir -p "$pkg_root/var/lib/realms/data"

CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go build -ldflags "$ldflags" -o "$pkg_root/usr/bin/realms" ./cmd/realms

install -m 0644 "packaging/debian/realms.env" "$pkg_root/etc/realms/realms.env"
install -m 0644 "packaging/debian/realms.service" "$pkg_root/lib/systemd/system/realms.service"
install -m 0755 "packaging/debian/postinst" "$pkg_root/DEBIAN/postinst"
install -m 0755 "packaging/debian/prerm" "$pkg_root/DEBIAN/prerm"

cat > "$pkg_root/DEBIAN/control" <<EOF
Package: realms
Version: $deb_version
Section: net
Priority: optional
Architecture: $arch
Maintainer: Realms Maintainers
Depends: ca-certificates, adduser
Description: Realms (OpenAI-compatible API relay)
 Realms is a Go monolithic service providing an OpenAI-compatible API and a web console.
EOF

echo "/etc/realms/realms.env" > "$pkg_root/DEBIAN/conffiles"

mkdir -p "$out_dir"

if command -v fakeroot >/dev/null 2>&1; then
  fakeroot dpkg-deb --build "$pkg_root" "$out_dir/$pkg_name"
else
  dpkg-deb --build "$pkg_root" "$out_dir/$pkg_name"
fi

echo "[OK] $out_dir/$pkg_name"
