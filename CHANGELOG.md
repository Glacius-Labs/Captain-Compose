# Changelog

## 1.1.0-rc.1 — 2026-09-30

### Operator workflow

- Add the `captain-compose` controller CLI with named environments, verified TLS,
  explicit request records, bounded retries, queryable outcomes and automation exit codes.
- Add plan, observed status, inspect, doctor and explicitly acknowledged revision revert.
- Report fixed diagnostic codes without echoing raw Docker stderr or workload secrets.

### Deployment and protocol

- Introduce version 2 requests with execution expiry, optimistic revision guards and
  durable request identity. Continue accepting version 1 create/remove requests.
- Persist desired and successful revisions with bounded, queryable history; read 1.0
  manifests during upgrade. Application data is never rolled back automatically.
- Separate serial mutations, read-only queries and result delivery; bound each queue
  and reserve diagnostic capacity. Keep undelivered results until delivery succeeds.
- Retain mutation receipts for seven days and query receipts separately for ten minutes.

### Operations and delivery

- Add loopback-only health/readiness/status endpoints, Prometheus metrics, heartbeat
  publication and example alerts.
- Verify archive provenance by default, package both executables, and provide scripted
  backup, staged restore and pinned upgrades plus an Ubuntu Ansible role.
- Add credential-rotation guidance and explicit Docker registry credential configuration.
- Add native ARM64 CI and a disposable fault-injection/upgrade pilot with timestamped
  evidence, including explicit boundaries for multi-day and physical-host acceptance.

### Compatibility

- Back up the complete 1.0 state and configuration before upgrading. Once revisioned
  deployment state is written, binary-only downgrade to 1.0 is unsupported.
- The new installer requires GitHub CLI for provenance verification unless the operator
  explicitly selects checksum-only bootstrap. See the offline verification procedure.

## 1.0.0 — 2026-09-30

First stable release of the MQTT deployment agent and its installation tooling.

### Deployment and recovery

- Persist normalized Compose manifests before applying workloads.
- Confine project state to the agent's private state directory and reserve the
  `cc-` project prefix for managed deployments.
- Wait for running/healthy services, enforce operation deadlines, and preserve
  volumes during removal. Unknown managed removal is an idempotent no-op.
- Persist accepted MQTT requests and result events; deduplicate caller-generated
  UUIDs and retry event delivery without repeating completed Docker work.
- Restore subscriptions after broker reconnects and serialize command execution
  using persisted enqueue sequences, unaffected by system-clock corrections.
- Sync manifest directory changes on Unix and reject further journal writes after
  an uncertain persistence failure.

### Security and operations

- Require verified TLS unless plaintext development mode is explicitly selected.
- Support mutual TLS, password files and environment-based password injection.
- Validate configuration and protocol fields, bound payloads, and reject remote
  Compose features that depend on implicit local files.
- Add preflight/version flags, exclusive state locking, structured logs and graceful
  process shutdown, including cancellation of Docker CLI/plugin process trees.
- Provide checksummed offline-capable installation and repeatable Linux/systemd
  service provisioning.

### Delivery and documentation

- Test on Linux, macOS and Windows, including race detection, real Docker/MQTT
  integration, vulnerability scanning and installed-service smoke tests.
- Publish native amd64/arm64 archives for Linux, macOS and Windows, accompanied by
  SHA-256 checksums and GitHub build attestations.
- Document protocol, migration, architecture, operations, security and known limits.

### Breaking changes from the untagged prototype

- Every command now requires a UUID `id`.
- Projects use `cc-<name>`; unprefixed prototype workloads are not implicitly adopted.
- Removal preserves data volumes instead of deleting them.
- Use per-node MQTT client IDs/topics. Plaintext requires `allow_insecure: true`;
  disabled TLS certificate verification is no longer supported.
- State is persistent and must be backed up. Prototype deployments need explicit
  migration as described in [the protocol guide](docs/protocol.md).

There is no cross-system exactly-once transaction or automatic rollback of partial
Compose updates. Workloads must tolerate reapplication after an interrupted command.
