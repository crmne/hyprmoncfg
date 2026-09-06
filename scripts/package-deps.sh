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
GOMODCACHE="$cache_dir/go-mod" go mod download all
GOMODCACHE="$cache_dir/go-mod" GOPROXY=off go mod verify
mkdir -p "$destination"
tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner \
  --exclude='*.lock' -C "$cache_dir" -c go-mod | xz -T2 > "$archive"
