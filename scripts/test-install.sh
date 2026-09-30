#!/usr/bin/env bash
set -euo pipefail
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
mkdir -p "$work/source" "$work/releases" "$work/bin"
printf '#!/bin/sh\necho captain-compose-mqtt test\n' > "$work/source/captain-compose-mqtt"
archive=captain-compose_0.0.0-test_linux_amd64.tar.gz
tar -czf "$work/releases/$archive" -C "$work/source" .
(cd "$work/releases" && sha256sum "$archive" > checksums.txt)
bash scripts/install.sh --version 0.0.0-test --prefix "$work/bin" --archive-dir "$work/releases"
test -x "$work/bin/captain-compose-mqtt"
before=$(sha256sum "$work/bin/captain-compose-mqtt")
printf corrupt >> "$work/releases/$archive"
if bash scripts/install.sh --version 0.0.0-test --prefix "$work/bin" --archive-dir "$work/releases"; then echo 'Checksum rejection failed' >&2; exit 1; fi
test "$before" = "$(sha256sum "$work/bin/captain-compose-mqtt")"
bash scripts/provision.sh --config config/mqtt/config.example.yaml --dry-run
