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
2. **Application rollback:** explicit revision revert restores configuration after data-risk
   acknowledgement. Database/data rollback still requires an application-specific procedure.
   Failed Compose updates can be partial; saved YAML does not make data rollback automatic.
3. **High throughput:** serial mutations and bounded journal budgets target edge nodes.
   Queries and event delivery now have independent workers. Parallel mutations across
   projects would need ordering/resource policies and a separate capacity benchmark.
4. **Automatic reconciliation:** read-only status/metrics and drift observation are implemented.
   Automatically repairing drift remains a separate policy: another trusted operator may
   have intentionally changed or stopped a workload. Observation does not silently repair it.
5. **Host provisioning:** cloud VM creation, Docker installation, broker operation and
   certificate issuance remain outside the agent. Add provider-specific Terraform or
   Terraform modules when target environments and credential ownership are decided.
   The Ubuntu Ansible role installs/configures the agent on an existing Docker host.
6. **Container distribution:** consider GHCR images if customers prefer a containerized
   agent. Docker CLI/plugin compatibility and host bind-path semantics require separate
   validation. Native binary/systemd distribution is the initial supported route.
7. **Resource policy:** trusted operators can submit powerful Compose options. A future
   untrusted-tenant product would need a fundamentally stricter allowlist and isolation
   boundary; do not present current input validation as that boundary.
8. **Native Windows services:** binaries build for Windows; managed service provisioning
   and power-loss journal guarantees are validated primarily on Linux.
9. **Extended acceptance:** the disposable pilot records its actual observation interval.
   Physical reboot/power-loss and customer-specific network/storage acceptance require
   controlled reference infrastructure. A short CI run cannot establish multi-day stability.

## Release administration

Branch protection, required CI checks, immutable version tags, a tag-restricted release
environment and private vulnerability reporting are configured. The first stable
release uses explicit artifact verification before publishing its draft; a separate
required-human-reviewer gate is optional for a future multi-maintainer team.
Publishing a release and deploying onto a real production node are separate operations.
A successful test run is not evidence of production credentials or ACLs.

## Independent pre-release review

Luna High reviewers identified and prompted repairs for wall-clock-dependent queue
ordering, missing manifest-directory synchronization, stale/local-dirty release
packaging, existing configuration permissions, and gaps in TLS integration coverage.
Tests are added alongside each repair. These replace earlier prototype assumptions
with verifiable behavior; they do not remove the operating boundaries above.
