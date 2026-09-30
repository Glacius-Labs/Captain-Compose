#!/usr/bin/env bash
set -euo pipefail
version=${1:?Usage: scripts/release.sh VERSION}
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]] || { echo 'Invalid version' >&2; exit 1; }
requested_output_dir=${2:-dist/$version}
if [[ -n "$(git status --porcelain --untracked-files=all)" ]]; then
  echo 'Refusing to package a dirty source tree; commit or remove source changes first.' >&2
  git status --short >&2
  exit 1
fi
output_parent=$(dirname -- "$requested_output_dir")
output_name=$(basename -- "$requested_output_dir")
mkdir -p -- "$output_parent"
output_parent=$(cd -- "$output_parent" && pwd)
output_dir="$output_parent/$output_name"
if [[ -e "$output_dir" || -L "$output_dir" ]]; then
  echo "Output directory already exists: $output_dir" >&2
  exit 1
fi
stage_output=$(mktemp -d "$output_parent/.captain-compose-${version}.XXXXXX")
stage=''
cleanup() {
  [[ -z "$stage" ]] || rm -rf -- "$stage"
  rm -rf -- "$stage_output"
}
trap cleanup EXIT
commit=$(git rev-parse HEAD)
date=$(git show -s --format=%cI HEAD)
for os in linux darwin windows; do
  for arch in amd64 arm64; do
    stage=$(mktemp -d)
    binary=captain-compose-mqtt
    [[ "$os" != windows ]] || binary+=.exe
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=$version -X main.commit=$commit -X main.buildDate=$date" -o "$stage/$binary" ./cmd/captain-compose-mqtt
    cli_binary=captain-compose
    [[ "$os" != windows ]] || cli_binary+=.exe
    if [[ -d cmd/captain-compose ]]; then
      CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=$version -X main.commit=$commit -X main.buildDate=$date" -o "$stage/$cli_binary" ./cmd/captain-compose
    else
      echo 'Missing operator CLI source at cmd/captain-compose' >&2
      exit 1
    fi
    cp LICENSE README.md SECURITY.md CONTRIBUTING.md CHANGELOG.md captain-compose-banner.png "$stage/"
    mkdir -p "$stage/scripts"
    cp scripts/install.sh scripts/provision.sh scripts/provision-lib.sh scripts/backup.sh scripts/restore.sh scripts/upgrade.sh scripts/archive-safety.py "$stage/scripts/"
    cp -R docs config schemas deploy "$stage/"
    archive="captain-compose_${version}_${os}_${arch}"
    if [[ "$os" == windows ]]; then
      (cd "$stage" && zip -qr "$stage_output/$archive.zip" .)
    else
      tar -czf "$stage_output/$archive.tar.gz" -C "$stage" .
    fi
    # Only delete this invocation's mktemp directory.
    rm -rf -- "$stage"
    stage=''
  done
done
(cd "$stage_output" && sha256sum captain-compose_"$version"_* > checksums.txt)
if [[ -e "$output_dir" || -L "$output_dir" ]]; then
  echo "Output directory appeared while packaging: $output_dir" >&2
  exit 1
fi
mv -- "$stage_output" "$output_dir"
trap - EXIT
