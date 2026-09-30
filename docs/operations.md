# Installation and operations

## Prerequisites

Linux/systemd, Docker Engine with Compose v2.20+ (including `up --wait` and JSON
`config` output), and a reachable MQTT 3.1.1 broker. The tested reference versions
are recorded in `production-readiness.md`. The installer does not install Docker,
open firewall ports, create cloud resources, or alter existing workloads.

## Install a release

Download `scripts/install.sh` from a reviewed repository tag or extract a release
archive, then run from its root:

```bash
bash scripts/install.sh --version 1.0.0 --prefix "$HOME/.local/bin"
captain-compose-mqtt --version
```

The version is an example; choose an actually published release. Installation
requires an explicit version and checks the archive against the release SHA-256
manifest before replacing the executable. `--archive-dir /path/to/assets` supports
offline installation with both the archive and `checksums.txt`. Releases contain
Linux/macOS tarballs and Windows ZIPs for amd64 and arm64. On Windows, compare
`Get-FileHash -Algorithm SHA256` with the manifest, extract the ZIP, and run the EXE.

Checksums detect corruption. Verify provenance separately when using GitHub CLI:

```bash
gh attestation verify captain-compose_1.0.0_linux_amd64.tar.gz --repo Glacius-Labs/Captain-Compose
```

## Provision a Linux service

1. Install the binary into `/usr/local/bin` using the release installer with sudo.
2. Copy `config/mqtt/production.example.yaml`; set the real broker, unique client ID
   and topics. Use absolute paths and `state_dir: /var/lib/captain-compose`.
3. Place the CA and password (and optional client certificate/key) under
   `/etc/captain-compose`, readable by the service group only. Configure credentials
   on the broker and test its ACLs before enabling deployment access.
4. Preview, provision, then start:

```bash
bash scripts/provision.sh --config ./production.yaml --dry-run
sudo bash scripts/provision.sh --config ./production.yaml
# Install secrets now: root:captain-compose, mode 0640.
sudo bash scripts/provision.sh --config ./production.yaml --start
sudo journalctl -u captain-compose.service -f
```

Provisioning is repeatable: it preserves existing configuration contents, secures an
existing config as `root:captain-compose` mode `0640`, creates the system user, grants
Docker group access, installs the unit, and enables it. Symlinked or non-regular config
destinations are rejected. Starting is explicit. The unit restarts failed processes and
stops the agent on SIGTERM without tearing down workloads. `--start` stops any running
agent before preflight to release its state lock, then restarts it. A failed preflight
leaves the service stopped for repair. Existing config edits require `systemctl restart`.

## Configuration

`--config PATH` defaults to `config.yaml`. `--check` validates config, TLS files and
Docker access without contacting the MQTT broker or deploying workloads. It also
checks exclusive access to state, so stop the service before running it. This is a
preflight, not proof of broker reachability or successful authentication.

YAML keys are strict. Defaults are `state_dir: ./state`, `operation_timeout: 5m`,
and JSON info logs to stdout. Operation timeout accepts Go durations from 1s to 1h.
Use journald for log retention/rotation. Optional `log.file_path` is unrotated and
must be managed externally; prefer stdout for services.

MQTT passwords can come from `password_file` or `password`; the environment variable
`CAPTAIN_COMPOSE_MQTT_PASSWORD` overrides both. TLS validates certificates and hostnames,
supports custom CAs and optional mutual TLS, and requires TLS 1.2+. Never share a
client ID between agents. Preserve the client ID across restarts for broker sessions.

## Recovery and upgrades

- Stop the service and back up state, configuration and workload data before upgrades.
- Install the new pinned binary, run preflight, restart, and inspect logs and events.
- Roll back by reinstalling the previous binary only if its documented state/protocol
  format is compatible. The prototype is **not** a compatible rollback target.
- A failed apply can leave a partially changed workload. Inspect
  `docker compose -p cc-NAME ps` and container logs. Resubmit a corrected manifest
  with a new request ID, or send remove. There is no automatic rollback.
- An undeliverable event blocks later operations. Restore broker connectivity and
  permissions; the outbox retries without repeating the completed Docker operation.
- Never delete a pending journal to unblock the agent. Corrupt state requires an
  offline backup restore and reconciliation of actual Docker state.
- Stop the service before moving state. Copy the entire directory, including receipts;
  losing receipts can allow old commands to execute again.
- Monitor process restarts, journal file counts/disk space, `Event delivery pending`,
  broker connections and application health. No HTTP health or metrics endpoint exists.

## Uninstall

Stop and disable `captain-compose.service`, then remove its unit and binary if desired.
Keep `/var/lib/captain-compose` until all deployments and recovery obligations are
resolved. Uninstalling the agent does not stop containers or delete volumes. Explicitly
remove deployments before retiring a node; dispose of data only with a backup plan.
