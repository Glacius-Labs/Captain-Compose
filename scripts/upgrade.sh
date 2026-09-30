#!/usr/bin/env bash
# Stop, verify/install a pinned release, preflight, and restore the prior service state.
set -euo pipefail
version=''
archive_dir=''
attestation_dir=''
checksum_only=false
while (($#)); do
  case "$1" in
    --version) version=${2:?Missing version}; shift 2 ;;
    --archive-dir) archive_dir=${2:?Missing archive directory}; shift 2 ;;
    --attestation-dir) attestation_dir=${2:?Missing attestation directory}; shift 2 ;;
    --checksum-only) checksum_only=true; shift ;;
    --help) echo 'Usage: upgrade.sh --version X.Y.Z [--archive-dir DIR] [--attestation-dir DIR | --checksum-only]'; exit 0 ;;
    *) echo 'Unknown argument' >&2; exit 2 ;;
  esac
done
[[ "$EUID" == 0 ]] || { echo 'Run upgrade as root' >&2; exit 1; }
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]] || { echo 'An explicit semantic version is required' >&2; exit 2; }
if "$checksum_only" && [[ -n "$attestation_dir" ]]; then
  echo '--checksum-only cannot be combined with --attestation-dir' >&2
  exit 2
fi
command -v systemctl >/dev/null
was_active=false
if systemctl is-active --quiet captain-compose.service; then was_active=true; fi
systemctl stop captain-compose.service
install_args=(--version "$version" --prefix /usr/local/bin)
[[ -z "$archive_dir" ]] || install_args+=(--archive-dir "$archive_dir")
[[ -z "$attestation_dir" ]] || install_args+=(--attestation-dir "$attestation_dir")
if "$checksum_only"; then install_args+=(--checksum-only); fi
bash "$(dirname -- "${BASH_SOURCE[0]}")/install.sh" "${install_args[@]}"
runuser -u captain-compose -- env DOCKER_CONFIG=/etc/captain-compose/docker \
  /usr/local/bin/captain-compose-mqtt --config /etc/captain-compose/config.yaml --check
if "$was_active"; then
  systemctl start captain-compose.service
  systemctl is-active --quiet captain-compose.service
fi
echo "Upgraded to $version and passed configuration/Docker preflight."
echo 'No state rollback was attempted; confirm release state-format compatibility before any binary rollback.'
