#!/usr/bin/env bash
# Stage and restore a complete backup without overwriting existing directories by default.
set -euo pipefail
archive=''
replace=false
confirm=''
root=${CAPTAIN_COMPOSE_ROOT:-/}
while (($#)); do
  case "$1" in
    --archive) archive=${2:?Missing archive path}; shift 2 ;;
    --replace-existing) replace=true; shift ;;
    --confirm-replace) confirm=${2:?Missing replacement confirmation}; shift 2 ;;
    --help) echo 'Usage: restore.sh --archive ABSOLUTE_BACKUP.tar.gz [--replace-existing --confirm-replace state-and-config]'; exit 0 ;;
    *) echo 'Unknown argument' >&2; exit 2 ;;
  esac
done
[[ "$EUID" == 0 ]] || { echo 'Run as root to restore protected state/configuration' >&2; exit 1; }
[[ "$root" == /* && -d "$root" && ! -L "$root" ]] || { echo 'Restore root must be an absolute, real directory' >&2; exit 2; }
root=$(cd -- "$root" && pwd -P)
root=${root%/}
for parent in "$root/var" "$root/var/tmp" "$root/var/lib" "$root/etc"; do
  [[ ! -L "$parent" && -d "$parent" ]] || { echo "Restore parent must be an existing real directory: $parent" >&2; exit 1; }
done
[[ -f "$archive" && ! -L "$archive" ]] || { echo 'Backup must be a regular non-symlink file' >&2; exit 2; }
if "$replace"; then
  [[ "$confirm" == state-and-config ]] || { echo 'Replacement requires --confirm-replace state-and-config' >&2; exit 2; }
elif [[ -n "$confirm" ]]; then
  echo '--confirm-replace requires --replace-existing' >&2
  exit 2
fi
systemctl is-active --quiet captain-compose.service && { echo 'Stop captain-compose.service before restore' >&2; exit 1; }
python3 "$(dirname -- "${BASH_SOURCE[0]}")/archive-safety.py" validate "$archive"
stage=$(mktemp -d "$root/var/tmp/captain-compose-restore.XXXXXX")
trap 'rm -rf -- "$stage"' EXIT
python3 "$(dirname -- "${BASH_SOURCE[0]}")/archive-safety.py" extract "$archive" "$stage/extracted"
state_target="$root/var/lib/captain-compose"
config_target="$root/etc/captain-compose"
state_stage="$stage/extracted/var/lib/captain-compose"
config_stage="$stage/extracted/etc/captain-compose"
for target in "$state_target" "$config_target"; do
  [[ ! -L "$target" ]] || { echo "Refusing symlink restore destination: $target" >&2; exit 1; }
  if [[ -e "$target" ]] && ! "$replace"; then
    echo "Restore destination already exists: $target (use --replace-existing --confirm-replace state-and-config only after review)" >&2
    exit 1
  fi
done
stamp=$(date -u +%Y%m%dT%H%M%SZ)-$$
state_saved="$root/var/lib/captain-compose.pre-restore-$stamp"
config_saved="$root/etc/captain-compose.pre-restore-$stamp"
[[ ! -e "$state_saved" && ! -L "$state_saved" && ! -e "$config_saved" && ! -L "$config_saved" ]] || { echo 'Refusing colliding previous-state backup paths' >&2; exit 1; }
state_moved=false
config_moved=false
state_installed=false
config_installed=false
rollback() {
  local status=$?
  if ((status != 0)); then
    if "$config_installed"; then mv -T -- "$config_target" "$stage/failed-config"; fi
    if "$state_installed"; then mv -T -- "$state_target" "$stage/failed-state"; fi
    if "$config_moved"; then mv -T -- "$config_saved" "$config_target"; fi
    if "$state_moved"; then mv -T -- "$state_saved" "$state_target"; fi
    rm -rf -- "$stage"
    return "$status"
  fi
}
trap rollback EXIT
if [[ -e "$state_target" ]]; then mv -T -- "$state_target" "$state_saved"; state_moved=true; fi
if [[ -e "$config_target" ]]; then mv -T -- "$config_target" "$config_saved"; config_moved=true; fi
mv -T -- "$state_stage" "$state_target"
state_installed=true
if [[ "${CAPTAIN_COMPOSE_TEST_FAIL_AFTER_STATE_INSTALL:-0}" == 1 ]]; then echo 'Injected test failure after state install' >&2; exit 99; fi
mv -T -- "$config_stage" "$config_target"
config_installed=true
if id captain-compose >/dev/null 2>&1; then
  chown -R captain-compose:captain-compose "$state_target"
  chmod 0700 "$state_target"
  find "$state_target" -type d -exec chmod 0700 {} +
  find "$state_target" -type f -exec chmod 0600 {} +
  chown -R root:captain-compose "$config_target"
  find "$config_target" -type d -exec chmod 0750 {} +
  find "$config_target" -type f -exec chmod 0640 {} +
else
  echo 'captain-compose account is absent; restored files remain root-owned and service must not be started' >&2
fi
trap - EXIT
rm -rf -- "$stage"
echo 'Restore completed with the service stopped. Review configuration, preflight, reconcile Docker state, then start explicitly.'
if "$state_moved" || "$config_moved"; then echo "Previous data retained at $state_saved and $config_saved"; fi
