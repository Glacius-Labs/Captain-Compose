# Production operations delivery

This plan extends 1.0 without claiming that CI substitutes for production observation.
The supported production baseline is Ubuntu 24.04, systemd, Docker Engine/Compose,
and an authenticated TLS MQTT broker. One trusted agent owns one Docker daemon.

## Work packages

1. Version 2 commands, expiry, queryable results and independent bounded result delivery.
2. Operator CLI with named environments, safe credentials, stable exit codes and retries.
3. Revisioned desired/successful state, plans, observation, preflight and explicit revert.
4. Local read-only monitoring, heartbeat and bounded diagnostic information.
5. Verified installation, Ansible lifecycle, broker ACL/credential rotation and recovery.
6. Disposable reference-host acceptance, upgrade/recovery and fault-injection tests.

Version 1 commands remain accepted. Version 2 mutations carry a caller UUID and may
include an expiry and expected revision. Empty expected revision means create-only.
Queries use the same exact command/event topics and contain no caller-selected reply
topic. Results never expose submitted Compose payloads or secrets. Revert requires
explicit acknowledgement that application data is not rolled back.

## Acceptance

The 1.1 operations release is first published as `1.1.0-rc.1`. Stable promotion
requires green CI, verified signed release archives, and a successful 72-hour pilot
with no observation gap exceeding 120 seconds. Version tags remain immutable. Any
runtime correction restarts the full observation period on the corrected candidate.
The first two 72-hour attempts did not pass: Docker Desktop stopped the first, and
the second outlived a disposable broker certificate. Candidate `1.1.0-rc.2` fixes
that test fixture; it needs a new complete observation window before stable release.

- A new operator can install, plan, deploy, query an uncertain result, diagnose and recover.
- Event publication failure does not repeat completed Docker work or block other accepted
  operations until documented bounded storage capacity is exhausted.
- Expired queued commands do not mutate Docker; stale revisions fail before mutation.
- Previously accepted 1.0 state and requests remain readable; upgrade tests prove it.
- CI verifies Linux/Windows/macOS, real Docker/MQTT, installation and systemd behavior.
- A pilot records its actual duration and injected failures. Multi-day and native ARM64
  acceptance remain unverified until those exact runs finish; infrastructure costs and
  production workloads are not implied by this development plan.
