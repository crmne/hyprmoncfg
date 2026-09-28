#!/usr/bin/env bash
set -euo pipefail

version=${1:?Usage: package-deps.sh VERSION OUTPUT_DIRECTORY}
destination=${2:?Usage: package-deps.sh VERSION OUTPUT_DIRECTORY}
if [[ ! $version =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][a-zA-Z0-9.-]+)?$ ]]; then
  echo "Invalid release version: $version" >&2
  exit 1
fi

archive="$destination/hyprmoncfg-$version-deps.tar.xz"
if [[ -e $archive ]]; then
  echo "Refusing to replace $archive" >&2
  exit 1
fi

cache_dir=$(mktemp -d)
trap 'chmod -R u+w "$cache_dir"; rm -rf -- "$cache_dir"' EXIT
# Downloading the complete graph may add indirect checksums. Keep those changes
# out of the release checkout, which GoReleaser requires to remain clean.
cp go.mod go.sum "$cache_dir/"
(
  cd "$cache_dir"
  GOMODCACHE="$cache_dir/go-mod" go mod download all
  GOMODCACHE="$cache_dir/go-mod" GOPROXY=off go mod verify
)
mkdir -p "$destination"
# Go leaves its module cache read-only. Packed as is, every package build
# extracts folders its own clean-up cannot delete (yay and makepkg -c print a
# wall of "Permission denied"). Go verifies modules against go.sum, not file
# modes, so ship them owner-writable.
tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner --mode=u+w \
  --exclude='*.lock' -C "$cache_dir" -c go-mod | xz -T2 > "$archive"
