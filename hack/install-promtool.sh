#!/usr/bin/env bash
# Download a pinned Prometheus release, verify its SHA-256 against the value
# pinned below, and install promtool into the given directory.
set -euo pipefail

version="${1:?usage: install-promtool.sh VERSION DEST}"
dest="${2:?usage: install-promtool.sh VERSION DEST}"

# Pinned checksums (from the release's sha256sums.txt). Add a line when bumping.
declare -A sums=(
  ["3.5.0-linux-amd64"]="e811827af26d822afb09a4f28314f61b618b12cff5369835a67f674d8b46f39a"
)

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
esac
key="${version}-${os}-${arch}"
want="${sums[$key]:-}"
if [[ -z "$want" ]]; then
  echo "no pinned checksum for prometheus ${key}; add it to $0" >&2
  exit 1
fi

name="prometheus-${version}.${os}-${arch}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -sSfL -o "$tmp/$name.tar.gz" \
  "https://github.com/prometheus/prometheus/releases/download/v${version}/${name}.tar.gz"
echo "${want}  $tmp/$name.tar.gz" | sha256sum -c -
tar -xzf "$tmp/$name.tar.gz" -C "$tmp" "$name/promtool"
mkdir -p "$dest"
install -m 0755 "$tmp/$name/promtool" "$dest/promtool"
"$dest/promtool" --version | head -1
