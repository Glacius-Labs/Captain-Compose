# Captain Compose

[![CI](https://github.com/Glacius-Labs/Captain-Compose/actions/workflows/ci.yml/badge.svg)](https://github.com/Glacius-Labs/Captain-Compose/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Captain Compose is an operator CLI and Go agent that receive MQTT commands and manage
long-running Docker Compose workloads on a single host. It is designed for trusted
operators managing edge nodes and distributed installations.

![Captain Compose](captain-compose-banner.png)

## What it does

- Creates and updates image-based Compose deployments; waits for running/healthy services.
- Removes managed workloads while preserving their volumes.
- Persists commands, Compose manifests and results across process restarts.
- Deduplicates request IDs for seven days and retries undelivered status events.
- Reconnects and resubscribes to MQTT with TLS, optional mutual TLS and per-node topics.
- Ships as native binaries for Linux, macOS and Windows, with a Linux service installer.

- Provides an operator CLI with plans, revision guards, explicit revert and queryable results.
- Observes Docker state and exposes local health, readiness and Prometheus metrics.
- Supports verified installation, Ansible provisioning, offline backup and staged restore.

MQTT publishers are trusted host administrators: Compose can access the Docker host.
This is not a container sandbox, a cloud provisioner, or an exactly-once job runner.
See the [security model](docs/security.md) and [delivery guarantees](docs/architecture.md).

## Quick start from source

Requirements: the Go version in [go.mod](go.mod), Docker with Compose v2.20+,
and a reachable MQTT 3.1.1 broker. Production targets Linux; Windows/macOS development
uses Docker Desktop's Linux engine.

```bash
go build -o bin/captain-compose-mqtt ./cmd/captain-compose-mqtt
go build -o bin/captain-compose ./cmd/captain-compose
cp config/mqtt/config.example.yaml config.yaml
cp config/operator.example.yaml operator.yaml
docker compose -p captain-compose-dev -f docker/mosquitto/docker-compose.yml up -d
./bin/captain-compose-mqtt --config config.yaml --check
./bin/captain-compose-mqtt --config config.yaml
```

On PowerShell, use `Copy-Item` and build `bin/captain-compose-mqtt.exe`. The example
broker binds only to localhost and permits anonymous connections for development.
Production requires authenticated TLS and topic ACLs.

In another terminal, create `compose.yaml`:

```yaml
services:
  web:
    image: nginx:1.28-alpine
    ports:
      - "127.0.0.1:8080:80"
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1/"]
      interval: 5s
      timeout: 3s
      retries: 10
```

Check the node, preview the change, then deploy and inspect it:

```bash
./bin/captain-compose --config operator.yaml --environment local doctor
./bin/captain-compose --config operator.yaml --environment local plan web compose.yaml
./bin/captain-compose --config operator.yaml --environment local deploy web compose.yaml --request-file web-request.json
./bin/captain-compose --config operator.yaml --environment local inspect web
```

The CLI waits for the agent's result. Keep `web-request.json` private: it contains the
submitted Compose configuration and the exact request identity for safe retry. Use a
new request file for a different operation. Custom controllers can use the
[versioned MQTT protocol](docs/protocol.md) and [JSON schemas](schemas/command-v2.schema.json).

## Install and provision

Use [Captain Compose 1.1.0](https://github.com/Glacius-Labs/Captain-Compose/releases/tag/v1.1.0)
or a newer stable version from GitHub Releases.
Archives include both binaries, documentation, example config, license and installers.
The release pipeline produces checksums and build attestations.

```bash
bash scripts/install.sh --version 1.1.0 --prefix "$HOME/.local/bin"
bash scripts/provision.sh --config ./production.yaml --dry-run
```

For systemd, install to `/usr/local/bin`
and follow the [operations guide](docs/operations.md) to configure secrets, service
ownership and startup. The installer verifies SHA-256 and GitHub build provenance
before replacing either executable; offline bundles are supported. Provisioning preserves existing configuration and
starts the service only when `--start` is supplied.

## Operate a node

Build the operator with `go build -o bin/captain-compose ./cmd/captain-compose` and
configure named environments from `config/operator.example.yaml`. Each environment
addresses one node through its exact command/event topics. Keep operator credentials
separate from agent credentials; the broker grants inverse topic permissions.

```bash
captain-compose --config operator.yaml --environment production doctor
captain-compose --config operator.yaml --environment production plan web compose.yaml
captain-compose --config operator.yaml --environment production deploy web compose.yaml
captain-compose --config operator.yaml --environment production status
captain-compose --config operator.yaml --environment production inspect web
```

Deployments wait for a correlated result by default. Preserve the exact request file
when delivery is uncertain; query its request ID before issuing new work. Plans show
added/changed/removed services and operational warnings. Revert restores a stored
Compose revision only after explicit acknowledgement that application data is not
rolled back. See the [CLI guide](docs/cli.md) and [runtime operations](docs/runtime-operations.md).

Enable `monitor_listen: 127.0.0.1:9080` for local health, readiness, safe status and
Prometheus metrics. An optional exact `status_topic` carries non-retained heartbeats.
Remote access requires a host collector or authenticated TLS proxy.

For automated provisioning use the [Ansible role](deploy/ansible/README.md).
The [lifecycle guide](docs/lifecycle.md), [credential rotation](docs/credential-rotation.md),
[monitoring guide](docs/monitoring.md), and [acceptance matrix](docs/acceptance-matrix.md)
describe supported procedures and their actual verification status.

## Verify and contribute

```bash
go test -race ./...
go vet ./...
CAPTAIN_INTEGRATION=1 go test -race -tags=integration -timeout=5m ./...
```

Integration tests create and clean up their own Docker resources. GitHub CI runs
unit/race tests on three operating systems, real Docker/MQTT integration tests,
dependency vulnerability checks, installer checks and release packaging smoke tests.
Version tags on main produce a tested, attested **draft** release for maintainer review.

- [Architecture and guarantees](docs/architecture.md)
- [MQTT protocol and migration](docs/protocol.md)
- [Installation, upgrades and recovery](docs/operations.md)
- [Security](docs/security.md)
- [Readiness evidence](docs/production-readiness.md)
- [Problems, decisions and ideas](docs/problems-and-ideas.md)
- [Production operations delivery plan](docs/delivery-plan.md)
- [Pilot acceptance](docs/pilot.md)
- [Contributing and releases](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)

Licensed under [Apache-2.0](LICENSE).
