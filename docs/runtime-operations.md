# Runtime operations

The Docker adapter implements the version 2 `create`, `remove`, `plan`, `status`,
`inspect`, `doctor`, and `revert` operations. The existing version 1 deploy, remove,
and list entry points remain available and share the same deployment state.

## Revisions and changes

A revision is the 64-character lowercase hexadecimal SHA-256 digest of Docker Compose's normalized
JSON configuration. Compose normalization runs with the deployment's isolated project
name and no implicit environment file. The hash covers the normalized configuration,
including values that may be secrets; query results expose only the hash and service
metadata, never the manifest or secret values.

`expected_revision` is optional. When supplied as an empty string it is create-only;
when non-empty it must match the current desired revision. `create` records the desired
revision before `compose up`, so failed or interrupted applies remain inspectable and
retryable. `successful_revision` advances only after Compose reports a successful apply.
After removal, there is no current desired revision; a non-empty expected revision from
before removal is stale and cannot recreate the deployment. Use an empty expected revision
for a create-only operation.
The same request ID, operation, and revision can resume its own persisted partial state.
Reusing a request ID for a different operation is rejected. Docker Compose is not
transactional: after a process or host crash, a retry can repeat Compose work.

The adapter persists one authoritative `state.json` per deployment. A synced temporary
file is atomically renamed into place, and the containing directory is synced where the
platform supports it. State records the desired manifest, phase, last successful
revision, request ID, and at most ten revision snapshots including the current desired
revision. The last successful content is retained while it differs from the current
desired content. Metadata hashes and history entries
are checked when read; corrupt state fails closed. Existing 1.0 `compose.json` state is
readable and migrates when next written. Legacy API mutations update the same state.

`revert` requires the caller's current `expected_revision`, a retained `revision`, and
`allow_data_risk: true`. It reapplies that configuration after an image pull preflight.
It does not restore named volumes, bind mounts, database contents, external services,
or any other application data. Retained revision metadata shows that a configuration can
be reapplied; it does not prove that the revision was healthy or accepted previously.

## Plans and observations

`plan` normalizes the proposed manifest and compares the service objects with the
stored desired configuration. It reports added, changed, and removed service names.
Warnings identify missing health checks, missing memory or CPU limits, image references
without a digest, and mounts. Limits describe container configuration only; the plan
does not reserve or establish host capacity. Registry access and image availability are
checked with `compose pull` before a mutation changes the stored desired state or starts
replacing services.

`status` without a name checks Docker daemon connectivity and returns all deployments;
with a name it returns one. Both forms use `docker compose ps --all --format json` for
observed container state. `inspect` uses the same bounded, safe summary for one
deployment. `drift` is true only when observed service names differ from configured
service names. It does not compare image digests, configuration values, mounts, networks,
or container health with desired policy. A status request is bounded by its caller's
context and performs one Docker observation per active deployment, so a large fleet can
consume that deadline.

`doctor` reports fixed, secret-safe checks for Docker daemon access, Docker Compose
version (minimum 2.20), a synced write-and-remove probe in the confined state store, and
read access to Docker's registry configuration when present. The registry check confirms
only that the config file is readable JSON; it does not validate credential correctness
or contact a registry.

## Error reporting and boundaries

Docker stderr is captured only in a bounded private buffer and is never returned. A few
known diagnostics map to fixed codes such as registry denial, unavailable image, registry
reachability, occupied port, or full disk. Other failures return a generic phase-specific
code. These messages do not include Compose payloads, registry credentials, arbitrary
stderr, container logs, or host paths.

The preflight pull verifies that referenced images can be pulled at that moment. It does
not reserve them against later registry changes, verify that the registry will remain
available, or guarantee that Compose apply will succeed. Daemon-side changes can occur
outside this adapter. Multi-host coordination, data rollback, and capacity reservation
are not provided.
