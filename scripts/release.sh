#!/usr/bin/env bash
set -euo pipefail
version=${1:?Usage: scripts/release.sh VERSION}
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]] || { echo 'Invalid version' >&2; exit 1; }
mkdir -p dist
commit=$(git rev-parse HEAD)
date=$(git show -s --format=%cI HEAD)
for os in linux darwin windows; do
  for arch in amd64 arm64; do
    stage=$(mktemp -d)
    binary=captain-compose-mqtt
    [[ "$os" != windows ]] || binary+=.exe
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=$version -X main.commit=$commit -X main.buildDate=$date" -o "$stage/$binary" ./cmd/captain-compose-mqtt
    cp LICENSE README.md SECURITY.md CONTRIBUTING.md "$stage/"
    mkdir -p "$stage/scripts"
    cp scripts/install.sh scripts/provision.sh "$stage/scripts/"
    cp -R docs config "$stage/"
    archive="captain-compose_${version}_${os}_${arch}"
    if [[ "$os" == windows ]]; then
      (cd "$stage" && zip -qr "$OLDPWD/dist/$archive.zip" .)
    else
      tar -czf "dist/$archive.tar.gz" -C "$stage" .
    fi
    # Only delete this invocation's mktemp directory.
    rm -rf -- "$stage"
  done
done
(cd dist && sha256sum captain-compose_"$version"_* > checksums.txt)
