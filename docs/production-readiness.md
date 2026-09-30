# Production-readiness evidence

This document distinguishes implemented behavior from release and environment
acceptance. It is not a claim that an unspecified production host has been deployed.

## Implemented

- Durable managed manifests, preserved volumes and bounded Compose operations.
- Strict configuration/protocol validation and verified TLS by default.
- Persistent inbox/outbox, request deduplication and reconnect subscriptions.
- Exclusive process state lock, signal cancellation and nonzero startup failures.
- Versioned cross-platform archives, checksums, build attestations and draft releases.
- Offline-capable binary installation and repeatable Linux/systemd provisioning.
- Maintainer, security, migration, protocol and operations documentation.

## Local verification (2026-09-30)

- Go 1.27.1 on Windows amd64: `go test ./...` passes.
- `CAPTAIN_INTEGRATION=1 go test -tags=integration -timeout=5m ./...` passes against
  Docker Engine 29.4.1 / Compose 5.1.3 and Mosquitto 2.0.22.
- Docker lifecycle test applies twice, reopens persisted state, removes twice, then
  mounts the retained volume in a fresh container and verifies its data.
- Real MQTT integration verifies command/result flow and resubscription after broker
  restart. Journal tests verify persistence, duplicate handling, corruption rejection
  and retrying a saved event after restart without repeating the Docker operation.
- `govulncheck` v1.8.0 using Go 1.27.1 reports no vulnerabilities.

## GitHub verification

The branch CI verifies race tests on Linux/macOS/Windows, Docker and MQTT integration,
formatting, module consistency, vulnerability scanning, shell lint, installation and
all six release archives. Live run evidence will be recorded after the branch run.

## Release and production acceptance

Before calling a release production accepted, maintainers must configure required
branch checks, release reviewers and tag permissions; publish a reviewed release;
verify artifact checksums/provenance; configure real broker TLS/ACLs and backups; and
run create/update/remove plus disconnect/restart tests on the intended production
host. These depend on environment ownership and credentials, and are not inferred
from unit tests or a successful build. See `problems-and-ideas.md` for boundaries.
