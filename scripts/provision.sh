#!/usr/bin/env bash
# Configure an already installed agent as a Linux systemd service.
set -euo pipefail
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=scripts/provision-lib.sh
source "$script_dir/provision-lib.sh"
config=''
start=false
dry_run=false
while (($#)); do
  case "$1" in
    --config) config=${2:?Missing config path}; shift 2 ;;
    --start) start=true; shift ;;
    --dry-run) dry_run=true; shift ;;
    --help) echo 'Usage: provision.sh --config FILE [--start] [--dry-run]'; exit 0 ;;
    *) echo 'Unknown argument' >&2; exit 2 ;;
  esac
done
[[ -f "$config" ]] || { echo 'Supply a production configuration with --config' >&2; exit 2; }
[[ "$(uname -s)" == Linux ]] || { echo 'Provisioning requires Linux/systemd' >&2; exit 2; }
if "$dry_run"; then
  echo 'Plan: create captain-compose system user with Docker access; install configuration only if absent; install systemd unit; enable service.'
  echo "Start/restart service: $start"
  exit 0
fi
[[ "$EUID" == 0 ]] || { echo 'Run with sudo (Docker access grants host administration)' >&2; exit 1; }
validate_config_destination /etc/captain-compose/config.yaml
[[ -x /usr/local/bin/captain-compose-mqtt ]] || { echo 'First install the release into /usr/local/bin' >&2; exit 1; }
command -v systemctl >/dev/null
getent group docker >/dev/null || { echo 'Install Docker Engine and Compose plugin first' >&2; exit 1; }
id captain-compose >/dev/null 2>&1 || useradd --system --home-dir /var/lib/captain-compose --shell /usr/sbin/nologin captain-compose
usermod -a -G docker captain-compose
install -d -o captain-compose -g captain-compose -m 0700 /var/lib/captain-compose
install -d -o root -g captain-compose -m 0750 /etc/captain-compose
if [[ ! -e /etc/captain-compose/config.yaml ]]; then
  install -o root -g captain-compose -m 0640 "$config" /etc/captain-compose/config.yaml
else
  secure_config_destination /etc/captain-compose/config.yaml captain-compose
  echo 'Existing configuration preserved: /etc/captain-compose/config.yaml'
fi
cat > /etc/systemd/system/captain-compose.service <<'UNIT'
[Unit]
Description=Captain Compose MQTT deployment agent
Wants=network-online.target
After=network-online.target docker.service
Requires=docker.service
StartLimitIntervalSec=120
StartLimitBurst=10

[Service]
Type=simple
User=captain-compose
Group=captain-compose
SupplementaryGroups=docker
WorkingDirectory=/var/lib/captain-compose
ExecStart=/usr/local/bin/captain-compose-mqtt --config /etc/captain-compose/config.yaml
Restart=on-failure
RestartSec=10
TimeoutStopSec=30
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/captain-compose
RestrictSUIDSGID=true
LockPersonality=true

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable captain-compose.service
if "$start"; then
  systemctl stop captain-compose.service
  (cd /var/lib/captain-compose && runuser -u captain-compose -- /usr/local/bin/captain-compose-mqtt --config /etc/captain-compose/config.yaml --check)
  systemctl restart captain-compose.service
  systemctl is-active --quiet captain-compose.service
fi
echo 'Provisioned. Inspect: journalctl -u captain-compose.service -f'
