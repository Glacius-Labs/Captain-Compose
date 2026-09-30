#!/usr/bin/env bash
# Install a specific release. No daemon or host configuration is changed here.
set -euo pipefail
version=''
prefix="$HOME/.local/bin"
archive_dir=''
usage() { echo 'Usage: install.sh --version X.Y.Z [--prefix DIR] [--archive-dir DIR]'; }
while (($#)); do
  case "$1" in
    --version) version=${2:?Missing version}; shift 2 ;;
    --prefix) prefix=${2:?Missing prefix}; shift 2 ;;
    --archive-dir) archive_dir=${2:?Missing archive directory}; shift 2 ;;
    --help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]] || { echo 'An explicit semantic version is required' >&2; exit 2; }
case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; *) echo 'Use the Windows release ZIP on Windows' >&2; exit 2 ;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo 'Unsupported architecture' >&2; exit 2 ;; esac
archive="captain-compose_${version}_${os}_${arch}.tar.gz"
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
if [[ -n "$archive_dir" ]]; then
  cp "$archive_dir/$archive" "$work/$archive"
  cp "$archive_dir/checksums.txt" "$work/checksums.txt"
else
  base="https://github.com/Glacius-Labs/Captain-Compose/releases/download/v$version"
  curl --fail --show-error --silent --location --proto '=https' --tlsv1.2 "$base/$archive" -o "$work/$archive"
  curl --fail --show-error --silent --location --proto '=https' --tlsv1.2 "$base/checksums.txt" -o "$work/checksums.txt"
fi
expected=$(awk -v name="$archive" '$2 == name { print $1 }' "$work/checksums.txt")
[[ "$expected" =~ ^[a-f0-9]{64}$ ]] || { echo 'Missing or ambiguous release checksum' >&2; exit 1; }
if command -v sha256sum >/dev/null; then actual=$(sha256sum "$work/$archive"); else actual=$(shasum -a 256 "$work/$archive"); fi
[[ "${actual%% *}" == "$expected" ]] || { echo 'Release checksum mismatch' >&2; exit 1; }
# Extract only the expected executable; no archive paths are installed.
tar -xOf "$work/$archive" ./captain-compose-mqtt > "$work/captain-compose-mqtt"
chmod 0755 "$work/captain-compose-mqtt"
"$work/captain-compose-mqtt" --version
mkdir -p "$prefix"
install -m 0755 "$work/captain-compose-mqtt" "$prefix/.captain-compose-mqtt.new"
mv -f -- "$prefix/.captain-compose-mqtt.new" "$prefix/captain-compose-mqtt"
echo "Installed $prefix/captain-compose-mqtt; ensure $prefix is on PATH."
