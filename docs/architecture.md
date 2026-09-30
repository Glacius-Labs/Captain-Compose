# Architecture and delivery semantics

Captain Compose is a single-node agent that reconciles remote Compose requests
against one Docker daemon. Linux with Docker Engine is the production deployment
target. macOS/Windows binaries support development against Docker Desktop's Linux
engine. This is a trusted administration channel, not a multi-tenant sandbox.

## Ownership and execution

- `cmd/captain-compose-mqtt` owns configuration, TLS, process locking, signals and
  composition of adapters. Startup failures return exit status 1.
- `internal/adapter/mqtt` validates the wire protocol and owns the persistent inbox,
  result outbox and seven-day duplicate receipts.
- `internal/app/deployment` owns create/remove orchestration and result events.
- `internal/domain/deployment` defines ports, events and input invariants.
- `internal/adapter/docker` owns manifest persistence and Docker Compose execution.
- `internal/control` defines the version 2 operation and observation contract.
- `internal/operator` and `cmd/captain-compose` own the operator workflow and exact
  request replay; they have no direct Docker access.
- `internal/observability` exposes loopback-only, read-only cached observations.

The callback stores a valid command before acknowledging QoS 1. A serial mutation
worker preserves persisted enqueue order, independent of wall-clock adjustments.
Queries and result delivery have separate bounded workers. Results are persisted
before publication and their delivery is recorded after broker acknowledgement.
The broker session is persistent, and the exact topic is resubscribed after every reconnect.

## Guarantees and limits

Commands require caller-generated UUIDs. Reusing an ID with identical content does
not repeat an operation within seven days. New IDs intentionally request new work.
Consumers deduplicate events by `id` and correlate by `request_id`: a crash between
publication and receipt storage can publish the same event again.

A crash between Docker changing state and result persistence can repeat the Compose
operation. There is no transaction spanning Docker and the journal. Apply/remove
converge on project state; exactly-once execution of application side effects is
not guaranteed. Do not use this agent for non-idempotent one-off jobs.

There is one in-flight mutation. Pending commands and undelivered results share a
bounded budget, with capacity reserved for diagnostic queries. Mutation receipts are
retained for seven days; query receipts have a separate, shorter retention and capacity.
Undelivered results do not expire. See the protocol guide for exact limits.
An unavailable event broker does not block execution of already accepted commands
until capacity is exhausted. Bound publisher rates and alert on delivery backlog.
Disk errors fail closed. Keep state on a local durable filesystem;
network shares and multiple processes sharing state are unsupported.

The state lock prevents two local agent processes from sharing a directory. It does
not coordinate different state directories, Docker contexts or duplicate MQTT client
IDs. Operators must assign one agent owner per Docker daemon and unique node topics.

## Compose contract

Names contain 1-63 lowercase letters, digits, `_` or `-`, starting with a letter or
digit. Projects are named `cc-<name>`. Reserve that prefix for this agent.
Only persisted managed manifests can be removed. Unknown removal is a successful
no-op; volumes are always retained.

Payloads are self-contained YAML, at most 1 MiB, with image-based long-running
services. Build contexts, include/extends, profiles, env/label files, develop mode,
relative bind mounts, and file/environment-backed secrets/configs are rejected.
Explicit absolute Linux bind mounts and privileged settings remain powerful host
operations: authorize publishers accordingly. Pin production images by digest.

Compose validates and normalizes a request before replacing its manifest. The
manifest is saved before `up --wait --remove-orphans`, including when startup fails
partially. `success: true` means Compose observed services running/healthy; meaningful
application readiness requires service healthchecks. There is no automatic rollback.
Version 2 plans expose service changes and warnings without returning submitted secrets.
Revision preconditions detect concurrent changes; persisted request identity allows an
interrupted operation to recognize its own prior intent. Expiry rejects a mutation
that has not durably started; it does not promise to undo an already started operation.
An update removes services omitted from the new manifest. Removal uses the saved
manifest with `down --remove-orphans`, preserving named and anonymous volumes.

Subprocesses inherit only Docker/OS connectivity settings, never MQTT credentials.
Implicit `.env` loading is disabled. Compose diagnostics are suppressed because they
may contain submitted secrets. Logs and events contain generic operation errors.
Cancellation terminates the Docker CLI process group on Linux/macOS and requests
termination of its process tree on Windows. Already accepted Docker Engine requests
can still leave partial changes; cancellation is not a rollback. Windows requires
permission to terminate the CLI's descendants, as provided in normal local execution.

## Storage

Legacy `state_dir/deployments/<name>/compose.json` is read during upgrade.
Version 2 `state_dir/deployments/<name>/state.json` is the authoritative deployment
record: desired and last successful revision, operation identity, phase and bounded
revision history. History contains private Compose configuration and must be protected
with the same controls as the active manifest. Old binaries cannot interpret the new
deployment state: restore the complete pre-upgrade backup for a binary downgrade.
`state_dir/journal/<hash>.json` contains pending commands or completed receipts.
Files are written with restrictive permissions using write, fsync, and atomic rename;
the MQTT journal and deployment-manifest directories are synced on Unix. `os.OpenRoot` confines access.
Power-loss guarantees depend on the filesystem; Windows lacks directory fsync here.
Back up the complete state directory together while the agent is stopped.
