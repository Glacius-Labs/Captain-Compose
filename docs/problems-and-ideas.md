# Problems, decisions and future work

## Resolved in the production-readiness branch

| Prototype problem | Resolution |
| --- | --- |
| Deleted Compose files after apply | Persist normalized manifests before execution |
| Remove omitted a manifest and deleted volumes | Managed-state lookup; explicit project prefix; preserve volumes |
| Invalid Compose listing command | List only persisted managed intents |
| MQTT reconnect lost subscription | Resubscribe on every connection and pre-register message route |
| Callback ran Docker and waited for publication | Durable inbox plus a separate serial worker |
| Lost commands/events on crashes | Durable command/result journal and duplicate receipts |
| Unbounded waits, permissive configuration | Context deadlines, strict YAML/JSON and TLS checks |
| Prototype Go version mismatched release workflow | One Go version in go.mod; gated release matrix |
| No tested installation path | Checksummed installer, offline mode and systemd provisioning |

## Intentional boundaries and remaining decisions

1. **Exactly-once effects:** Docker and filesystem updates cannot commit atomically.
   Recovery may reapply a command. Keep workloads idempotent; a transaction protocol
   across arbitrary containers is outside scope.
2. **Rollback:** failed Compose updates can be partial. Add an explicit versioned
   deployment/revert command only after defining volume migration and data rollback
   semantics. Never advertise automatic rollback based only on saved YAML.
3. **High throughput:** the serial worker, 128 pending commands and 10,000 seven-day
   receipts target edge nodes. A future version could use a transactional database,
   per-project queues and independent event delivery, with ordering defined explicitly.
4. **Observability:** add read-only status/metrics and controller reconciliation once
   the expected monitoring system and authentication model are known.
5. **Host provisioning:** cloud VM creation, Docker installation, broker operation and
   certificate issuance remain outside the agent. Add provider-specific Terraform or
   Ansible modules when target environments and credential ownership are decided.
6. **Container distribution:** consider GHCR images if customers prefer a containerized
   agent. Docker CLI/plugin compatibility and host bind-path semantics require separate
   validation. Native binary/systemd distribution is the initial supported route.
7. **Resource policy:** trusted operators can submit powerful Compose options. A future
   untrusted-tenant product would need a fundamentally stricter allowlist and isolation
   boundary; do not present current input validation as that boundary.
8. **Native Windows services:** binaries build for Windows; managed service provisioning
   and power-loss journal guarantees are validated primarily on Linux.

## Release administration

Before the first public release, configure branch protection, required CI checks,
tag permissions, the release environment reviewers and private security reporting.
Publishing a release and deploying onto a real production node remain explicit release
operations. A successful test run is not evidence of production credentials or ACLs.
