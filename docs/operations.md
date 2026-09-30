# Installation and operations

## Prerequisites

Linux/systemd, Docker Engine with Compose v2.20+ (including `up --wait` and JSON
`config` output), and a reachable MQTT 3.1.1 broker. The tested reference versions
are recorded in `production-readiness.md`. The installer does not install Docker,
open firewall ports, create cloud resources, or alter existing workloads.

## Install a release

Download `scripts/install.sh` from a reviewed repository tag or extract a release
archive, then run:

```bash
bash scripts/install.sh --version 1.1.0-rc.1 --prefix "$HOME/.local/bin"
captain-compose-mqtt --version
captain-compose --version
```

The version is an example; choose an actually published release. Installation
requires an explicit version, checks the archive against the release SHA-256
manifest, and verifies a GitHub artifact attestation for the exact tag and release
workflow before replacing either executable. It installs both `captain-compose-mqtt`
and `captain-compose` from the same archive. `--archive-dir /path/to/assets` supplies
the archive and `checksums.txt`. Releases contain
Linux/macOS tarballs and Windows ZIPs for amd64 and arm64. On Windows, compare
`Get-FileHash -Algorithm SHA256` with the manifest, verify provenance, then extract
the ZIP and run both EXEs:

```powershell
gh attestation verify .\captain-compose_1.1.0-rc.1_windows_amd64.zip `
  --repo Glacius-Labs/Captain-Compose `
  --signer-workflow Glacius-Labs/Captain-Compose/.github/workflows/release.yml `
  --source-ref refs/tags/v1.1.0-rc.1 --deny-self-hosted-runners
```

Checksums detect corruption; the signed provenance check establishes the expected
GitHub repository, release workflow, and version tag. GitHub CLI (`gh`) must be
installed for online verification. A release includes per-archive `.jsonl` bundle
files and `trusted_root.jsonl` under its `offline` assets for disconnected installs:

```bash
bash scripts/install.sh --version 1.1.0-rc.1 --archive-dir ./release-assets \
  --attestation-dir ./release-assets/offline
```

For a controlled bootstrap where signed provenance cannot yet be checked, pass
`--checksum-only` explicitly. This verifies only the checksum manifest and provides
no publisher identity guarantee. Never use this mode for routine production upgrades.

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
Docker group access, creates a root-owned `/etc/captain-compose/docker` for explicitly
managed registry credentials, installs the unit, and enables it. The service sets
`DOCKER_CONFIG` to that directory; Docker credentials are not taken from root's home.
Symlinked or non-regular config
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
- Install the new pinned release with `scripts/upgrade.sh`, which verifies provenance,
  stops the service, runs a Docker/configuration preflight and returns the service to
  its previous active state when the check succeeds.
- Version 1.1 state cannot be downgraded directly to 1.0. Preserve the complete
  pre-upgrade backup; to return to 1.0, stop the service, restore that backup, then
  install the trusted 1.0 MQTT agent binary using the 1.0 release procedure. Do not
  try to make a current release installer accept a 1.0 archive that lacks the CLI.
- A failed apply can leave a partially changed workload. Inspect
  `docker compose -p cc-NAME ps` and container logs. Resubmit a corrected manifest
  with a new request ID, or send remove. An explicit revert changes desired Compose
  configuration but does not restore Docker volumes or external databases.
- Result delivery runs independently from Docker execution. A broker outage can delay
  saved events without holding up other accepted operations. The durable journal is
  bounded at 128 entries: 112 mutation slots and 16 reserved query slots. When the
  relevant slots are full, new commands remain unacknowledged until capacity returns.
  Restore broker connectivity and ACLs so saved results can drain; operations are not
  repeated just because event publication is retried.
- Never delete a pending journal to unblock the agent. Corrupt state requires an
  offline backup restore and reconciliation of actual Docker state.
- Stop the service before moving state. Copy the entire directory, including receipts;
  losing receipts can allow old commands to execute again.
- Monitor process restarts, journal file counts/disk space, `Event delivery pending`,
  broker connections and application health. For the loopback HTTP endpoints and
  optional non-retained MQTT heartbeat see [monitoring.md](monitoring.md). The sample
  configuration defaults to `monitor_listen: 127.0.0.1:9080`; do not expose this
  listener directly to an untrusted network.

Use `captain-compose doctor` from the operator CLI for live broker, TLS, credential,
Docker Engine and Compose checks. Agent `--check` validates local config/TLS paths and
Docker access without contacting the broker; it does not prove MQTT reachability.

For scripted lifecycle controls, see [lifecycle.md](lifecycle.md) and the Ansible role
in [deploy/ansible/README.md](../deploy/ansible/README.md). The role is pinned to an
explicit release version, requires attestation verification by default, and leaves
existing config contents unchanged. A restored/updated agent always requires manual
reconciliation of actual Docker workloads before resuming operation.

## Uninstall

Stop and disable `captain-compose.service`, then remove its unit and binary if desired.
Keep `/var/lib/captain-compose` until all deployments and recovery obligations are
resolved. Uninstalling the agent does not stop containers or delete volumes. Explicitly
remove deployments before retiring a node; dispose of data only with a backup plan.
