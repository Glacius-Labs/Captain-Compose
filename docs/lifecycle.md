# Service lifecycle and recovery

This service can change Docker workloads and keeps its command journal, receipts and
deployment state under `/var/lib/captain-compose`. The full persistent data directory
and `/etc/captain-compose` must move together. Stop the service before backup, restore,
or upgrade; keep the broker from directing new commands to the node during maintenance.

## Backup

Install `scripts/backup.sh` and `scripts/archive-safety.py` from the reviewed release.
Run as root while the service is stopped. The script includes all state and protected
configuration, including MQTT TLS/password files and the explicit Docker registry
configuration directory. It refuses symlinked source roots, writes a mode `0600`
compressed archive, and requires the destination outside the backed-up trees:

```bash
sudo systemctl stop captain-compose.service
sudo bash scripts/backup.sh /secure/offline/captain-compose-2026-09-30.tar.gz
```

Encrypt the backup at rest and store it separately from the Docker host. It may contain
credentials, pending command payloads, workload definitions, and receipts. Do not place
it in source control, a public artifact store, or an unencrypted shared directory.

## Restore

Restore requires the agent service to be stopped. The archive validator rejects paths
outside the two expected trees, duplicate paths, symlinks, hard links and special files.
Restore first extracts into a private staging directory and checks the whole archive.
By default, it refuses to overwrite either destination:

```bash
sudo systemctl stop captain-compose.service
sudo bash scripts/restore.sh --archive /secure/offline/captain-compose-2026-09-30.tar.gz
```

For replacement, inspect the archive and capture a separate current backup first, then
pass both `--replace-existing` and the exact acknowledgement
`--confirm-replace state-and-config`. Existing directories are retained as
`*.pre-restore-TIMESTAMP` siblings. A failed staged move attempts to put the previous
directories back. Restore leaves the service stopped; verify file ownership, run the
binary preflight, inspect `docker compose ps`, and reconcile workloads before starting.
The restore only returns Captain Compose's local state and configuration. It does not
restore Docker volumes, containers, external databases, or broker data.

## Upgrade

Back up first, pin a published semantic version, and use a recent `gh` for signed
provenance verification. The script stops the service, installs both binaries only after
archive checksum and attestation checks pass, runs `captain-compose-mqtt --check` as the
service user, then returns an active service to active state. A preflight failure leaves
the service stopped for investigation:

```bash
sudo bash scripts/upgrade.sh --version 1.1.0-rc.2
```

`--archive-dir` supplies a downloaded release and checksum file. For offline use, pass
the detached archive bundle directory with `--attestation-dir`; for controlled bootstrap
only, `--checksum-only` skips provenance. Neither path makes state rollback safe.
Install the prior binary only after confirming it can read the exact current state
format. Captain Compose does not roll Docker workloads or application data back when
the binary is downgraded.

## Uninstall and node retirement

Disable and remove the service unit only after draining broker commands and resolving
pending operations. Keep state and backups until every deployment and receipt has a
known disposition. Removing Captain Compose does not stop containers or delete Docker
volumes. Retire workloads explicitly and dispose of application data under the host's
data-retention policy.
