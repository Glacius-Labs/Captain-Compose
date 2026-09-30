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

[GitHub Actions run 36712417655](https://github.com/Glacius-Labs/Captain-Compose/actions/runs/36712417655)
passed all five jobs for implementation commit `9d4357c` on 2026-09-30:

- Race tests, vet and build on Linux, Windows and macOS.
- Real Docker lifecycle and MQTT broker-restart integration with the race detector.
- Formatting/module checks and vulnerability scanning.
- Shell lint, successful offline installation and rejection of a corrupted archive.
- All six Linux/macOS/Windows amd64/arm64 release archives and their checksums.
- Linux packaged-binary smoke test; systemd provisioning/start performed twice;
  installed service processed an MQTT command and emitted the matching success event.

The first Windows run exposed PowerShell splitting an unquoted coverage argument;
the corrected workflow passed. Local integration also exposed missing Windows Docker
plugin environment variables and an unstable ephemeral broker port; both were fixed
before this verified run. These findings are recorded to explain the earlier failed
runs, not as unresolved defects.

## Release and production acceptance

The release preparation includes independent Luna High reviews of runtime recovery,
delivery tooling and the operator contract. Findings led to persisted queue sequence
ordering, directory-synced manifests, stricter release packaging and additional TLS
coverage. See the release PR and `problems-and-ideas.md` for details.

Repository controls configured and read back on 2026-09-30:

- `main` requires an up-to-date PR with all five checks from the GitHub Actions app,
  including for administrators. Force pushes and branch deletion are disabled.
- The `release` environment accepts only `v*` tags.
- An active tag ruleset prevents updates and deletion of existing `v*` tags.
- Private vulnerability reporting is enabled.
- Release artifacts remain draft until their checksums and attestations are verified
  and the authorized release operator publishes them. No extra human-reviewer gate
  is configured for this single-maintainer release workflow.

Production-host acceptance is separate from publishing binaries: configure real
broker TLS/ACLs and backups, then run create/update/remove plus disconnect/restart
tests on the intended host. No production host has been selected or modified by this
release work. These environment checks are not inferred from a successful build.
