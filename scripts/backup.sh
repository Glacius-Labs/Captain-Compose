#!/usr/bin/env bash
# Create a private, offline backup of agent state and credentials/configuration.
set -euo pipefail
output=${1:?Usage: backup.sh ABSOLUTE_OUTPUT.tar.gz}
root=${CAPTAIN_COMPOSE_ROOT:-/}
[[ "$EUID" == 0 ]] || { echo 'Run as root to capture complete state and credentials' >&2; exit 1; }
[[ "$output" == /* && "$output" != *$'\n'* ]] || { echo 'Backup destination must be an absolute path' >&2; exit 2; }
[[ "$root" == /* && -d "$root" && ! -L "$root" ]] || { echo 'Backup root must be an absolute, real directory' >&2; exit 2; }
root=$(cd -- "$root" && pwd -P)
root_prefix=${root%/}
for parent in "$root/var" "$root/var/lib" "$root/etc"; do
  [[ ! -L "$parent" && -d "$parent" ]] || { echo "Backup parent must be an existing real directory: $parent" >&2; exit 1; }
done
command -v systemctl >/dev/null
systemctl is-active --quiet captain-compose.service && { echo 'Stop captain-compose.service before backup' >&2; exit 1; }
for source in "$root/var/lib/captain-compose" "$root/etc/captain-compose"; do
  [[ -d "$source" && ! -L "$source" ]] || { echo "Missing or unsafe backup source: $source" >&2; exit 1; }
done
parent=$(dirname -- "$output")
name=$(basename -- "$output")
[[ -d "$parent" && ! -L "$parent" && ! -e "$output" && ! -L "$output" ]] || { echo 'Destination parent must exist and destination must not exist' >&2; exit 1; }
canonical_parent=$(cd -- "$parent" && pwd -P)
case "$canonical_parent/$name" in
  "$root_prefix"/var/lib/captain-compose/*|"$root_prefix"/etc/captain-compose/*) echo 'Backup must be stored outside the backed-up directories' >&2; exit 1 ;;
esac
tmp=$(mktemp "$canonical_parent/.captain-compose-backup.XXXXXX")
trap 'rm -f -- "$tmp"' EXIT
tar --numeric-owner -czf "$tmp" -C "$root" ./var/lib/captain-compose ./etc/captain-compose
python3 "$(dirname -- "${BASH_SOURCE[0]}")/archive-safety.py" validate "$tmp"
chmod 0600 "$tmp"
mv -- "$tmp" "$canonical_parent/$name"
trap - EXIT
echo "Offline backup written with mode 0600: $canonical_parent/$name"
