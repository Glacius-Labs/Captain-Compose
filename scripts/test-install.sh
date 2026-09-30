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

source scripts/provision-lib.sh
config_dir="$work/config"
mkdir -p "$config_dir"
printf 'secret\n' > "$config_dir/config.yaml"
chmod 0644 "$config_dir/config.yaml"
secure_config_destination "$config_dir/config.yaml" "$(id -gn)" "$(id -un)"
test "$(stat -c '%a' "$config_dir/config.yaml")" = 640
test "$(cat "$config_dir/config.yaml")" = secret
ln -s "$config_dir/config.yaml" "$config_dir/config-link.yaml"
if secure_config_destination "$config_dir/config-link.yaml" "$(id -gn)" "$(id -un)"; then
  echo 'Symlink configuration destination was accepted' >&2
  exit 1
fi
test "$(stat -c '%a' "$config_dir/config.yaml")" = 640

dirty_marker=".release-dirty-check-$$"
trap 'rm -f -- "$dirty_marker"; rm -rf -- "$work"' EXIT
printf 'dirty marker\n' > "$dirty_marker"
if bash scripts/release.sh 0.0.0-test "$work/should-not-be-created" > "$work/release-dirty.log" 2>&1; then
  echo 'Dirty release source tree was accepted' >&2
  exit 1
fi
grep -q 'Refusing to package a dirty source tree' "$work/release-dirty.log"
test ! -e "$work/should-not-be-created"
rm -f -- "$dirty_marker"
